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
	createNoWindow                   = 0x08000000

	th32csSnapProcess = 0x00000002

	processQueryLimitedInformation = 0x1000
	processVMRead                  = 0x0010
)

// ---------------------------------------------------------------------------
// A pseudoconsole we own, and the child inside it
// ---------------------------------------------------------------------------

type session struct {
	hpc     uintptr
	inWrite syscall.Handle
	outRead syscall.Handle

	inRead   syscall.Handle
	outWrite syscall.Handle

	childProc syscall.Handle
	childPID  uint32

	hostPID  uint32
	hostName string

	col *collector

	width, height int

	// dllCreate, when set, is the CreatePseudoConsole of a bundled
	// conpty.dll rather than the inbox one.
	viaBundledDLL bool
}

// collector accumulates everything the pseudoconsole emits, so a measurement
// can name the byte range that belongs to it. Every read is timestamped
// because "how long did the repaint take" is one of the questions.
type collector struct {
	mu       sync.Mutex
	buf      []byte
	lastRead time.Time
	closed   bool
}

func (c *collector) append(p []byte) {
	c.mu.Lock()
	c.buf = append(c.buf, p...)
	c.lastRead = time.Now()
	c.mu.Unlock()
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
	var inRead, inWrite, outRead, outWrite syscall.Handle
	if err := syscall.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return nil, fmt.Errorf("CreatePipe(in): %w", err)
	}
	if err := syscall.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
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
	r, _, err := create.Call(uintptr(size), uintptr(inRead), uintptr(outWrite), 0, uintptr(unsafe.Pointer(&hpc)))
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
		viaBundledDLL: viaDLL,
	}

	if err := s.spawn(childArgs); err != nil {
		s.Close(host)
		return nil, err
	}

	go s.pump()
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

	var pi syscall.ProcessInformation
	err = syscall.CreateProcess(nil, argp, nil, nil, false,
		extendedStartupInfoPresent|createNoWindow,
		nil, nil, &si.StartupInfo, &pi)
	if err != nil {
		return fmt.Errorf("CreateProcess(%s): %w", cmdline, err)
	}
	syscall.CloseHandle(pi.Thread)
	s.childProc = pi.Process
	s.childPID = pi.ProcessId

	// The pseudoconsole's own conhost/OpenConsole is our child too; find it so
	// its memory can be reported per height.
	s.hostPID, s.hostName = findConsoleHost(uint32(os.Getpid()))
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
		if err != nil || n == 0 {
			s.col.mu.Lock()
			s.col.closed = true
			s.col.mu.Unlock()
			return
		}
		s.col.append(buf[:n])
	}
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

func (s *session) Close(host bundledHost) {
	if s.childProc != 0 {
		syscall.TerminateProcess(s.childProc, 0)
		syscall.CloseHandle(s.childProc)
		s.childProc = 0
	}
	if s.hpc != 0 {
		closefn := procClosePseudoConsole
		if s.viaBundledDLL && host.found {
			closefn = host.closefn
		}
		closefn.Call(s.hpc)
		s.hpc = 0
	}
	for _, h := range []*syscall.Handle{&s.inWrite, &s.outRead, &s.inRead, &s.outWrite} {
		if *h != 0 {
			syscall.CloseHandle(*h)
			*h = 0
		}
	}
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

func findConsoleHost(parentPID uint32) (uint32, string) {
	snap, _, _ := procCreateToolhelp32.Call(uintptr(th32csSnapProcess), 0)
	if snap == uintptr(syscall.InvalidHandle) || snap == 0 {
		return 0, ""
	}
	defer syscall.CloseHandle(syscall.Handle(snap))

	var e processEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	ret, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&e)))
	for ret != 0 {
		name := syscall.UTF16ToString(e.ExeFile[:])
		lower := strings.ToLower(name)
		if e.ParentProcessID == parentPID && (lower == "conhost.exe" || lower == "openconsole.exe") {
			return e.ProcessID, name
		}
		ret, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&e)))
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
