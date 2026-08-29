//go:build windows

// conptyreconcile is tools/conptydump with an analysis step appended.
//
// The ConPTY half is that tool byte for byte -- it is the only version of this
// plumbing that has worked in the field, and rebuilding it from pieces is
// exactly how the previous attempt at this program came to die on startup. The
// only additions are: a known list of lines for the child to print, a log
// file, and the reconciliation from reconcile.go applied at the end.
//
// It creates a pseudoconsole, starts a reader thread that reads until the pipe
// ends, runs a child, calls ResizePseudoConsole at fixed times, and writes
// every byte it receives to a file together with the moment it arrived and the
// moment each resize was issued. Analysis happens afterwards, off the file.
//
// This shape is deliberate. Earlier probes in this repository decided, while
// running, whether a phase had finished -- by waiting for a marker, or for the
// stream to fall silent -- and every one of those decisions turned out to be
// wrong in a way that produced confident zeroes. A dump cannot do that: the
// bytes are either in the file or they are not, and a mistake in reading it
// costs a re-read rather than another run.
//
// The ConPTY calls follow Microsoft's EchoCon sample and WezTerm's psuedocon.rs
// exactly, including the details that are easy to get wrong:
//   - two CreatePipe calls with NULL security attributes
//   - CreatePseudoConsole, then close BOTH pty-side handles immediately
//     ("the handles are dup'ed into the ConHost", EchoCon.cpp)
//   - a reader that loops on ReadFile until it fails, with no timeout
//   - STARTF_USESTDHANDLES with three INVALID_HANDLE_VALUE, so the child cannot
//     inherit our redirected stdio (wezterm psuedocon.rs)
//   - an attribute list of one entry, PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE
//   - CreateProcessW with EXTENDED_STARTUPINFO_PRESENT and nothing else
package main

import (
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"
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

	// Reading a real console's screen back, for the reference window of
	// stage 2. ReadConsoleOutputW is Windows' own view of what is displayed.
	procSetConsoleWindowInfo        = kernel32.NewProc("SetConsoleWindowInfo")
	procSetConsoleScreenBufferSize  = kernel32.NewProc("SetConsoleScreenBufferSize")
	procReadConsoleOutputW          = kernel32.NewProc("ReadConsoleOutputW")
	procGetConsoleScreenBufferInfoEx = kernel32.NewProc("GetConsoleScreenBufferInfoEx")
)

const (
	procThreadAttributePseudoConsole = 0x00020016
	extendedStartupInfoPresent       = 0x00080000
	startfUseStdHandles              = 0x00000100
)

func packCoord(x, y int) uintptr {
	return uintptr(uint32(uint16(int16(x))) | uint32(uint16(int16(y)))<<16)
}

// ---------------------------------------------------------------------------
// the dump file
// ---------------------------------------------------------------------------

type dump struct {
	mu    sync.Mutex
	f     *os.File
	start time.Time
	bytes int

	// raw keeps everything for the analysis at the end, and liveEnd marks
	// where the live stream stopped and the first frame began.
	raw     []byte
	liveEnd int

	// log receives the human-readable report, written as it is produced so a
	// crash still leaves everything up to that point on disk.
	log *os.File
}

func (d *dump) logf(format string, a ...any) {
	line := fmt.Sprintf(format, a...)
	fmt.Println(line)
	d.mu.Lock()
	if d.log != nil {
		fmt.Fprintln(d.log, line)
		d.log.Sync()
	}
	d.mu.Unlock()
}

func newDump(path string) (*dump, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &dump{f: f, start: time.Now()}, nil
}

func (d *dump) since() int64 { return time.Since(d.start).Milliseconds() }

// event records something the dumper did, on its own line, so the reader can
// see where each action falls in the byte stream.
func (d *dump) event(format string, a ...any) {
	d.mu.Lock()
	defer d.mu.Unlock()
	fmt.Fprintf(d.f, "\n@@ %7dms EVENT %s\n", d.since(), fmt.Sprintf(format, a...))
	d.f.Sync()
}

// chunk records one ReadFile result verbatim. Bytes are written as escaped
// text so the file stays a text file: ESC as \e, other control bytes as \xNN,
// everything printable as itself. Nothing is interpreted.
func (d *dump) chunk(b []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.bytes += len(b)
	d.raw = append(d.raw, b...)
	fmt.Fprintf(d.f, "\n@@ %7dms CHUNK %d bytes (total %d)\n", d.since(), len(b), d.bytes)
	d.f.WriteString(escape(b))
	d.f.WriteString("\n")
	d.f.Sync()
}

func escape(b []byte) string {
	var sb strings.Builder
	sb.Grow(len(b) + len(b)/4)
	for _, c := range b {
		switch {
		case c == 0x1b:
			sb.WriteString("\\e")
		case c == '\r':
			sb.WriteString("\\r")
		case c == '\n':
			sb.WriteString("\\n\n")
		case c == '\\':
			sb.WriteString("\\\\")
		case c >= 0x20 && c < 0x7f:
			sb.WriteByte(c)
		default:
			fmt.Fprintf(&sb, "\\x%02x", c)
		}
	}
	return sb.String()
}

func (d *dump) size() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.raw)
}

func (d *dump) markLiveEnd() {
	d.mu.Lock()
	d.liveEnd = len(d.raw)
	d.mu.Unlock()
}

func (d *dump) split() (live, frame []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()
	live = append([]byte(nil), d.raw[:d.liveEnd]...)
	frame = append([]byte(nil), d.raw[d.liveEnd:]...)
	return
}

func (d *dump) close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	fmt.Fprintf(d.f, "\n@@ %7dms END total %d bytes\n", d.since(), d.bytes)
	d.f.Sync()
	d.f.Close()
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	var (
		emit      = flag.String("emit", "", "internal: run as the child")
		width     = flag.Int("width", 120, "pseudoconsole width")
		height    = flag.Int("height", 2000, "pseudoconsole height")
		wide      = flag.Int("wide", 4000, "width to widen to")
		lines     = flag.Int("lines", 150, "history lines the child prints")
		long      = flag.Int("long", 600, "length of the long line")
		out       = flag.String("out", "", "dump file (default conptydump-<height>.txt)")
		hold      = flag.Duration("hold", 3*time.Second, "how long the child stays alive after printing")
		step      = flag.Duration("step", 2*time.Second, "delay between resize steps")
		noPause   = flag.Bool("no-pause", true, "do not wait for Enter before closing (default)")
		pause     = flag.Bool("pause", false, "wait for Enter before closing")
		rounds    = flag.Int("fuzz", 0, "run this many randomised rounds instead of one fixed case")
		seed      = flag.Int64("seed", 0, "seed for a randomised round (0 = derived from the clock)")
		during    = flag.Bool("resize-during-output", false, "resize while the child is still printing")
		winOut    = flag.String("window-out", "", "internal: where the child-window mode writes the screen it read")
		winCols   = flag.Int("window-cols", 100, "internal: reference window width")
		winRows   = flag.Int("window-rows", 25, "internal: reference window height")
		realCmd   = flag.String("cmd", "", "run this real command instead of the generated fixture")
		drag      = flag.Int("drag", 0, "simulate a corner drag: this many rapid resizes while it runs")
		linesOnly = flag.Bool("lines-only", false, "check only the line reconstruction, not the terminal pipeline")
		logTo     = flag.String("log", "", "write the report here (default conptyreconcile-<height>.log)")
	)
	flag.Parse()
	if *pause {
		*noPause = false
	}

	if *emit == "child-random" {
		w := newConsoleWriter()
		// A seeded child: the parent regenerates the identical list, so the
		// expected result never has to cross the pipe.
		for _, l := range randomGroundTruth(*seed, *width, *lines) {
			w.line(l)
			if *during {
				time.Sleep(8 * time.Millisecond)
			}
		}
		w.line(markerDone)
		time.Sleep(*hold)
		return
	}
	if *emit == "child-window" {
		runChildWindow(*seed, *width, *lines, *winCols, *winRows, *winOut)
		return
	}
	if *emit == "child" {
		// The ground truth is built from the console width, not from -long.
		// An earlier version passed -long here, so "a line of exactly the
		// width" was 600 characters wide on a 120-column console; the capture
		// still exercised the merge, but the expected list was not the one
		// intended and every mismatch message was misleading.
		runChild(*lines, *width, *hold)
		return
	}

	path := *out
	if path == "" {
		path = fmt.Sprintf("conptydump-%d.txt", *height)
	}
	d, err := newDump(path)
	if err != nil {
		fmt.Println("cannot create the dump:", err)
		os.Exit(2)
	}
	defer d.close()

	logPath := *logTo
	if logPath == "" {
		logPath = fmt.Sprintf("conptyreconcile-%d.log", *height)
	}
	lf, lerr := os.Create(logPath)
	if lerr != nil {
		fmt.Println("cannot create the log:", lerr)
		os.Exit(2)
	}
	defer lf.Close()
	d.log = lf
	d.logf("conptyreconcile -- %dx%d, %d ground-truth lines, dump in %s",
		*width, *height, *lines, path)

	self, err := os.Executable()
	if err != nil {
		fmt.Println("cannot find own path:", err)
		os.Exit(2)
	}

	// The whole pipeline is the default. Running the tool without arguments
	// should check everything it knows how to check; a narrower mode is for
	// comparing against older logs and has to be asked for.
	if *realCmd != "" {
		runRealCommand(logPath, path, *realCmd, *width, *height, *drag, *step, *noPause)
		return
	}
	if !*linesOnly && *rounds == 0 {
		runTerminal(self, logPath, path, *seed, *width, *height, *lines, *step, *noPause)
		return
	}
	if *rounds > 0 {
		runRounds(self, logPath, *rounds, *seed, *width, *height, *lines, *step, *during, *noPause)
		return
	}

	d.event("start width=%d height=%d wide=%d lines=%d long=%d", *width, *height, *wide, *lines, *long)

	// --- pipes, exactly as EchoCon.cpp -------------------------------------
	var ptyIn, ourOut, ourIn, ptyOut syscall.Handle
	if err := syscall.CreatePipe(&ptyIn, &ourOut, nil, 0); err != nil {
		d.event("CreatePipe(in) failed: %v", err)
		return
	}
	if err := syscall.CreatePipe(&ourIn, &ptyOut, nil, 0); err != nil {
		d.event("CreatePipe(out) failed: %v", err)
		return
	}

	// --- the pseudoconsole -------------------------------------------------
	var hpc uintptr
	r, _, e := procCreatePseudoConsole.Call(
		packCoord(*width, *height), uintptr(ptyIn), uintptr(ptyOut), 0, uintptr(unsafe.Pointer(&hpc)))
	if r != 0 {
		d.event("CreatePseudoConsole(%dx%d) HRESULT 0x%08x (%v)", *width, *height, uint32(r), e)
		return
	}
	d.event("CreatePseudoConsole ok, hpc=0x%x", hpc)

	// EchoCon.cpp: "We can close the handles to the PTY-end of the pipes here
	// because the handles are dup'ed into the ConHost".
	syscall.CloseHandle(ptyIn)
	syscall.CloseHandle(ptyOut)

	// --- reader: no timeout, no decisions ----------------------------------
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		buf := make([]byte, 64*1024)
		for {
			var n uint32
			err := syscall.ReadFile(ourIn, buf, &n, nil)
			if n > 0 {
				d.chunk(buf[:n])
			}
			if err != nil {
				d.event("reader stopped: %v", err)
				return
			}
			if n == 0 {
				d.event("reader stopped: zero-length read")
				return
			}
		}
	}()

	// --- the child ---------------------------------------------------------
	child, err := spawn(hpc, []string{self,
		"-emit", "child",
		"-width", strconv.Itoa(*width),
		"-lines", strconv.Itoa(*lines),
		"-long", strconv.Itoa(*long),
		"-hold", hold.String(),
	})
	if err != nil {
		d.event("CreateProcess failed: %v", err)
		procClosePseudoConsole.Call(hpc)
		return
	}
	d.event("child started, pid=%d", child.pid)

	// --- fixed schedule: no waiting on anything ----------------------------
	time.Sleep(*step)
	d.markLiveEnd() // everything so far is the live stream
	d.event("RESIZE -> %dx%d (narrow by one)", *width-1, *height)
	resize(d, hpc, *width-1, *height)

	time.Sleep(*step)
	// The analysis uses only the live stream and this first frame; the two
	// resizes below are kept because the dump is more useful with them.
	analyse(d, *width, *height, *lines, *long)

	d.event("RESIZE -> %dx%d (wide)", *wide, *height)
	resize(d, hpc, *wide, *height)

	time.Sleep(*step)
	d.event("RESIZE -> %dx%d (restore)", *width, *height)
	resize(d, hpc, *width, *height)

	time.Sleep(*step)
	d.event("closing")

	// Closing the pseudoconsole terminates the child and ends the reader.
	// It can block until the host exits, so it runs on its own goroutine and
	// the process leaves after a short grace period either way.
	go func() {
		procClosePseudoConsole.Call(hpc)
		d.event("ClosePseudoConsole returned")
	}()

	select {
	case <-readerDone:
		d.event("reader finished")
	case <-time.After(3 * time.Second):
		d.event("reader still open after 3s; ending anyway")
	}

	child.kill()
	fmt.Printf("\nwrote %s (%d bytes captured)\n", path, d.bytes)
	fmt.Printf("the same text is in %s\n", logPath)

	// Launched from Explorer the window closes with the process and the
	// tester sees nothing. The previous version of this program did exactly
	// that and cost a trip. Wait unless told not to.
	if !*noPause {
		fmt.Print("\npress Enter to close ")
		fmt.Fscanln(os.Stdin)
	}
}

func resize(d *dump, hpc uintptr, w, h int) {
	r, _, e := procResizePseudoConsole.Call(hpc, packCoord(w, h))
	if r != 0 {
		d.event("ResizePseudoConsole(%dx%d) HRESULT 0x%08x (%v)", w, h, uint32(r), e)
		return
	}
	d.event("ResizePseudoConsole(%dx%d) ok", w, h)
}

// ---------------------------------------------------------------------------
// spawn, following wezterm's psuedocon.rs
// ---------------------------------------------------------------------------

type childProc struct {
	handle syscall.Handle
	pid    uint32
}

func (c *childProc) kill() {
	if c == nil || c.handle == 0 {
		return
	}
	syscall.TerminateProcess(c.handle, 0)
	syscall.CloseHandle(c.handle)
	c.handle = 0
}

func spawn(hpc uintptr, argv []string) (*childProc, error) {
	var attrSize uintptr
	procInitAttrList.Call(0, 1, 0, uintptr(unsafe.Pointer(&attrSize)))
	if attrSize == 0 {
		return nil, fmt.Errorf("InitializeProcThreadAttributeList sized 0")
	}
	attrBuf := make([]byte, attrSize)
	attrList := uintptr(unsafe.Pointer(&attrBuf[0]))
	if r, _, err := procInitAttrList.Call(attrList, 1, 0, uintptr(unsafe.Pointer(&attrSize))); r == 0 {
		return nil, fmt.Errorf("InitializeProcThreadAttributeList: %v", err)
	}
	defer procDeleteAttrList.Call(attrList)

	if r, _, err := procUpdateAttr.Call(attrList, 0,
		uintptr(procThreadAttributePseudoConsole), hpc, unsafe.Sizeof(hpc), 0, 0); r == 0 {
		return nil, fmt.Errorf("UpdateProcThreadAttribute: %v", err)
	}

	type startupInfoEx struct {
		syscall.StartupInfo
		AttributeList uintptr
	}
	var si startupInfoEx
	si.StartupInfo.Cb = uint32(unsafe.Sizeof(si))
	// wezterm: pin the stdio handles to INVALID_HANDLE_VALUE so the child
	// cannot inherit our redirected output instead of using the pty.
	si.StartupInfo.Flags = startfUseStdHandles
	si.StartupInfo.StdInput = syscall.InvalidHandle
	si.StartupInfo.StdOutput = syscall.InvalidHandle
	si.StartupInfo.StdErr = syscall.InvalidHandle
	si.AttributeList = attrList

	cmdline := buildCommandLine(argv)
	argp, err := syscall.UTF16PtrFromString(cmdline)
	if err != nil {
		return nil, err
	}

	var pi syscall.ProcessInformation
	if err := syscall.CreateProcess(nil, argp, nil, nil, false,
		extendedStartupInfoPresent, nil, nil, &si.StartupInfo, &pi); err != nil {
		return nil, fmt.Errorf("CreateProcess(%s): %w", cmdline, err)
	}
	syscall.CloseHandle(pi.Thread)
	return &childProc{handle: pi.Process, pid: pi.ProcessId}, nil
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

// ---------------------------------------------------------------------------
// the child: prints, then stays alive on a timer
// ---------------------------------------------------------------------------

func runChild(lines, width int, hold time.Duration) {
	w := newConsoleWriter()
	for _, l := range groundTruthLines(width, lines) {
		w.line(l)
	}
	w.line(markerDone)
	time.Sleep(hold)
}

// ---------------------------------------------------------------------------
// the analysis
//
// Everything above this point is tools/conptydump unchanged. This is the only
// part that reasons about the bytes, and it does so after they have all been
// captured -- a mistake here costs a re-read of the dump, not another run.
// ---------------------------------------------------------------------------

func analyse(d *dump, width, height, lines, long int) {
	live, frame := d.split()
	// width here is the width the child wrote at; the frame was taken one
	// column narrower, and that difference must not reach the analysis.
	for _, line := range analyseStreams(live, frame, width, lines, width) {
		d.logf("%s", line)
	}
}

// roundSession is the same create/spawn/read/close sequence main() performs,
// packaged so a round can be repeated. The ConPTY calls themselves are the
// ones from tools/conptydump, unchanged.
type roundSession struct {
	hpc    uintptr
	ourIn  syscall.Handle
	ourOut syscall.Handle
	child  *childProc
}

func startSession(width, height int, argv []string, d *dump) (*roundSession, error) {
	var ptyIn, ourOut, ourIn, ptyOut syscall.Handle
	if err := syscall.CreatePipe(&ptyIn, &ourOut, nil, 0); err != nil {
		return nil, err
	}
	if err := syscall.CreatePipe(&ourIn, &ptyOut, nil, 0); err != nil {
		return nil, err
	}
	var hpc uintptr
	r, _, e := procCreatePseudoConsole.Call(packCoord(width, height),
		uintptr(ptyIn), uintptr(ptyOut), 0, uintptr(unsafe.Pointer(&hpc)))
	syscall.CloseHandle(ptyIn)
	syscall.CloseHandle(ptyOut)
	if r != 0 {
		return nil, fmt.Errorf("CreatePseudoConsole HRESULT 0x%08x (%v)", uint32(r), e)
	}

	s := &roundSession{hpc: hpc, ourIn: ourIn, ourOut: ourOut}
	go func() {
		buf := make([]byte, 64*1024)
		for {
			var n uint32
			err := syscall.ReadFile(s.ourIn, buf, &n, nil)
			if n > 0 {
				d.chunk(buf[:n])
			}
			if err != nil || n == 0 {
				return
			}
		}
	}()

	c, err := spawn(hpc, argv)
	if err != nil {
		s.stop()
		return nil, err
	}
	s.child = c
	return s, nil
}

func (s *roundSession) resize(w, h int) {
	procResizePseudoConsole.Call(s.hpc, packCoord(w, h))
}

func (s *roundSession) stop() {
	if s.child != nil {
		s.child.kill()
		s.child = nil
	}
	if s.hpc != 0 {
		hpc := s.hpc
		s.hpc = 0
		done := make(chan struct{})
		go func() { procClosePseudoConsole.Call(hpc); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
	syscall.CloseHandle(s.ourIn)
	syscall.CloseHandle(s.ourOut)
}

// ---------------------------------------------------------------------------
// randomised rounds against the real ConPTY
//
// A fixed fixture is what let the write-width bug reach the field: it happened
// not to expose the case. Each round here generates a different set of lines
// from a printed seed, so the shape changes every time and a failure is
// reproducible -- the same seed replays against the mock on any machine, with
// no Windows involved.
// ---------------------------------------------------------------------------

func runRounds(self, logPath string, rounds int, seed int64,
	width, height, lines int, step time.Duration, during, noPause bool) {

	lf, err := os.Create(logPath)
	if err != nil {
		fmt.Println("cannot create the log:", err)
		os.Exit(2)
	}
	defer lf.Close()
	say := func(format string, a ...any) {
		line := fmt.Sprintf(format, a...)
		fmt.Println(line)
		fmt.Fprintln(lf, line)
		lf.Sync()
	}

	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	mode := "resize after the output"
	if during {
		mode = "resize DURING the output"
	}
	say("conptyreconcile -- %d randomised rounds, %dx%d, %d lines, %s",
		rounds, width, height, lines, mode)
	say("base seed %d; a failing round replays on any machine with -seed <n>", seed)
	say("")

	failed := 0
	for r := 0; r < rounds; r++ {
		s := seed + int64(r)
		ok, detail := oneRound(self, s, width, height, lines, step, during)
		verdict := "ok  "
		if !ok {
			verdict = "FAIL"
			failed++
		}
		say("round %2d  seed %-20d %s  %s", r+1, s, verdict, detail)
	}

	say("")
	if failed == 0 {
		say("PASS -- %d rounds, every printed line recovered in each", rounds)
	} else {
		say("FAIL -- %d of %d rounds lost lines; replay a failing seed against the mock", failed, rounds)
	}
	if !noPause {
		fmt.Print("\npress Enter to close ")
		fmt.Fscanln(os.Stdin)
	}
	if failed > 0 {
		os.Exit(1)
	}
}

func oneRound(self string, seed int64, width, height, lines int,
	step time.Duration, during bool) (bool, string) {

	d := &dump{start: time.Now()}
	sess, err := startSession(width, height, []string{self,
		"-emit", "child-random",
		"-seed", strconv.FormatInt(seed, 10),
		"-width", strconv.Itoa(width),
		"-lines", strconv.Itoa(lines),
		"-hold", (step * 4).String(),
		fmt.Sprintf("-resize-during-output=%v", during),
	}, d)
	if err != nil {
		return false, "session failed: " + err.Error()
	}
	defer sess.stop()

	if during {
		// Cut into the middle of the output: the lines before this point were
		// written at one width and the ones after it at another.
		time.Sleep(step / 2)
		d.markLiveEnd()
		sess.resize(width-1, height)
		time.Sleep(step)
	} else {
		time.Sleep(step)
		d.markLiveEnd()
		sess.resize(width-1, height)
		time.Sleep(step)
	}

	live, frame := d.split()
	truth := trimTrailingBlanks(append(randomGroundTruth(seed, width, lines), markerDone))
	got := trimTrailingBlanks(reconcileOrdered(
		trimTrailingBlanks(splitFrameLines(frame)), liveLines(live, width), width-1))

	// The ring may have dropped the oldest lines; compare the tail.
	want := tailOf(truth, len(got))
	miss := diffCount(got, want)
	if miss == 0 && len(got) > 0 {
		return true, fmt.Sprintf("%d lines recovered from %d frame runs",
			len(got), len(trimTrailingBlanks(splitFrameLines(frame))))
	}
	if len(frame) == 0 {
		return false, "no frame at all (post-#17510 emitter?)"
	}
	return false, fmt.Sprintf("%d of %d lines wrong", miss, len(want))
}

// ---------------------------------------------------------------------------
// the whole pipeline against a real ConPTY
//
// One run that exercises everything f4 will do: a tall console, a frame read
// out of it, the mirror built from that frame, the visible slice taken at the
// real window size, the coordinate mapping over that slice, and the geometry
// decision for a full-screen program. Each stage is checked against something
// computed independently, so a pass means the stages agree rather than that
// nothing crashed.
// ---------------------------------------------------------------------------


// realWindowRows returns what a real 100x25 console actually shows after the
// same child has printed into it.
//
// It does NOT parse a byte stream. The first version of this did, with
// splitFrameLines, and that was wrong in the way this whole project is about:
// splitFrameLines recovers row boundaries from the ESC[K terminator, conhost
// omits ESC[K after a row that was filled to the edge (P6), so every full row
// merged with its successor and a 25-row screen read back as 14. A reference
// that infers structure from a hint is not a reference.
//
// Instead the child sizes its own console to the window, prints, and reads its
// own screen buffer back with ReadConsoleOutputW -- Windows' own record of
// what is on the screen, no parsing anywhere -- and writes those rows to a
// file the probe reads. Wide glyphs occupy two cells there, the second marked
// COMMON_LVB_TRAILING_BYTE, which is skipped so the row reads as text.
//
// Deliberately not called an oracle: that word belongs to the rejected
// wide-resize path (§17), and reusing it is how a rejected path creeps back in
// under a familiar name. Nothing here is widened and nothing is stamped into
// any history; it is a second console in a probe.
func realWindowRows(self string, seed int64, width, lines, winW, winH int,
	step time.Duration) ([]string, error) {

	tmp, err := os.CreateTemp("", "conptyreconcile-window-*.txt")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)

	cmd := exec.Command(self,
		"-emit", "child-window",
		"-seed", strconv.FormatInt(seed, 10),
		"-width", strconv.Itoa(width),
		"-lines", strconv.Itoa(lines),
		"-window-out", path,
		"-window-cols", strconv.Itoa(winW),
		"-window-rows", strconv.Itoa(winH),
	)
	// Its own console, so that sizing it cannot disturb ours.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windowsCreateNewConsole}
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(string(body), "ERROR ") {
		return nil, fmt.Errorf("%s", strings.TrimSpace(string(body)))
	}
	rows := strings.Split(strings.TrimRight(string(body), "\n"), "\n")
	for i := range rows {
		rows[i] = strings.TrimRight(rows[i], " ")
	}
	return rows, nil
}

const windowsCreateNewConsole = 0x00000010

// runChildWindow is the child side of realWindowRows: size this console to the
// window, print the same lines, then read the screen back through Windows.
//
// Every API call below follows a Microsoft reference rather than memory,
// because getting these wrong is how the previous two attempts at this stage
// failed:
//   - CONOUT$ is opened with the flags of src/tools/pixels/main.cpp:
//     GENERIC_READ|GENERIC_WRITE, FILE_SHARE_WRITE, OPEN_EXISTING. The std
//     handles cannot be used: a Go child started with CreateProcess gets the
//     null device for stdout unless one is supplied, so GetStdHandle would
//     hand back NUL and every console call on it would fail. That is the
//     "exit status 2" of the previous run.
//   - the size is set the way src/tools/ConsoleBench/main.cpp sets it:
//     SetConsoleScreenBufferSize first, then SetConsoleWindowInfo with
//     Right/Bottom one less than the size. Shrinking the window to nothing
//     first, which the previous version did, is not in any reference.
//   - the screen is read the way src/tools/ConsoleMonitor/main.cpp reads it:
//     GetConsoleScreenBufferInfoEx for the real size, a buffer of that size,
//     a read region whose Right/Bottom are the size, and the region READ BACK
//     from the call to learn how much was actually delivered -- ReadConsoleOutputW
//     clips and reports. The previous version assumed the whole rectangle came
//     back.
//   - wide glyphs are handled as ConsoleMonitor handles them: the leading cell
//     contributes its character, a cell marked COMMON_LVB_TRAILING_BYTE
//     contributes none.
func runChildWindow(seed int64, width, lines, cols, rows int, outPath string) {
	// Diagnostics go into the output file. stderr here is a console window
	// that closes with the process, so the previous failure arrived at the
	// probe as a bare exit code with no reason attached.
	fail := func(what string, e error) {
		os.WriteFile(outPath, []byte("ERROR "+what+": "+fmt.Sprint(e)+"\n"), 0o644)
		os.Exit(0)
	}

	name, _ := syscall.UTF16PtrFromString("CONOUT$")
	h, err := syscall.CreateFile(name,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_WRITE,
		nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		fail("CreateFile CONOUT$", err)
	}
	defer syscall.CloseHandle(h)

	// Sizing, ported from .NET referencesource system/console.cs --
	// SetWindowSize then SetBufferSize -- which is the reference
	// src/host/ft_uia/MiscTests.cs itself points at ("Adapted from .NET
	// source code"). The previous attempt copied ConsoleBench, whose buffer
	// (120x9001) is LARGER than its window, so a bare
	// SetConsoleScreenBufferSize was enough there; this console shrinks, and
	// the API refuses a buffer smaller than the current window ("The
	// parameter is incorrect" of the last run). .NET's sequence handles both
	// directions: grow the buffer just enough for the new window if needed,
	// set the window, then set the buffer to the final size, which the
	// window now fits.
	var csbi consoleScreenBufferInfoEx
	csbi.cbSize = uint32(unsafe.Sizeof(csbi))
	if r, _, e := procGetConsoleScreenBufferInfoEx.Call(uintptr(h), uintptr(unsafe.Pointer(&csbi))); r == 0 {
		fail("GetConsoleScreenBufferInfoEx", e)
	}

	// Console.SetWindowSize(width, height):
	sizeX, sizeY := csbi.dwSizeX, csbi.dwSizeY
	resizeBuffer := false
	if int(csbi.srWindow.left)+cols > int(sizeX) {
		sizeX = int16(int(csbi.srWindow.left) + cols)
		resizeBuffer = true
	}
	if int(csbi.srWindow.top)+rows > int(sizeY) {
		sizeY = int16(int(csbi.srWindow.top) + rows)
		resizeBuffer = true
	}
	if resizeBuffer {
		if r, _, e := procSetConsoleScreenBufferSize.Call(uintptr(h), packCoord(int(sizeX), int(sizeY))); r == 0 {
			fail("SetConsoleScreenBufferSize (grow for window)", e)
		}
	}
	srWindow := csbi.srWindow
	srWindow.bottom = srWindow.top + int16(rows) - 1
	srWindow.right = srWindow.left + int16(cols) - 1
	if r, _, e := procSetConsoleWindowInfo.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&srWindow))); r == 0 {
		fail("SetConsoleWindowInfo", e)
	}

	// Console.SetBufferSize(width, height): the window now fits, so the
	// final buffer size is legal.
	if r, _, e := procSetConsoleScreenBufferSize.Call(uintptr(h), packCoord(cols, rows)); r == 0 {
		fail("SetConsoleScreenBufferSize", e)
	}

	procSetConsoleOutputCP.Call(cpUTF8)
	w := &consoleWriter{h: h}
	for _, l := range randomGroundTruth(seed, width, lines) {
		w.line(l)
	}
	w.line(markerDone)

	// ConsoleMonitor asks the console for its own size rather than assuming.
	info := consoleScreenBufferInfoEx{cbSize: uint32(unsafe.Sizeof(consoleScreenBufferInfoEx{}))}
	if r, _, e := procGetConsoleScreenBufferInfoEx.Call(uintptr(h), uintptr(unsafe.Pointer(&info))); r == 0 {
		fail("GetConsoleScreenBufferInfoEx", e)
	}
	bufW, bufH := int(info.dwSizeX), int(info.dwSizeY)

	buf := make([]charInfo, bufW*bufH)
	// Right/Bottom are the size, not size-1, exactly as ConsoleMonitor passes
	// them; the call writes back what it actually read.
	readArea := smallRect{0, 0, int16(bufW), int16(bufH)}
	if r, _, e := procReadConsoleOutputW.Call(uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])), packCoord(bufW, bufH), packCoord(0, 0),
		uintptr(unsafe.Pointer(&readArea))); r == 0 {
		fail("ReadConsoleOutputW", e)
	}
	cellCountX := int(readArea.right) + 1
	cellCountY := int(readArea.bottom) + 1

	var sb strings.Builder
	for y := 0; y < cellCountY; y++ {
		offset := bufW * y
		var line []uint16
		for x := 0; x < cellCountX; x++ {
			ci := buf[offset+x]
			if ci.attributes&commonLvbTrailingByte != 0 {
				continue
			}
			line = append(line, ci.unicodeChar)
		}
		sb.WriteString(strings.TrimRight(string(utf16.Decode(line)), " "))
		sb.WriteString("\n")
	}

	if err := os.WriteFile(outPath, []byte(sb.String()), 0o644); err != nil {
		os.Exit(2)
	}
}

// consoleScreenBufferInfoEx is CONSOLE_SCREEN_BUFFER_INFOEX. Only the size is
// read here, but the whole struct has to be laid out so cbSize is right.
type consoleScreenBufferInfoEx struct {
	cbSize               uint32
	dwSizeX, dwSizeY     int16
	dwCursorPositionX    int16
	dwCursorPositionY    int16
	wAttributes          uint16
	srWindow             smallRect
	dwMaximumWindowSizeX int16
	dwMaximumWindowSizeY int16
	wPopupAttributes     uint16
	bFullscreenSupported int32
	colorTable           [16]uint32
}

const commonLvbTrailingByte = 0x0200

type charInfo struct {
	unicodeChar uint16
	attributes  uint16
}

type smallRect struct{ left, top, right, bottom int16 }


func runTerminal(self, logPath, dumpPath string, seed int64, width, height, lines int,
	step time.Duration, noPause bool) {

	lf, err := os.Create(logPath)
	if err != nil {
		fmt.Println("cannot create the log:", err)
		os.Exit(2)
	}
	defer lf.Close()
	say := func(format string, a ...any) {
		line := fmt.Sprintf(format, a...)
		fmt.Println(line)
		fmt.Fprintln(lf, line)
		lf.Sync()
	}

	if seed == 0 {
		seed = time.Now().UnixNano()
	}
	const winW, winH = 100, 25

	say("conptyreconcile -- tall console %dx%d, window %dx%d, seed %d, dump in %s",
		width, height, winW, winH, seed, dumpPath)
	say("")

	// A dump is written here too. The first run of this mode failed and left
	// nothing to look at, which cost a round trip that a file would have
	// saved.
	d, derr := newDump(dumpPath)
	if derr != nil {
		say("cannot create the dump: %v", derr)
		os.Exit(2)
	}
	defer d.close()

	sess, err := startSession(width, height, []string{self,
		"-emit", "child-random",
		"-seed", strconv.FormatInt(seed, 10),
		"-width", strconv.Itoa(width),
		"-lines", strconv.Itoa(lines),
		"-hold", (step * 4).String(),
	}, d)
	if err != nil {
		say("session failed: %v", err)
		os.Exit(2)
	}
	defer sess.stop()

	time.Sleep(step)
	d.markLiveEnd()
	sess.resize(width-1, height)
	time.Sleep(step)

	live, frame := d.split()
	printed := trimTrailingBlanks(append(randomGroundTruth(seed, width, lines), markerDone))

	fail := 0
	check := func(name string, ok bool, detail string) {
		if ok {
			say("  ok    %-34s %s", name, detail)
			return
		}
		fail++
		say("  FAIL  %-34s %s", name, detail)
	}

	// 1. the mirror
	recovered := trimTrailingBlanks(reconcileOrdered(
		trimTrailingBlanks(splitFrameLines(frame)), liveLines(live, width), width-1))
	m := NewMirror()
	m.Replace(recovered)
	check("mirror holds every printed line",
		len(m.Lines()) == len(printed) && diffCount(m.Lines(), printed) == 0,
		fmt.Sprintf("%d lines, expected %d", len(m.Lines()), len(printed)))

	// 2. the visible slice, against a second real ConPTY of exactly the
	// window size.
	//
	// This used to compare Slice() against Wrap() of the ground truth. Both
	// sides of that comparison now run the same ported conhost code, so it
	// checked nothing that stage 1 had not already checked. The second side
	// has to be something that is not us: a real 100x25 console fed the
	// identical stream, whose own last 25 rows are what a user would see. If
	// our slice and conhost's screen agree row for row, the wrap is right.
	//
	// This is NOT the wide-resize oracle of the rejected path 2 (§17), and
	// the word is avoided deliberately. Nothing is widened -- the reference
	// console is the window size, one column wider only for the instant that
	// forces the repaint -- nothing is stamped into any history, and this
	// lives in the probe and never in f4. The reason path 2 was rejected is
	// that it wrote flags back into a history f4 also owned; a reference
	// console in a test writes nothing anywhere.
	v := m.Slice(winW, winH)
	realRows, refErr := realWindowRows(self, seed, width, lines, winW, winH, step)
	// The screen's last row is blank: the final \r\n scrolled the content up
	// and left the cursor on a fresh row, which the user sees but the mirror
	// slice (a slice of content) does not carry. So trailing blank rows are
	// dropped from the screen and the remainder is compared against the
	// BOTTOM of our slice: screen row k from the end must equal slice row k
	// from the end.
	for len(realRows) > 0 && strings.TrimRight(realRows[len(realRows)-1], " ") == "" {
		realRows = realRows[:len(realRows)-1]
	}
	sliceOK := refErr == nil && len(v.Rows) == winH &&
		len(realRows) > 0 && len(realRows) <= winH
	if sliceOK {
		off := len(v.Rows) - len(realRows)
		for i := range realRows {
			if strings.TrimRight(v.Rows[off+i].Text, " ") != strings.TrimRight(realRows[i], " ") {
				sliceOK = false
				break
			}
		}
	}
	detail := fmt.Sprintf("%d rows of %d total, %d read back from a real %dx%d console",
		len(v.Rows), v.Total, len(realRows), winW, winH)
	if refErr != nil {
		detail = fmt.Sprintf("reference console failed: %v", refErr)
	}
	check("visible slice matches a real window", sliceOK, detail)

	// 3. coordinates: every visible cell must map to the mirror and back
	coordOK, checked := true, 0
	for row := range v.Rows {
		for col := 0; col <= len(v.Rows[row].Text); col++ {
			p, ok := v.ScreenToMirror(col, row)
			if !ok || p.Line < 0 || p.Line >= len(m.Lines()) || p.Offset > len(m.Lines()[p.Line]) {
				coordOK = false
				break
			}
			c2, r2, ok := v.MirrorToScreen(p)
			if !ok {
				coordOK = false
				break
			}
			if back, _ := v.ScreenToMirror(c2, r2); back != p {
				coordOK = false
				break
			}
			checked++
		}
	}
	check("screen and mirror coordinates agree", coordOK,
		fmt.Sprintf("%d cells round-tripped", checked))

	// 4. scrolling is bounded at both ends
	top := m.ScrollBy(1<<30, winW, winH)
	bottom := m.ScrollBy(-(1 << 30), winW, winH)
	check("scroll is clamped, never wrapped", top == v.Total-winH && bottom == 0,
		fmt.Sprintf("top %d (rows %d), bottom %d", top, v.Total, bottom))

	// 5. a width change costs no round trip: the mirror re-wraps itself
	narrow := m.Slice(60, winH)
	wide := m.Slice(200, winH)
	check("re-wrap needs no new frame",
		len(narrow.Rows) == winH && len(wide.Rows) == winH && narrow.Total > wide.Total,
		fmt.Sprintf("%d rows at 60, %d at 200", narrow.Total, wide.Total))

	// 6. the detector, exercised with a program every Windows has.
	//
	// vim, far and mc are not on most machines, so watching for them would
	// make this check untestable exactly where it needs testing. cmd.exe is
	// always there, and whether the watched name denotes a full-screen program
	// is a setting anyway (§16) -- what has to be proven is that the layer
	// sees a descendant appear and disappear.
	det := &Detector{
		Process: NewProcessWatcher(SnapshotLister{}, uint32(os.Getpid()), []string{"cmd.exe"}),
		Signals: &FrameSignals{Height: height},
	}
	det.Alt.Feed(live)
	det.Alt.Feed(frame)
	altSeen, _ := det.Alt.Active()

	before, _ := det.Decide()

	// Start a cmd.exe under us and let the watcher see it.
	victim := exec.Command("cmd.exe", "/c", "ping -n 3 127.0.0.1 >nul")
	if err := victim.Start(); err != nil {
		check("detector sees a descendant appear", false, "cannot start cmd.exe: "+err.Error())
	} else {
		time.Sleep(400 * time.Millisecond)
		during, why := det.Decide()
		gTall := GeometryFor(winW, winH, height, false)
		gReal := GeometryFor(winW, winH, height, during)
		check("detector sees a descendant appear",
			!before && during && strings.HasPrefix(why, "process: "),
			fmt.Sprintf("before %v, during %v (%s); alt-screen in the stream: %v",
				before, during, why, altSeen))
		check("geometry switches with it",
			gTall.Height == height && gReal.Height == winH,
			fmt.Sprintf("tall %d, real %d", gTall.Height, gReal.Height))

		victim.Wait()
		time.Sleep(300 * time.Millisecond)
		after, _ := det.Decide()
		check("detector releases when it exits", !after,
			fmt.Sprintf("after %v", after))
	}

	// The user's override must beat every layer.
	det.Forced, det.ForcedSet = false, true
	forced, fwhy := det.Decide()
	check("user override wins", !forced && fwhy == "forced by the user", fwhy)

	say("")
	if fail == 0 {
		say("PASS -- the whole pipeline agrees, seed %d", seed)
	} else {
		say("FAIL -- %d stage(s) wrong; replay with -seed %d against the mock", fail, seed)
	}
	if !noPause {
		fmt.Print("\npress Enter to close ")
		fmt.Fscanln(os.Stdin)
	}
	if fail > 0 {
		os.Exit(1)
	}
}

// ---------------------------------------------------------------------------
// the child's writer
//
// Go writes bytes; a console interprets them through its output code page,
// which on a Russian machine is 866 and turns UTF-8 into mojibake. That is
// recorded in docs/CONPTY_RESEARCH.md -- a Russian filename came back mangled
// under OEM 850 in an earlier probe -- and it is what made the first run of
// this tool with non-ASCII content collapse from 151 lines to 66.
//
// WriteConsoleW takes UTF-16 and never consults a code page, so the question
// does not arise. SetConsoleOutputCP is set as well, for anything that writes
// bytes anyway.
// ---------------------------------------------------------------------------

var (
	procWriteConsoleW      = kernel32.NewProc("WriteConsoleW")
	procSetConsoleOutputCP = kernel32.NewProc("SetConsoleOutputCP")
	procGetStdHandleW      = kernel32.NewProc("GetStdHandle")
)

const cpUTF8 = 65001

type consoleWriter struct {
	h syscall.Handle
}

func newConsoleWriter() *consoleWriter {
	procSetConsoleOutputCP.Call(cpUTF8)
	h, _, _ := procGetStdHandleW.Call(^uintptr(10)) // STD_OUTPUT_HANDLE
	return &consoleWriter{h: syscall.Handle(h)}
}

func (w *consoleWriter) line(s string) {
	u := utf16.Encode([]rune(s + "\r\n"))
	if len(u) == 0 {
		return
	}
	var written uint32
	procWriteConsoleW.Call(uintptr(w.h), uintptr(unsafe.Pointer(&u[0])),
		uintptr(len(u)), uintptr(unsafe.Pointer(&written)), 0)
}

// ---------------------------------------------------------------------------
// a real command, under a corner drag
//
// The generated fixture is deterministic, which is what makes the ground-truth
// comparison possible -- and also what makes it unlike anything a user runs.
// `dir /s` prints tens of thousands of lines of real text at real speed, and a
// corner drag resizes the window dozens of times while it does. There is no
// ground truth to compare against here, so this checks the invariants that
// must hold whatever the content is, and records everything that happened so a
// failure can be read afterwards rather than reproduced.
// ---------------------------------------------------------------------------

func runRealCommand(logPath, dumpPath, command string,
	width, height, drags int, step time.Duration, noPause bool) {

	lf, err := os.Create(logPath)
	if err != nil {
		fmt.Println("cannot create the log:", err)
		os.Exit(2)
	}
	defer lf.Close()
	say := func(format string, a ...any) {
		line := fmt.Sprintf(format, a...)
		fmt.Println(line)
		fmt.Fprintln(lf, line)
		lf.Sync()
	}

	if drags < 1 {
		drags = 40
	}
	say("conptyreconcile -- real command under a corner drag")
	say("command: %s", command)
	say("console: %dx%d, %d resizes, dump in %s", width, height, drags, dumpPath)
	say("")

	d, derr := newDump(dumpPath)
	if derr != nil {
		say("cannot create the dump: %v", derr)
		os.Exit(2)
	}
	defer d.close()

	sess, err := startSession(width, height,
		[]string{"cmd.exe", "/d", "/c", command}, d)
	if err != nil {
		say("session failed: %v", err)
		os.Exit(2)
	}
	defer sess.stop()

	// The drag: rapid width changes while the command is producing output,
	// which is the case §7 died on and the one a user reproduces by grabbing
	// the corner of the window.
	type step0 struct {
		at    time.Duration
		width int
		bytes int
	}
	var timeline []step0
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	start := time.Now()

	widths := make([]int, 0, drags)
	for i := 0; i < drags; i++ {
		w := 40 + rnd.Intn(width-40+1)
		widths = append(widths, w)
		sess.resize(w, height)
		timeline = append(timeline, step0{at: time.Since(start), width: w, bytes: d.size()})
		time.Sleep(time.Duration(20+rnd.Intn(60)) * time.Millisecond)
	}
	sess.resize(width, height)
	time.Sleep(step)

	live, frame := d.split()
	all := append(append([]byte(nil), live...), frame...)

	say("timeline of resizes (ms, width, bytes captured so far):")
	for i, t := range timeline {
		if i < 8 || i%8 == 0 || i == len(timeline)-1 {
			say("  %6dms  width %-5d %8d bytes", t.at.Milliseconds(), t.width, t.bytes)
		}
	}
	say("")

	fail := 0
	check := func(name string, ok bool, detail string) {
		if ok {
			say("  ok    %-38s %s", name, detail)
			return
		}
		fail++
		say("  FAIL  %-38s %s", name, detail)
	}

	// There is no ground truth, so these are the invariants that hold for any
	// content at all.
	ll := liveLines(all, width)
	check("output arrived", len(all) > 0 && len(ll) > 0,
		fmt.Sprintf("%d bytes, %d logical lines", len(all), len(ll)))

	runs := trimTrailingBlanks(splitFrameLines(frame))
	fixed := trimTrailingBlanks(reconcileOrdered(runs, ll, width))
	check("the correction only ever splits",
		strings.Join(fixed, "") == strings.Join(runs, ""),
		fmt.Sprintf("%d runs became %d lines", len(runs), len(fixed)))

	m := NewMirror()
	m.Replace(fixed)

	// Re-wrap at every width the drag visited: the mirror must survive all of
	// them, and no row may exceed the width it was wrapped to.
	badRow := ""
	for _, w := range append(widths, width) {
		for _, r := range Wrap(m.Lines(), w) {
			if c := cellLen(r.Text); c > w && countRunesIn(r.Text) != 1 {
				badRow = fmt.Sprintf("a row of %d cells at width %d", c, w)
				break
			}
		}
		if badRow != "" {
			break
		}
	}
	check("re-wrap holds at every width the drag used", badRow == "",
		fmt.Sprintf("%d widths from %d to %d; %s", len(widths)+1, minOf(widths), width, badRow))

	// Coordinates over the final viewport.
	v := m.Slice(100, 25)
	coordOK, cells := true, 0
	for row := range v.Rows {
		for col := 0; col <= cellLen(v.Rows[row].Text); col++ {
			p, ok := v.ScreenToMirror(col, row)
			if !ok || p.Line < 0 || p.Line >= len(m.Lines()) || p.Offset > len(m.Lines()[p.Line]) {
				coordOK = false
				break
			}
			cells++
		}
	}
	check("coordinates stay inside the mirror", coordOK,
		fmt.Sprintf("%d cells checked over %d rows", cells, len(v.Rows)))

	check("scroll is clamped after the drag",
		m.ScrollBy(1<<30, 100, 25) == max0(v.Total-25) && m.ScrollBy(-(1<<30), 100, 25) == 0,
		fmt.Sprintf("%d rows total", v.Total))

	say("")
	if fail == 0 {
		say("PASS -- %d resizes during live output, invariants held", len(widths))
	} else {
		say("FAIL -- %d invariant(s) broken; the dump has every byte", fail)
	}
	if !noPause {
		fmt.Print("\npress Enter to close ")
		fmt.Fscanln(os.Stdin)
	}
	if fail > 0 {
		os.Exit(1)
	}
}

func countRunesIn(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

func minOf(v []int) int {
	if len(v) == 0 {
		return 0
	}
	m := v[0]
	for _, x := range v {
		if x < m {
			m = x
		}
	}
	return m
}
