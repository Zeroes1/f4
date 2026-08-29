//go:build windows

// conptydump captures what a ConPTY emits. It makes no decisions, waits for
// nothing, and measures nothing.
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
		emit   = flag.String("emit", "", "internal: run as the child")
		width  = flag.Int("width", 120, "pseudoconsole width")
		height = flag.Int("height", 2000, "pseudoconsole height")
		wide   = flag.Int("wide", 4000, "width to widen to")
		lines  = flag.Int("lines", 150, "history lines the child prints")
		long   = flag.Int("long", 600, "length of the long line")
		out    = flag.String("out", "", "dump file (default conptydump-<height>.txt)")
		hold   = flag.Duration("hold", 3*time.Second, "how long the child stays alive after printing")
		step   = flag.Duration("step", 2*time.Second, "delay between resize steps")
	)
	flag.Parse()

	if *emit == "child" {
		runChild(*lines, *long, *hold)
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

	self, err := os.Executable()
	if err != nil {
		fmt.Println("cannot find own path:", err)
		os.Exit(2)
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
	d.event("RESIZE -> %dx%d (narrow by one)", *width-1, *height)
	resize(d, hpc, *width-1, *height)

	time.Sleep(*step)
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
	fmt.Printf("wrote %s (%d bytes captured)\n", path, d.bytes)
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

func runChild(lines, long int, hold time.Duration) {
	out := os.Stdout
	for i := 1; i <= lines; i++ {
		fmt.Fprintf(out, "~F%06d~==========%d\r\n", i, i)
	}
	fmt.Fprintf(out, "~L1~%s~E1~\r\n", strings.Repeat("x", long))
	fmt.Fprintf(out, "~DONE~\r\n")
	time.Sleep(hold)
}
