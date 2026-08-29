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
	"os"
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
		emit    = flag.String("emit", "", "internal: run as the child")
		width   = flag.Int("width", 120, "pseudoconsole width")
		height  = flag.Int("height", 2000, "pseudoconsole height")
		wide    = flag.Int("wide", 4000, "width to widen to")
		lines   = flag.Int("lines", 150, "history lines the child prints")
		long    = flag.Int("long", 600, "length of the long line")
		out     = flag.String("out", "", "dump file (default conptydump-<height>.txt)")
		hold    = flag.Duration("hold", 3*time.Second, "how long the child stays alive after printing")
		step    = flag.Duration("step", 2*time.Second, "delay between resize steps")
		noPause = flag.Bool("no-pause", false, "do not wait for Enter before closing")
		rounds  = flag.Int("fuzz", 0, "run this many randomised rounds instead of one fixed case")
		seed    = flag.Int64("seed", 0, "seed for a randomised round (0 = derived from the clock)")
		during  = flag.Bool("resize-during-output", false, "resize while the child is still printing")
		logTo   = flag.String("log", "", "write the report here (default conptyreconcile-<height>.log)")
	)
	flag.Parse()

	if *emit == "child-random" {
		// A seeded child: the parent regenerates the identical list, so the
		// expected result never has to cross the pipe.
		for _, l := range randomGroundTruth(*seed, *width, *lines) {
			fmt.Print(l, "\r\n")
			if *during {
				time.Sleep(8 * time.Millisecond)
			}
		}
		fmt.Print(markerDone, "\r\n")
		time.Sleep(*hold)
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
	out := os.Stdout
	for _, l := range groundTruthLines(width, lines) {
		fmt.Fprint(out, l, "\r\n")
	}
	fmt.Fprintf(out, "%s\r\n", markerDone)
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
		trimTrailingBlanks(splitFrameLines(frame)), liveLines(live, width)))

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
