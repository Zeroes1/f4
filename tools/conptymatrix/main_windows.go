//go:build windows

// conptymatrix sweeps a grid rather than a list of questions.
//
// The probes before it each covered whichever questions their author had in
// mind that day, and each time the interesting one turned out to be the one
// nobody had thought to ask -- the constant term that was really a content
// term, the buffer that was never full, the line that was never exactly the
// width of the console. A list of questions cannot be complete because it is
// written by the same person who is missing something. A grid can: name the
// axes, take the extremes of each, run the product.
//
// The axes:
//
//	fill      empty | small | full | overflow      (does the buffer hold, and does it evict)
//	height    500 | 2000 | 32000                   (does anything change with depth)
//	width     narrow-by-one | wide | restore | repeated | during-output
//	shape     short | exactly-width | longer | double-width | coloured
//	child     our emitter | cmd.exe | alternate screen | size-querying
//	product   4000 columns against every height    (where the host wedges)
//
// Every line shape appears in every session, so the shape axis costs nothing.
// Fill, height and child are real sessions. The product sweep is its own pass.
//
// Like conptydump, this decides nothing while running: fixed schedules, a
// reader with no timeout, raw bytes to a file. The summary is computed
// afterwards from those bytes, where a mistake costs a re-read rather than
// another run on someone's machine.
//
// The ConPTY calls follow Microsoft's EchoCon sample and wezterm's
// psuedocon.rs where the two agree.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procCreatePseudoConsole = kernel32.NewProc("CreatePseudoConsole")
	procResizePseudoConsole = kernel32.NewProc("ResizePseudoConsole")
	procClosePseudoConsole  = kernel32.NewProc("ClosePseudoConsole")
	procInitAttrList        = kernel32.NewProc("InitializeProcThreadAttributeList")
	procUpdateAttr          = kernel32.NewProc("UpdateProcThreadAttribute")
	procDeleteAttrList      = kernel32.NewProc("DeleteProcThreadAttributeList")
	procCreateToolhelp32    = kernel32.NewProc("CreateToolhelp32Snapshot")
	procProcess32FirstW     = kernel32.NewProc("Process32FirstW")
	procProcess32NextW      = kernel32.NewProc("Process32NextW")

	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procGetStdHandle               = kernel32.NewProc("GetStdHandle")
)

const (
	procThreadAttributePseudoConsole = 0x00020016
	extendedStartupInfoPresent       = 0x00080000
	startfUseStdHandles              = 0x00000100
	th32csSnapProcess                = 0x00000002

	stdOutputHandle                 = ^uintptr(10)
	enableVirtualTerminalProcessing = 0x0004
)

func packCoord(x, y int) uintptr {
	return uintptr(uint32(uint16(int16(x))) | uint32(uint16(int16(y)))<<16)
}

// ---------------------------------------------------------------------------
// capture
// ---------------------------------------------------------------------------

type capture struct {
	mu     sync.Mutex
	raw    []byte
	marks  []mark
	start  time.Time
	dumpF  *os.File
	dumped int
	cap    int
}

type mark struct {
	at     time.Duration
	off    int
	text   string
	isCall bool
}

func newCapture(dumpPath string, capBytes int) (*capture, error) {
	f, err := os.Create(dumpPath)
	if err != nil {
		return nil, err
	}
	return &capture{start: time.Now(), dumpF: f, cap: capBytes}, nil
}

func (c *capture) event(format string, a ...any) {
	txt := fmt.Sprintf(format, a...)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.marks = append(c.marks, mark{at: time.Since(c.start), off: len(c.raw), text: txt, isCall: true})
	fmt.Fprintf(c.dumpF, "\n@@ %7dms EVENT %s\n", time.Since(c.start).Milliseconds(), txt)
}

func (c *capture) chunk(b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.marks = append(c.marks, mark{at: time.Since(c.start), off: len(c.raw), text: "chunk"})
	c.raw = append(c.raw, b...)
	if c.dumped < c.cap {
		n := len(b)
		if c.dumped+n > c.cap {
			n = c.cap - c.dumped
		}
		fmt.Fprintf(c.dumpF, "\n@@ %7dms CHUNK %d bytes (total %d)\n", time.Since(c.start).Milliseconds(), len(b), len(c.raw))
		c.dumpF.WriteString(escape(b[:n]))
		c.dumpF.WriteString("\n")
		c.dumped += n
		if c.dumped >= c.cap {
			fmt.Fprintf(c.dumpF, "\n@@ DUMP TRUNCATED at %d bytes; counts in the summary use the full stream\n", c.cap)
		}
	}
}

// segment returns the bytes that arrived after the given event text.
func (c *capture) segment(eventPrefix string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	startOff := -1
	endOff := len(c.raw)
	for i, m := range c.marks {
		if m.isCall && strings.HasPrefix(m.text, eventPrefix) && startOff < 0 {
			startOff = m.off
			for _, later := range c.marks[i+1:] {
				if later.isCall && strings.HasPrefix(later.text, "RESIZE") {
					endOff = later.off
					break
				}
			}
			break
		}
	}
	if startOff < 0 {
		return nil
	}
	out := make([]byte, endOff-startOff)
	copy(out, c.raw[startOff:endOff])
	return out
}

// firstChunkAfter is the latency from an event to the first byte that followed.
func (c *capture) firstChunkAfter(eventPrefix string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	seen := false
	var t0 time.Duration
	for _, m := range c.marks {
		if m.isCall && strings.HasPrefix(m.text, eventPrefix) {
			seen = true
			t0 = m.at
			continue
		}
		if seen && !m.isCall {
			return m.at - t0
		}
	}
	return -1
}

func (c *capture) size() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.raw)
}

func (c *capture) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	fmt.Fprintf(c.dumpF, "\n@@ %7dms END total %d bytes\n", time.Since(c.start).Milliseconds(), len(c.raw))
	c.dumpF.Close()
}

func escape(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b) + len(b)/4)
	for _, ch := range b {
		switch {
		case ch == 0x1b:
			sb.WriteString("\\e")
		case ch == '\r':
			sb.WriteString("\\r")
		case ch == '\n':
			sb.WriteString("\\n\n")
		case ch == '\\':
			sb.WriteString("\\\\")
		case ch >= 0x20 && ch < 0x7f:
			sb.WriteByte(ch)
		default:
			fmt.Fprintf(&sb, "\\x%02x", ch)
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// session
// ---------------------------------------------------------------------------

type session struct {
	hpc     uintptr
	ourIn   syscall.Handle
	ourOut  syscall.Handle
	child   syscall.Handle
	childID uint32
	hostID  uint32
	cap     *capture
	done    chan struct{}
}

func newSession(width, height int, argv []string, cap *capture) (*session, error) {
	var ptyIn, ourOut, ourIn, ptyOut syscall.Handle
	if err := syscall.CreatePipe(&ptyIn, &ourOut, nil, 0); err != nil {
		return nil, fmt.Errorf("CreatePipe(in): %w", err)
	}
	if err := syscall.CreatePipe(&ourIn, &ptyOut, nil, 0); err != nil {
		syscall.CloseHandle(ptyIn)
		syscall.CloseHandle(ourOut)
		return nil, fmt.Errorf("CreatePipe(out): %w", err)
	}

	hostsBefore := consoleHosts()

	var hpc uintptr
	r, _, e := procCreatePseudoConsole.Call(packCoord(width, height),
		uintptr(ptyIn), uintptr(ptyOut), 0, uintptr(unsafe.Pointer(&hpc)))
	// EchoCon.cpp closes both pty-side handles here regardless.
	syscall.CloseHandle(ptyIn)
	syscall.CloseHandle(ptyOut)
	if r != 0 {
		syscall.CloseHandle(ourIn)
		syscall.CloseHandle(ourOut)
		return nil, fmt.Errorf("CreatePseudoConsole(%dx%d) HRESULT 0x%08x (%v)", width, height, uint32(r), e)
	}

	s := &session{hpc: hpc, ourIn: ourIn, ourOut: ourOut, cap: cap, done: make(chan struct{})}

	go func() {
		defer close(s.done)
		buf := make([]byte, 64*1024)
		for {
			var n uint32
			err := syscall.ReadFile(s.ourIn, buf, &n, nil)
			if n > 0 {
				cap.chunk(buf[:n])
			}
			if err != nil || n == 0 {
				return
			}
		}
	}()

	if err := s.spawn(argv); err != nil {
		s.kill()
		return nil, err
	}
	s.hostID = newHost(hostsBefore)
	return s, nil
}

func (s *session) resize(w, h int) error {
	r, _, e := procResizePseudoConsole.Call(s.hpc, packCoord(w, h))
	if r != 0 {
		return fmt.Errorf("HRESULT 0x%08x (%v)", uint32(r), e)
	}
	return nil
}

// kill tears the session down without being able to block. ClosePseudoConsole
// waits for the host on this build and has been observed never to return after
// a 128-million-cell resize, so it runs on its own goroutine and the host is
// terminated directly if it does not come back.
func (s *session) kill() bool {
	if s.child != 0 {
		syscall.TerminateProcess(s.child, 0)
		syscall.CloseHandle(s.child)
		s.child = 0
	}
	closed := make(chan struct{})
	if s.hpc != 0 {
		hpc := s.hpc
		s.hpc = 0
		go func() { procClosePseudoConsole.Call(hpc); close(closed) }()
	} else {
		close(closed)
	}
	clean := true
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		clean = false
		if s.hostID != 0 {
			if h, err := syscall.OpenProcess(syscall.PROCESS_TERMINATE, false, s.hostID); err == nil {
				syscall.TerminateProcess(h, 1)
				syscall.CloseHandle(h)
			}
		}
	}
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
	if s.ourIn != 0 {
		syscall.CloseHandle(s.ourIn)
		s.ourIn = 0
	}
	if s.ourOut != 0 {
		syscall.CloseHandle(s.ourOut)
		s.ourOut = 0
	}
	return clean
}

func (s *session) spawn(argv []string) error {
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
		uintptr(procThreadAttributePseudoConsole), s.hpc, unsafe.Sizeof(s.hpc), 0, 0); r == 0 {
		return fmt.Errorf("UpdateProcThreadAttribute: %v", err)
	}

	type startupInfoEx struct {
		syscall.StartupInfo
		AttributeList uintptr
	}
	var si startupInfoEx
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(si))
	si.StartupInfo.Flags = startfUseStdHandles
	si.StartupInfo.StdInput = syscall.InvalidHandle
	si.StartupInfo.StdOutput = syscall.InvalidHandle
	si.StartupInfo.StdErr = syscall.InvalidHandle
	si.AttributeList = attrList

	argp, err := syscall.UTF16PtrFromString(commandLine(argv))
	if err != nil {
		return err
	}
	var pi syscall.ProcessInformation
	if err := syscall.CreateProcess(nil, argp, nil, nil, false,
		extendedStartupInfoPresent, nil, nil, &si.StartupInfo, &pi); err != nil {
		return fmt.Errorf("CreateProcess: %w", err)
	}
	syscall.CloseHandle(pi.Thread)
	s.child, s.childID = pi.Process, pi.ProcessId
	return nil
}

func commandLine(args []string) string {
	out := make([]string, 0, len(args))
	for _, a := range args {
		if strings.ContainsAny(a, " \t\"") {
			out = append(out, `"`+strings.ReplaceAll(a, `"`, `\"`)+`"`)
		} else {
			out = append(out, a)
		}
	}
	return strings.Join(out, " ")
}

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

func consoleHosts() map[uint32]bool {
	out := map[uint32]bool{}
	snap, _, _ := procCreateToolhelp32.Call(uintptr(th32csSnapProcess), 0)
	if snap == 0 || snap == uintptr(syscall.InvalidHandle) {
		return out
	}
	defer syscall.CloseHandle(syscall.Handle(snap))
	var e processEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	ret, _, _ := procProcess32FirstW.Call(snap, uintptr(unsafe.Pointer(&e)))
	for ret != 0 {
		n := strings.ToLower(syscall.UTF16ToString(e.ExeFile[:]))
		if n == "conhost.exe" || n == "openconsole.exe" {
			out[e.ProcessID] = true
		}
		ret, _, _ = procProcess32NextW.Call(snap, uintptr(unsafe.Pointer(&e)))
	}
	return out
}

func newHost(before map[uint32]bool) uint32 {
	for pid := range consoleHosts() {
		if !before[pid] {
			return pid
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// mechanical analysis of a captured frame
// ---------------------------------------------------------------------------

var (
	markerRE  = regexp.MustCompile(`~F(\d{7})~`)
	sizeRepRE = regexp.MustCompile(`\x1b\[8;(\d+);(\d+)t`)
)

type frameFacts struct {
	Bytes     int
	Latency   time.Duration
	Markers   int
	Lowest    int
	Highest   int
	SizeRows  int
	SizeCols  int
	AtHome    bool
	HidesCur  bool
	AltEnter  bool
	AltLeave  bool
	EraseEOL  int  // count of ESC[K CR LF, i.e. logical line terminators
	LongWhole bool // the over-wide line arrived with no break inside it
	ExactAmb  string
}

func analyse(b []byte) frameFacts {
	f := frameFacts{Bytes: len(b)}
	s := string(b)
	f.HidesCur = strings.Contains(s, "\x1b[?25l")
	f.AltEnter = strings.Contains(s, "\x1b[?1049h")
	f.AltLeave = strings.Contains(s, "\x1b[?1049l")
	f.EraseEOL = strings.Count(s, "\x1b[K\r\n")
	if m := sizeRepRE.FindStringSubmatch(s); m != nil {
		f.SizeRows, _ = strconv.Atoi(m[1])
		f.SizeCols, _ = strconv.Atoi(m[2])
	}
	if i := strings.Index(s, "\x1b["); i >= 0 {
		rest := s[i:]
		for _, cand := range []string{"\x1b[H", "\x1b[1;1H"} {
			if strings.HasPrefix(rest, cand) {
				f.AtHome = true
			}
		}
		if !f.AtHome {
			// The first cursor-positioning of the frame, whatever it is.
			if j := strings.Index(s, "\x1b[H"); j >= 0 && j < 64 {
				f.AtHome = true
			}
		}
	}

	seen := map[int]bool{}
	lo, hi := -1, -1
	for _, m := range markerRE.FindAllStringSubmatch(s, -1) {
		n, _ := strconv.Atoi(m[1])
		if !seen[n] {
			seen[n] = true
			if lo < 0 || n < lo {
				lo = n
			}
			if n > hi {
				hi = n
			}
		}
	}
	f.Markers, f.Lowest, f.Highest = len(seen), lo, hi

	// The over-wide line: did it arrive as one run?
	if i := strings.Index(s, markerLongStart); i >= 0 {
		if j := strings.Index(s[i:], markerLongEnd); j > 0 {
			body := s[i+len(markerLongStart) : i+j]
			f.LongWhole = !strings.ContainsAny(body, "\r\n\x1b")
		}
	}

	// The exactly-width line: is it followed by ESC[K (a hard break) or not
	// (indistinguishable from a wrap)? This is P13, the one ambiguity the
	// stream cannot resolve, and it is recorded rather than judged.
	if i := strings.Index(s, markerExact); i >= 0 {
		tail := s[i:]
		if k := strings.Index(tail, "\x1b[K"); k >= 0 && k < 200 {
			f.ExactAmb = "followed by ESC[K within " + strconv.Itoa(k) + " bytes"
		} else {
			f.ExactAmb = "no ESC[K after it (indistinguishable from a wrap)"
		}
	} else {
		f.ExactAmb = "not present"
	}
	return f
}

func (f frameFacts) line() string {
	return fmt.Sprintf("%7dB %6dms  lines=%-6d range=[%d..%d]  term=%-6d size=%dx%d home=%-5v hide=%-5v longWhole=%-5v alt=%v/%v",
		f.Bytes, f.Latency.Milliseconds(), f.Markers, f.Lowest, f.Highest,
		f.EraseEOL, f.SizeRows, f.SizeCols, f.AtHome, f.HidesCur, f.LongWhole, f.AltEnter, f.AltLeave)
}

// ---------------------------------------------------------------------------
// markers the child prints
// ---------------------------------------------------------------------------

const (
	markerLongStart = "~L1~"
	markerLongEnd   = "~E1~"
	markerExact     = "~EXACT~"
	markerWide      = "~WIDE~"
	markerColour    = "~COLOUR~"
	markerDone      = "~DONE~"
	markerAltIn     = "~ALTIN~"
)

func fillMarker(n int) string { return fmt.Sprintf("~F%07d~", n) }

// ---------------------------------------------------------------------------
// main: the grid
// ---------------------------------------------------------------------------

func main() {
	var (
		emit    = flag.String("emit", "", "internal: child mode")
		lines   = flag.Int("lines", 0, "internal: history lines for the child")
		width   = flag.Int("width", 120, "console width")
		wide    = flag.Int("wide", 4000, "wide width for the rejoin pass")
		outDir  = flag.String("out", ".", "directory for the dumps")
		step    = flag.Duration("step", 1500*time.Millisecond, "delay between scheduled actions")
		dumpCap = flag.Int("dump-cap", 4<<20, "max bytes written per dump file")
		only    = flag.String("only", "", "run only cases whose name contains this")
	)
	flag.Parse()

	if *emit != "" {
		runChild(*emit, *lines, *width)
		return
	}

	logPath := filepath.Join(*outDir, fmt.Sprintf("conptymatrix-%d.log", os.Getpid()))
	lf, err := os.Create(logPath)
	if err != nil {
		fmt.Println("cannot create the log:", err)
		os.Exit(2)
	}
	defer lf.Close()
	say := func(format string, a ...any) {
		line := fmt.Sprintf(format, a...)
		fmt.Fprintln(lf, line)
		fmt.Println(line)
		lf.Sync()
	}

	self, err := os.Executable()
	if err != nil {
		say("cannot find own path: %v", err)
		return
	}

	say("conptymatrix -- a grid, not a list of questions")
	say("started %s", time.Now().Format(time.RFC3339))
	say("axes: fill x height x width-op x line-shape x child; plus a 4000-column product sweep")
	say("")

	heights := []int{500, 2000, 32000}
	fills := []struct {
		name  string
		lines func(h int) int
	}{
		{"empty", func(int) int { return 0 }},
		{"small", func(int) int { return 150 }},
		{"full", func(h int) int { return h - 10 }},
		{"overflow", func(h int) int { return h + h/2 }},
	}

	for _, h := range heights {
		for _, fl := range fills {
			name := fmt.Sprintf("fill=%s/height=%d", fl.name, h)
			if *only != "" && !strings.Contains(name, *only) {
				continue
			}
			n := fl.lines(h)
			// A 48000-line child into a 32000-row console is minutes of
			// output; the overflow case only needs to exceed the buffer.
			if n > 40000 {
				n = 40000
			}
			runCase(say, self, *outDir, name, h, *width, *wide, n, "lines", *step, *dumpCap)
		}
	}

	// The child axis: cases that are about who is writing, not how much.
	for _, c := range []struct{ name, mode string }{
		{"child=alt", "alt"},
		{"child=size", "size"},
	} {
		if *only != "" && !strings.Contains(*only, "child") && !strings.Contains(c.name, *only) {
			continue
		}
		runCase(say, self, *outDir, c.name, 2000, *width, *wide, 40, c.mode, *step, *dumpCap)
	}

	if *only == "" || strings.Contains("child=cmd", *only) {
		runCmdCase(say, *outDir, 2000, *width, *wide, *step, *dumpCap)
	}

	if *only == "" || strings.Contains("product", *only) {
		productSweep(say, self, *outDir, *width, *wide, *step, *dumpCap)
	}

	say("")
	say("log written to %s; dumps are conptymatrix-<case>.txt in %s", logPath, *outDir)
}

func caseFile(dir, name string) string {
	safe := strings.NewReplacer("/", "_", "=", "-", " ", "_").Replace(name)
	return filepath.Join(dir, "conptymatrix-"+safe+".txt")
}

func runCase(say func(string, ...any), self, dir, name string,
	height, width, wide, lines int, mode string, step time.Duration, dumpCap int) {

	cap, err := newCapture(caseFile(dir, name), dumpCap)
	if err != nil {
		say("%-28s dump file failed: %v", name, err)
		return
	}
	defer cap.close()

	cap.event("case %s height=%d width=%d lines=%d mode=%s", name, height, width, lines, mode)
	argv := []string{self, "-emit", mode, "-lines", strconv.Itoa(lines), "-width", strconv.Itoa(width)}

	s, err := newSession(width, height, argv, cap)
	if err != nil {
		say("%-28s create failed: %v", name, err)
		return
	}

	// A big history takes real time to print; the schedule scales with it
	// rather than guessing, and nothing waits on a condition.
	settle := step + time.Duration(lines/200)*time.Second
	time.Sleep(settle)
	initial := analyse(cap.segment("case "))

	ops := []struct {
		label string
		w     int
	}{
		{"narrow", width - 1},
		{"wide", wide},
		{"restore", width},
		{"narrow-again", width - 1},
		{"restore-again", width},
	}
	facts := map[string]frameFacts{}
	for _, op := range ops {
		cap.event("RESIZE %s -> %dx%d", op.label, op.w, height)
		if err := s.resize(op.w, height); err != nil {
			cap.event("RESIZE %s FAILED: %v", op.label, err)
			say("%-28s resize %s failed: %v", name, op.label, err)
			continue
		}
		time.Sleep(step)
		f := analyse(cap.segment("RESIZE " + op.label))
		f.Latency = cap.firstChunkAfter("RESIZE " + op.label)
		facts[op.label] = f
	}

	// A resize while the child is still writing: the interleaving case.
	cap.event("RESIZE during-output -> %dx%d", width-1, height)
	s.resize(width-1, height)
	time.Sleep(step)
	during := analyse(cap.segment("RESIZE during-output"))

	clean := s.kill()

	say("%-28s host=%d initial %s", name, s.hostID, initial.line())
	for _, op := range ops {
		if f, ok := facts[op.label]; ok {
			say("%-28s   %-14s %s", "", op.label, f.line())
		}
	}
	say("%-28s   %-14s %s", "", "during-output", during.line())
	say("%-28s   exact-width line in the narrow frame: %s", "", facts["narrow"].ExactAmb)
	if !clean {
		say("%-28s   ClosePseudoConsole did NOT return; host terminated", "")
	}
	if lines > 0 {
		lost := facts["narrow"].Lowest
		say("%-28s   oldest line still present after reflow: %d of %d printed", "", lost, lines)
	}
	say("")
}

func runCmdCase(say func(string, ...any), dir string, height, width, wide int, step time.Duration, dumpCap int) {
	name := "child=cmd"
	cap, err := newCapture(caseFile(dir, name), dumpCap)
	if err != nil {
		return
	}
	defer cap.close()
	cap.event("case %s height=%d width=%d", name, height, width)
	argv := []string{"cmd.exe", "/d", "/c", "dir", "/w", "/-p", `%SystemRoot%\System32`}
	s, err := newSession(width, height, argv, cap)
	if err != nil {
		say("%-28s create failed: %v", name, err)
		return
	}
	time.Sleep(step * 2)
	initial := analyse(cap.segment("case "))
	cap.event("RESIZE narrow -> %dx%d", width-1, height)
	s.resize(width-1, height)
	time.Sleep(step)
	narrow := analyse(cap.segment("RESIZE narrow"))
	cap.event("RESIZE wide -> %dx%d", wide, height)
	s.resize(wide, height)
	time.Sleep(step)
	wideF := analyse(cap.segment("RESIZE wide"))
	clean := s.kill()
	say("%-28s initial %s", name, initial.line())
	say("%-28s   narrow %s", "", narrow.line())
	say("%-28s   wide   %s", "", wideF.line())
	if !clean {
		say("%-28s   ClosePseudoConsole did NOT return; host terminated", "")
	}
	say("")
}

// productSweep finds where a 4000-column resize stops working. 4000x32000
// wedged the host in an earlier dump: S_OK, then no bytes, no EOF, and a
// ClosePseudoConsole that never returned. This walks the heights to locate the
// boundary rather than leaving it as "somewhere between 8M and 128M cells".
func productSweep(say func(string, ...any), self, dir string, width, wide int, step time.Duration, dumpCap int) {
	say("product sweep: %d columns against each height", wide)
	for _, h := range []int{500, 1000, 2000, 4000, 6000, 8000, 12000, 16000, 24000, 32000} {
		name := fmt.Sprintf("product=%dx%d", wide, h)
		cap, err := newCapture(caseFile(dir, name), 1<<18)
		if err != nil {
			continue
		}
		cap.event("case %s", name)
		argv := []string{self, "-emit", "lines", "-lines", "40", "-width", strconv.Itoa(width)}
		s, err := newSession(width, h, argv, cap)
		if err != nil {
			say("  %-18s create failed: %v", name, err)
			cap.close()
			continue
		}
		time.Sleep(step)
		before := cap.size()
		cap.event("RESIZE wide -> %dx%d", wide, h)
		errR := s.resize(wide, h)
		time.Sleep(step * 2)
		after := cap.size()
		clean := s.kill()
		cap.close()

		verdict := "ok"
		switch {
		case errR != nil:
			verdict = "resize refused: " + errR.Error()
		case after == before:
			verdict = "NO OUTPUT (host wedged)"
		case !clean:
			verdict = "output, but ClosePseudoConsole hung"
		}
		say("  %-18s cells=%-11d bytes=%-9d close=%-5v  %s",
			name, wide*h, after-before, clean, verdict)
	}
	say("")
}

// ---------------------------------------------------------------------------
// the child
// ---------------------------------------------------------------------------

func childStdout() syscall.Handle {
	h, _, _ := procGetStdHandle.Call(stdOutputHandle)
	return syscall.Handle(h)
}

func enableVT() {
	h := childStdout()
	var mode uint32
	procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode)))
	procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
}

type coord struct{ X, Y int16 }
type smallRect struct{ Left, Top, Right, Bottom int16 }
type screenBufferInfo struct {
	Size              coord
	CursorPosition    coord
	Attributes        uint16
	Window            smallRect
	MaximumWindowSize coord
}

func runChild(mode string, lines, width int) {
	enableVT()
	out := os.Stdout

	// The line-shape axis: every session gets all of them, so shape costs no
	// extra runs. The exactly-width line is the P13 case that no probe in this
	// repository had ever actually emitted.
	shapes := func() {
		fmt.Fprintf(out, "%s%s\r\n", markerExact,
			strings.Repeat("=", max(0, width-len(markerExact))))
		fmt.Fprintf(out, "%s%s%s\r\n", markerLongStart, strings.Repeat("x", width*3), markerLongEnd)
		fmt.Fprintf(out, "%s\u4e2d\u6587\u6587\u5b57\u5e45\u5ea6\u6d4b\u8bd5\r\n", markerWide)
		fmt.Fprintf(out, "\x1b[38;2;255;96;0m%s\x1b[0m\r\n", markerColour)
	}

	switch mode {
	case "alt":
		fmt.Fprintf(out, "%s\r\n", fillMarker(1))
		shapes()
		fmt.Fprint(out, "\x1b[?1049h")
		fmt.Fprintf(out, "\x1b[1;1H%s\r\n", markerAltIn)
		time.Sleep(2 * time.Second)
		fmt.Fprint(out, "\x1b[?1049l")
		fmt.Fprintf(out, "%s\r\n", markerDone)
		time.Sleep(30 * time.Second)
		return
	case "size":
		var info screenBufferInfo
		procGetConsoleScreenBufferInfo.Call(uintptr(childStdout()), uintptr(unsafe.Pointer(&info)))
		fmt.Fprintf(out, "~SZ:win=%dx%d;buf=%dx%d~\r\n",
			info.Window.Right-info.Window.Left+1, info.Window.Bottom-info.Window.Top+1,
			info.Size.X, info.Size.Y)
		shapes()
		// Report again after each resize the parent will make.
		for i := 0; i < 8; i++ {
			time.Sleep(1500 * time.Millisecond)
			procGetConsoleScreenBufferInfo.Call(uintptr(childStdout()), uintptr(unsafe.Pointer(&info)))
			fmt.Fprintf(out, "~SZ:win=%dx%d;buf=%dx%d~\r\n",
				info.Window.Right-info.Window.Left+1, info.Window.Bottom-info.Window.Top+1,
				info.Size.X, info.Size.Y)
		}
		return
	}

	for i := 1; i <= lines; i++ {
		fmt.Fprintf(out, "%s==========%d\r\n", fillMarker(i), i)
	}
	shapes()
	fmt.Fprintf(out, "%s\r\n", markerDone)

	// Keep writing slowly, so a resize can land during live output.
	for i := 0; i < 60; i++ {
		time.Sleep(500 * time.Millisecond)
		fmt.Fprintf(out, "%s tick %d\r\n", fillMarker(900000+i), i)
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
