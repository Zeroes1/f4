//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	ntdll    = syscall.NewLazyDLL("ntdll.dll")
	version  = syscall.NewLazyDLL("version.dll")

	procCreatePseudoConsole = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole  = kernel32.NewProc("ClosePseudoConsole")
	procInitAttrList        = kernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateAttr          = kernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteAttrList      = kernel32.NewProc("DeleteProcThreadAttributeList")
	procCreateToolhelp32    = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW     = kernel32.NewProc("Process32FirstW")
	procProcess32NextW      = kernel32.NewProc("Process32NextW")
	procGetProcessMemInfo   = kernel32.NewProc("K32GetProcessMemoryInfo")

	procRtlGetVersion = ntdll.NewProc("RtlGetVersion")

	procGetFileVersionInfoSizeW = version.NewProc("GetFileVersionInfoSizeW")
	procGetFileVersionInfoW     = version.NewProc("GetFileVersionInfoW")
	procVerQueryValueW          = version.NewProc("VerQueryValueW")
)

const (
	procThreadAttributePseudoConsole = 0x00020016
	extendedStartupInfoPresent       = 0x00080000

	// CREATE_NO_WINDOW belongs to the same family as CREATE_NEW_CONSOLE and
	// DETACHED_PROCESS: it says "a console application run without a console
	// window", and it competes with PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE for
	// deciding which console the child gets. The first field run of this
	// probe passed it and measured silence: the children ran and exited 0,
	// having written to a console of their own. Microsoft's sample passes
	// EXTENDED_STARTUPINFO_PRESENT and nothing else. Kept as a named constant
	// because one strategy in the smoke-test matrix deliberately sets it, to
	// keep that finding reproducible rather than folkloric.
	createNoWindow = 0x08000000

	// DETACHED_PROCESS: no console at all. This is what direction A needs.
	detachedProcess = 0x00000008

	th32csSnapProcess = 0x00000002

	processQueryLimitedInformation = 0x1000
	processVMRead                  = 0x0010
)

// ---------------------------------------------------------------------------
// A pseudoconsole we own, and the child inside it
// ---------------------------------------------------------------------------

// spawnStrategy is the set of choices that turned out to matter for whether a
// pseudoconsole delivers anything at all. The smoke test walks them and the
// rounds use the first that works, so a machine that needs a different
// combination reports it instead of producing an unexplained silence.
type spawnStrategy struct {
	Name            string
	NoWindowFlag    bool // add CREATE_NO_WINDOW (expected to break attachment)
	Inheritable     bool // create the pipes with an inheritable SECURITY_ATTRIBUTES
	CloseChildEnds  bool // close our copies of the pseudoconsole's ends after spawn
	PumpBeforeSpawn bool
	PipeSize        uint32
}

func defaultStrategies() []spawnStrategy {
	return []spawnStrategy{
		{Name: "plain (MS sample shape)", CloseChildEnds: true, PumpBeforeSpawn: true},
		{Name: "keep our pipe ends open", CloseChildEnds: false, PumpBeforeSpawn: true},
		{Name: "inheritable pipes", Inheritable: true, CloseChildEnds: true, PumpBeforeSpawn: true},
		{Name: "pump after spawn", CloseChildEnds: true, PumpBeforeSpawn: false},
		{Name: "1MB pipe buffer", CloseChildEnds: true, PumpBeforeSpawn: true, PipeSize: 1 << 20},
		{Name: "with CREATE_NO_WINDOW", NoWindowFlag: true, CloseChildEnds: true, PumpBeforeSpawn: true},
	}
}

type session struct {
	hpc     uintptr
	inWrite syscall.Handle
	outRead syscall.Handle

	inRead   syscall.Handle
	outWrite syscall.Handle

	childProc syscall.Handle
	childPID  uint32

	hostPID     uint32
	hostName    string
	hostsBefore map[uint32]string

	col *collector

	width, height int

	// dllCreate, when set, is the CreatePseudoConsole of a bundled
	// conpty.dll rather than the inbox one.
	viaBundledDLL bool

	strategy  spawnStrategy
	pumpErr   string
	pumpReads int
	closeHung bool
}

// collector accumulates everything the pseudoconsole emits, so a measurement
// can name the byte range that belongs to it. Every read is timestamped
// because "how long did the repaint take" is one of the questions.
type collector struct {
	mu        sync.Mutex
	buf       []byte
	lastRead  time.Time
	firstRead time.Time
	closed    bool
}

func (c *collector) append(p []byte) {
	c.mu.Lock()
	if c.firstRead.IsZero() {
		c.firstRead = time.Now()
	}
	c.buf = append(c.buf, p...)
	c.lastRead = time.Now()
	c.mu.Unlock()
}

// size is how the caller asks the question that matters most and that this
// probe originally could not ask: has anything arrived at all? A phase that
// times out with zero bytes is a dead session, not a slow one, and the two
// deserve completely different handling.
func (c *collector) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.buf)
}

func (c *collector) silent() bool { return c.size() == 0 }

// firstAt is when the first byte arrived, zero if none has.
func (c *collector) firstAt() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.firstRead
}

func (c *collector) mark() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.buf)
}

func (c *collector) since(off int) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if off > len(c.buf) {
		off = len(c.buf)
	}
	out := make([]byte, len(c.buf)-off)
	copy(out, c.buf[off:])
	return out
}

func (c *collector) idleFor(d time.Duration) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.lastRead.IsZero() {
		return false
	}
	return time.Since(c.lastRead) >= d
}

// waitForMarker blocks until the marker appears after off, or the deadline
// passes. It returns the byte offset just past the marker, or -1.
func (c *collector) waitForMarker(off int, marker string, timeout time.Duration) int {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		c.mu.Lock()
		idx := strings.Index(string(c.buf[min(off, len(c.buf)):]), marker)
		l := len(c.buf)
		c.mu.Unlock()
		if idx >= 0 {
			return off + idx + len(marker)
		}
		_ = l
		time.Sleep(5 * time.Millisecond)
	}
	return -1
}

// waitQuiet waits until nothing has arrived for `quiet`, i.e. the frame in
// flight has finished, or the deadline passes.
func (c *collector) waitQuiet(quiet, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.idleFor(quiet) {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// bundledCreate holds the bundled conpty.dll entry points when one is found
// next to the probe. Direction D3 begins with "does a bundled host work at
// all", so the whole ladder can be re-run against it with one flag.
type bundledHost struct {
	found   bool
	path    string
	version string
	create  *syscall.LazyProc
	resize  *syscall.LazyProc
	closefn *syscall.LazyProc
}

func findBundledHost() bundledHost {
	exe, err := os.Executable()
	if err != nil {
		return bundledHost{}
	}
	dir := filepath.Dir(exe)
	dllPath := filepath.Join(dir, "conpty.dll")
	if _, err := os.Stat(dllPath); err != nil {
		return bundledHost{}
	}
	dll := syscall.NewLazyDLL(dllPath)
	if err := dll.Load(); err != nil {
		return bundledHost{found: false, path: dllPath, version: "load failed: " + err.Error()}
	}
	return bundledHost{
		found:   true,
		path:    dllPath,
		version: fileVersion(dllPath),
		create:  dll.NewProc("CreatePseudoConsole"),
		resize:  dll.NewProc("ResizePseudoConsole"),
		closefn: dll.NewProc("ClosePseudoConsole"),
	}
}

func newSession(width, height int, childArgs []string, host bundledHost, useBundled bool) (*session, error) {
	return newSessionWith(chosenStrategy, width, height, childArgs, host, useBundled)
}

// chosenStrategy is what the smoke test settled on; the rounds use it.
var chosenStrategy = defaultStrategies()[0]

// createMu serialises the create-and-identify-the-host window only. Creation
// costs tens of milliseconds, so this does not undo the parallelism, but it
// does fix the attribution: with three sessions starting at once, the "which
// conhost is new" difference was being split between them and most rungs
// reported no host at all.
var createMu sync.Mutex

func newSessionWith(st spawnStrategy, width, height int, childArgs []string,
	host bundledHost, useBundled bool) (*session, error) {

	if err := procCreatePseudoConsole.Find(); err != nil {
		return nil, fmt.Errorf("CreatePseudoConsole is not available on this system: %w", err)
	}

	var sa *syscall.SecurityAttributes
	if st.Inheritable {
		sa = &syscall.SecurityAttributes{InheritHandle: 1}
		sa.Length = uint32(unsafe.Sizeof(*sa))
	}

	var inRead, inWrite, outRead, outWrite syscall.Handle
	if err := syscall.CreatePipe(&inRead, &inWrite, sa, st.PipeSize); err != nil {
		return nil, fmt.Errorf("CreatePipe(in): %w", err)
	}
	if err := syscall.CreatePipe(&outRead, &outWrite, sa, st.PipeSize); err != nil {
		syscall.CloseHandle(inRead)
		syscall.CloseHandle(inWrite)
		return nil, fmt.Errorf("CreatePipe(out): %w", err)
	}

	size := uint32(uint32(uint16(width)) | uint32(uint16(height))<<16)
	var hpc uintptr

	create := procCreatePseudoConsole
	viaDLL := false
	if useBundled && host.found {
		create = host.create
		viaDLL = true
	}

	createMu.Lock()
	hostsBefore := snapshotConsoleHosts()
	r, _, err := create.Call(uintptr(size), uintptr(inRead), uintptr(outWrite), 0, uintptr(unsafe.Pointer(&hpc)))
	if r == 0 {
		// Identify this session's host before anyone else creates one.
		defer createMu.Unlock()
	} else {
		createMu.Unlock()
	}
	if r != 0 {
		syscall.CloseHandle(inRead)
		syscall.CloseHandle(inWrite)
		syscall.CloseHandle(outRead)
		syscall.CloseHandle(outWrite)
		return nil, fmt.Errorf("CreatePseudoConsole(%dx%d) HRESULT 0x%08x (%v)", width, height, uint32(r), err)
	}

	s := &session{
		hpc: hpc, inWrite: inWrite, outRead: outRead,
		inRead: inRead, outWrite: outWrite,
		col: &collector{}, width: width, height: height,
		viaBundledDLL: viaDLL, strategy: st,
		hostsBefore: hostsBefore,
	}

	// Read before spawning. Microsoft's own guidance for ConPTY is that the
	// output pipe must be drained promptly: conhost writes its first frame
	// while holding a lock, and a parent that starts reading afterwards can
	// deadlock the host before the child has produced a byte. This probe's
	// first field run showed exactly that shape -- every wait timing out with
	// zero bytes received.
	if st.PumpBeforeSpawn {
		go s.pump()
	}

	if err := s.spawn(childArgs); err != nil {
		s.Close(host)
		return nil, err
	}

	if !st.PumpBeforeSpawn {
		go s.pump()
	}

	if st.CloseChildEnds {
		// The pseudoconsole duplicated these; holding them open also means the
		// reader never sees EOF when the child exits.
		if s.inRead != 0 {
			syscall.CloseHandle(s.inRead)
			s.inRead = 0
		}
		if s.outWrite != 0 {
			syscall.CloseHandle(s.outWrite)
			s.outWrite = 0
		}
	}

	return s, nil
}

func (s *session) spawn(args []string) error {
	var attrSize uintptr
	procInitAttrList.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrSize)))
	if attrSize == 0 {
		return fmt.Errorf("InitializeProcThreadAttributeList sized 0")
	}
	attrBuf := make([]byte, attrSize)
	attrList := uintptr(unsafe.Pointer(&attrBuf[0]))
	if r, _, err := procInitAttrList.Call(attrList, 1, 0, uintptr(unsafe.Pointer(&attrSize))); r == 0 {
		return fmt.Errorf("InitializeProcThreadAttributeList: %v", err)
	}
	defer procDeleteAttrList.Call(attrList)

	if r, _, err := procUpdateAttr.Call(attrList, 0,
		uintptr(procThreadAttributePseudoConsole), s.hpc,
		unsafe.Sizeof(s.hpc), 0, 0); r == 0 {
		return fmt.Errorf("UpdateProcThreadAttribute(PSEUDOCONSOLE): %v", err)
	}

	type startupInfoEx struct {
		syscall.StartupInfo
		AttributeList uintptr
	}
	si := startupInfoEx{}
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(si))
	si.AttributeList = attrList

	cmdline := buildCommandLine(args)
	argp, err := syscall.UTF16PtrFromString(cmdline)
	if err != nil {
		return err
	}

	flags := uint32(extendedStartupInfoPresent)
	if s.strategy.NoWindowFlag {
		flags |= createNoWindow
	}

	var pi syscall.ProcessInformation
	err = syscall.CreateProcess(nil, argp, nil, nil, false,
		flags, nil, nil, &si.StartupInfo, &pi)
	if err != nil {
		return fmt.Errorf("CreateProcess(%s): %w", cmdline, err)
	}
	syscall.CloseHandle(pi.Thread)
	s.childProc = pi.Process
	s.childPID = pi.ProcessId

	// The pseudoconsole's own conhost/OpenConsole is a new process; find the
	// one that was not there before. Matching only on "a conhost child of
	// ours" reported the probe's own console host instead, identically for
	// every session, which is how the first field log came to name pid 1776
	// twice.
	s.hostPID, s.hostName = newConsoleHost(s.hostsBefore)
	return nil
}

func buildCommandLine(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			quoted = append(quoted, `"`+strings.ReplaceAll(a, `"`, `\"`)+`"`)
		} else {
			quoted = append(quoted, a)
		}
	}
	return strings.Join(quoted, " ")
}

func (s *session) pump() {
	buf := make([]byte, 64*1024)
	for {
		var n uint32
		err := syscall.ReadFile(s.outRead, buf, &n, nil)
		s.pumpReads++
		if err != nil || n == 0 {
			s.col.mu.Lock()
			s.col.closed = true
			s.col.mu.Unlock()
			if err != nil {
				s.pumpErr = err.Error()
			} else {
				s.pumpErr = "EOF"
			}
			return
		}
		s.col.append(buf[:n])
	}
}

// readerStatus describes what the output reader did, so a silent session can
// say whether nothing was sent or nothing was read.
func (s *session) readerStatus() string {
	if s.pumpErr == "" {
		return fmt.Sprintf("reader active after %d read(s)", s.pumpReads)
	}
	return fmt.Sprintf("reader stopped after %d read(s): %s", s.pumpReads, s.pumpErr)
}

func (s *session) writeInput(text string) {
	b := []byte(text)
	var n uint32
	syscall.WriteFile(s.inWrite, b, &n, nil)
}

func (s *session) resize(width, height int, host bundledHost) error {
	size := uint32(uint32(uint16(width)) | uint32(uint16(height))<<16)
	resize := procResizePseudoConsole
	if s.viaBundledDLL && host.found {
		resize = host.resize
	}
	r, _, err := resize.Call(s.hpc, uintptr(size))
	if r != 0 {
		return fmt.Errorf("ResizePseudoConsole(%dx%d) HRESULT 0x%08x (%v)", width, height, uint32(r), err)
	}
	s.width, s.height = width, height
	return nil
}

// CloseGuarded tears the session down but never blocks for longer than the
// given budget. ClosePseudoConsole is a blocking wait on this era of Windows
// (it became non-blocking only in build 26100), and a session whose pipe ends
// were left open can make that wait indefinite -- which is exactly how the
// probe hung after answering its own question. A leaked handle in a probe is
// cheaper than a hang, so a close that does not return is abandoned and
// reported.
func (s *session) CloseGuarded(host bundledHost, budget time.Duration) string {
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.Close(host)
	}()
	select {
	case <-done:
		return ""
	case <-time.After(budget):
		return fmt.Sprintf("ClosePseudoConsole did not return within %s; session abandoned", budget)
	}
}

// Close tears the session down without ever being able to block the run.
//
// ClosePseudoConsole waits for the host to exit on builds before 26100, and a
// field run of this probe lost a minute to exactly that: a strategy that kept
// our copy of the output pipe open meant the host could never finish, so the
// close never returned. It is now a supervised step like any other, and a
// close that does not come back is abandoned with the handles leaked
// deliberately -- a leaked handle costs nothing in a probe that is about to
// exit, while a blocked close costs the whole run.
func (s *session) Close(host bundledHost) stepResult {
	if s.childProc != 0 {
		syscall.TerminateProcess(s.childProc, 0)
		syscall.CloseHandle(s.childProc)
		s.childProc = 0
	}

	res := stepResult{Name: "close", Outcome: stepOK}
	if s.hpc != 0 {
		closefn := procClosePseudoConsole
		if s.viaBundledDLL && host.found {
			closefn = host.closefn
		}
		hpc := s.hpc
		s.hpc = 0
		res = withTimeout("ClosePseudoConsole", 3*time.Second, func() (string, error) {
			closefn.Call(hpc)
			return "", nil
		})
		if res.Outcome == stepHung {
			s.closeHung = true
			// The pipe handles stay open on purpose: the abandoned goroutine
			// may still be inside the call and closing under it would be
			// worse than leaking.
			return res
		}
	}

	for _, h := range []*syscall.Handle{&s.inWrite, &s.outRead, &s.inRead, &s.outWrite} {
		if *h != 0 {
			syscall.CloseHandle(*h)
			*h = 0
		}
	}
	return res
}

// childStatus reports whether the spawned child is still running, and its
// exit code if it is not. A silent session with a child that exited with a
// non-zero code is a different diagnosis from one whose child is alive and
// blocked.
// hostDescription names the conhost/OpenConsole serving this session, when
// one could be identified.
func (s *session) hostDescription() string {
	if s.hostPID == 0 {
		return "not identified"
	}
	return fmt.Sprintf("%s pid %d, %dKB", s.hostName, s.hostPID, workingSetKB(s.hostPID))
}

func (s *session) childStatus() string {
	if s.childProc == 0 {
		return "no child handle"
	}
	var code uint32
	if err := syscall.GetExitCodeProcess(s.childProc, &code); err != nil {
		return "GetExitCodeProcess failed: " + err.Error()
	}
	const stillActive = 259
	if code == stillActive {
		return fmt.Sprintf("pid %d still running", s.childPID)
	}
	return fmt.Sprintf("pid %d exited with code %d", s.childPID, code)
}

// ---------------------------------------------------------------------------
// Host process discovery and memory
// ---------------------------------------------------------------------------

type processEntry32 struct {
	Size            uint32
	Usage           uint32
	ProcessID       uint32
	DefaultHeapID   uintptr
	ModuleID        uint32
	Threads         uint32
	ParentProcessID uint32
	PriClassBase    int32
	Flags           uint32
	ExeFile         [260]uint16
}

// snapshotConsoleHosts records every console host alive right now, so the one
// created for the next session can be identified by difference.
func snapshotConsoleHosts() map[uint32]string {
	out := map[uint32]string{}
	snap, _, _ := procCreateToolhelp32.Call(uintptr(th32csSnapProcess), 0)
	if snap == uintptr(syscall.InvalidHandle) || snap == 0 {
		return out
	}
	defer syscall.CloseHandle(syscall.Handle(snap))
	var e processEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	ret, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&e)))
	for ret != 0 {
		name := strings.ToLower(syscall.UTF16ToString(e.ExeFile[:]))
		if name == "conhost.exe" || name == "openconsole.exe" {
			out[e.ProcessID] = name
		}
		ret, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&e)))
	}
	return out
}

func newConsoleHost(before map[uint32]string) (uint32, string) {
	for pid, name := range snapshotConsoleHosts() {
		if _, existed := before[pid]; !existed {
			return pid, name
		}
	}
	return 0, ""
}

type processMemoryCounters struct {
	Cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

func workingSetKB(pid uint32) int64 {
	if pid == 0 {
		return -1
	}
	h, err := syscall.OpenProcess(processQueryLimitedInformation|processVMRead, false, pid)
	if err != nil {
		return -1
	}
	defer syscall.CloseHandle(h)
	var pmc processMemoryCounters
	pmc.Cb = uint32(unsafe.Sizeof(pmc))
	r, _, _ := procGetProcessMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.Cb))
	if r == 0 {
		return -1
	}
	return int64(pmc.WorkingSetSize / 1024)
}

// ---------------------------------------------------------------------------
// Build and binary versions -- the answers must say which build they are about
// ---------------------------------------------------------------------------

type osVersionInfoExW struct {
	OSVersionInfoSize uint32
	MajorVersion      uint32
	MinorVersion      uint32
	BuildNumber       uint32
	PlatformId        uint32
	CSDVersion        [128]uint16
	ServicePackMajor  uint16
	ServicePackMinor  uint16
	SuiteMask         uint16
	ProductType       uint8
	Reserved          uint8
}

func windowsBuild() string {
	var v osVersionInfoExW
	v.OSVersionInfoSize = uint32(unsafe.Sizeof(v))
	procRtlGetVersion.Call(uintptr(unsafe.Pointer(&v)))
	return fmt.Sprintf("%d.%d.%d", v.MajorVersion, v.MinorVersion, v.BuildNumber)
}

type vsFixedFileInfo struct {
	Signature        uint32
	StrucVersion     uint32
	FileVersionMS    uint32
	FileVersionLS    uint32
	ProductVersionMS uint32
	ProductVersionLS uint32
	FileFlagsMask    uint32
	FileFlags        uint32
	FileOS           uint32
	FileType         uint32
	FileSubtype      uint32
	FileDateMS       uint32
	FileDateLS       uint32
}

func fileVersion(path string) string {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return "?"
	}
	size, _, _ := procGetFileVersionInfoSizeW.Call(uintptr(unsafe.Pointer(p)), 0)
	if size == 0 {
		return "no version resource"
	}
	buf := make([]byte, size)
	if r, _, _ := procGetFileVersionInfoW.Call(uintptr(unsafe.Pointer(p)), 0, size, uintptr(unsafe.Pointer(&buf[0]))); r == 0 {
		return "unreadable"
	}
	sub, _ := syscall.UTF16PtrFromString(`\`)
	var info *vsFixedFileInfo
	var l uint32
	if r, _, _ := procVerQueryValueW.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(sub)),
		uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&l))); r == 0 || info == nil {
		return "no fixed info"
	}
	return fmt.Sprintf("%d.%d.%d.%d",
		info.FileVersionMS>>16, info.FileVersionMS&0xffff,
		info.FileVersionLS>>16, info.FileVersionLS&0xffff)
}

func systemFile(name string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", name)
}
