//go:build windows

package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// The probe is its own child. Driving cmd.exe through a pseudoconsole means
// fighting command echo, the prompt, code pages and pagers -- all of which
// cost earlier probes a false negative each. A child we wrote emits exactly
// what the measurement needs, reports the console geometry it sees, and can
// enter the alternate screen on cue. cmd.exe is still exercised, but as one
// scenario rather than as the instrument.

var (
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
	procSetConsoleScreenBufferSize = kernel32.NewProc("SetConsoleScreenBufferSize")
	procGetConsoleMode             = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode             = kernel32.NewProc("SetConsoleMode")
	procGetStdHandle               = kernel32.NewProc("GetStdHandle")
)

const (
	stdOutputHandle                 = ^uintptr(10) // (DWORD)-11
	stdInputHandle                  = ^uintptr(9)  // (DWORD)-10
	enableVirtualTerminalProcessing = 0x0004
)

type childCoord struct{ X, Y int16 }

type childSmallRect struct{ Left, Top, Right, Bottom int16 }

type childScreenBufferInfo struct {
	Size              childCoord
	CursorPosition    childCoord
	Attributes        uint16
	Window            childSmallRect
	MaximumWindowSize childCoord
}

func childStdout() syscall.Handle {
	h, _, _ := procGetStdHandle.Call(stdOutputHandle)
	return syscall.Handle(h)
}

func childBufferInfo() (childScreenBufferInfo, bool) {
	var info childScreenBufferInfo
	r, _, _ := procGetConsoleScreenBufferInfo.Call(uintptr(childStdout()), uintptr(unsafe.Pointer(&info)))
	return info, r != 0
}

// sizeLine is how the child reports what a width- and height-aware program
// would decide on. Direction C died because programs format for the width
// they are told; direction F must know what they are told about the height.
func sizeLine(phase string) string {
	info, ok := childBufferInfo()
	if !ok {
		return fmt.Sprintf("~SZ:%s;err=GetConsoleScreenBufferInfo~", phase)
	}
	return fmt.Sprintf("~SZ:%s;wincols=%d;winrows=%d;bufw=%d;bufh=%d~",
		phase,
		info.Window.Right-info.Window.Left+1,
		info.Window.Bottom-info.Window.Top+1,
		info.Size.X, info.Size.Y)
}

func enableVT() {
	h := childStdout()
	var mode uint32
	procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode)))
	procSetConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
}

// The child runs on its own clock rather than waiting for the parent.
//
// It used to block on stdin between phases, and in the field that read did not
// block at all: the child raced through every phase and exited within a
// hundred milliseconds, so by the time the parent resized there was no session
// left to answer. The whole reflow column of that run read as zeroes, which
// looked like a ConPTY finding and was a synchronisation bug. A fixed window,
// sized by the parent and printed in the log, cannot fail that way: if the
// parent runs late, the markers still say which phase each byte belongs to.
func runChild(fillLines, longWidth, windowMs int) {
	enableVT()
	out := os.Stdout

	fmt.Fprintf(out, "%s\r\n", sizeLine("start"))

	// Phase 1: history. Each line is individually identifiable, so a repaint
	// can be asked not only "how many lines" but "which ones".
	pad := strings.Repeat("=", 12)
	for i := 1; i <= fillLines; i++ {
		fmt.Fprintf(out, "%s%s%d\r\n", fillMarker(i), pad, i)
	}

	// Phase 2: one long line, for the rejoin question.
	fmt.Fprintf(out, "%s%s%s\r\n", markerLongStart, strings.Repeat("x", longWidth), markerLongEnd)

	// Phase 3: truecolor, to prove the stream keeps fidelity a cell grid
	// would have flattened.
	fmt.Fprintf(out, "\x1b[38;2;255;96;0m%s\x1b[0m\r\n", markerColor)

	fmt.Fprintf(out, "%s\r\n", markerDone)

	// The parent performs its width experiments during this window.
	fmt.Fprintf(out, "%s;window=%dms\r\n", markerReady, windowMs)
	time.Sleep(time.Duration(windowMs) * time.Millisecond)

	// Phase 4: can a client set a buffer taller than the viewport under
	// ConPTY? getset.cpp rejects only sizes smaller than the viewport, so
	// this should succeed -- measured rather than assumed.
	if info, ok := childBufferInfo(); ok {
		want := childCoord{X: info.Size.X, Y: info.Size.Y + 500}
		packed := uint32(uint32(uint16(want.X)) | uint32(uint16(want.Y))<<16)
		r, _, err := procSetConsoleScreenBufferSize.Call(uintptr(childStdout()), uintptr(packed))
		after, _ := childBufferInfo()
		fmt.Fprintf(out, "~SETBUF:asked=%dx%d;ok=%v;err=%v;now=%dx%d~\r\n",
			want.X, want.Y, r != 0, err, after.Size.X, after.Size.Y)
	}

	// Phase 5: the alternate screen, announced in the stream the way every
	// full-screen program announces it.
	fmt.Fprintf(out, "%s\r\n", sizeLine("pre-alt"))
	fmt.Fprint(out, "\x1b[?1049h")
	time.Sleep(250 * time.Millisecond)
	fmt.Fprintf(out, "%s\r\n", sizeLine("in-alt"))
	// Draw at the top of the alternate screen, where a TUI would.
	fmt.Fprintf(out, "\x1b[1;1H~ALTDRAW~\r\n")
	time.Sleep(150 * time.Millisecond)
	fmt.Fprint(out, "\x1b[?1049l")
	time.Sleep(150 * time.Millisecond)
	fmt.Fprintf(out, "%s\r\n", sizeLine("post-alt"))
	fmt.Fprintf(out, "%s\r\n", markerAltDone)

	// Stay alive briefly so the parent can read the tail before the session
	// ends; a child that exits immediately takes its pseudoconsole with it.
	time.Sleep(1500 * time.Millisecond)
	fmt.Fprintf(out, "%s\r\n", markerBye)
	time.Sleep(200 * time.Millisecond)
}

// runChildTopDraw exercises the risk that a program drawing at the top of the
// window draws far above the slice f4 shows. PowerShell 5's Write-Progress is
// the real-world case; this is the same shape without needing PowerShell.
func runChildTopDraw() {
	enableVT()
	out := os.Stdout
	fmt.Fprintf(out, "%s\r\n", sizeLine("topdraw"))
	fmt.Fprint(out, "\x1b[s\x1b[1;1H~TOPDRAW~\x1b[u")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(out, "%s bottom\r\n", fillMarker(i))
	}
	fmt.Fprintf(out, "%s\r\n", markerDone)
	time.Sleep(300 * time.Millisecond)
}
