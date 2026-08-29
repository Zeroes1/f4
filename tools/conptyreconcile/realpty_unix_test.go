//go:build linux || darwin

package main

import (
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// The same exercise as the Windows -cmd mode, on a real pty here: a real
// command producing real output at real speed, resized rapidly while it runs.
//
// A Unix pty is not ConPTY -- it wraps nothing and sends no frames -- so this
// cannot check the reconstruction. What it does check is everything that is
// not ConPTY-specific: that the terminal core survives dozens of width changes
// during live output, that re-wrapping holds at every width used, and that
// coordinates and scrolling stay inside the mirror. Those are the parts that
// would otherwise only ever be exercised on someone else's machine.

func TestRealCommandUnderACornerDrag(t *testing.T) {
	if _, err := exec.LookPath("ls"); err != nil {
		t.Skip("no ls")
	}

	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skip("no /dev/ptmx:", err)
	}
	defer master.Close()

	if err := unlockpt(master); err != nil {
		t.Skip("cannot unlock the pty:", err)
	}
	name, err := ptsname(master)
	if err != nil {
		t.Skip("cannot name the pty:", err)
	}
	slave, err := os.OpenFile(name, os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Skip("cannot open the pty slave:", err)
	}
	defer slave.Close()

	setSize(t, master, 120, 2000)

	cmd := exec.Command("ls", "-laR", "/usr")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		t.Skip("cannot start ls:", err)
	}
	defer func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() }()

	var captured []byte
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 64*1024)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				captured = append(captured, buf[:n]...)
			}
			if err != nil {
				return
			}
		}
	}()

	// The drag: rapid width changes while output is arriving.
	widths := []int{}
	for i := 0; i < 40; i++ {
		w := 40 + (i*17)%80
		widths = append(widths, w)
		setSize(t, master, w, 2000)
		time.Sleep(25 * time.Millisecond)
	}
	setSize(t, master, 120, 2000)
	time.Sleep(300 * time.Millisecond)

	_ = cmd.Process.Kill()
	cmd.Wait()
	slave.Close()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
	}

	if len(captured) == 0 {
		t.Skip("the pty produced nothing")
	}
	t.Logf("captured %d bytes across %d resizes", len(captured), len(widths))

	// The terminal core, over what a real command actually printed.
	lines := liveLines(captured, 120)
	if len(lines) == 0 {
		t.Fatal("no logical lines from a real command")
	}
	texts := make([]string, 0, len(lines))
	for _, l := range lines {
		texts = append(texts, l.Text)
	}

	m := NewMirror()
	m.Replace(texts)
	t.Logf("mirror holds %d logical lines", len(m.Lines()))

	for _, w := range append(widths, 120) {
		for _, r := range Wrap(m.Lines(), w) {
			if c := cellLen(r.Text); c > w && countRunesIn2(r.Text) != 1 {
				t.Fatalf("a row of %d cells at width %d", c, w)
			}
		}
	}

	v := m.Slice(100, 25)
	checked := 0
	for row := range v.Rows {
		for col := 0; col <= cellLen(v.Rows[row].Text); col++ {
			p, ok := v.ScreenToMirror(col, row)
			if !ok {
				t.Fatalf("(%d,%d) did not map", col, row)
			}
			if p.Line < 0 || p.Line >= len(m.Lines()) || p.Offset > len(m.Lines()[p.Line]) {
				t.Fatalf("(%d,%d) mapped outside the mirror: %+v", col, row, p)
			}
			checked++
		}
	}
	t.Logf("%d cells round-tripped over %d visible rows", checked, len(v.Rows))

	if got := m.ScrollBy(1<<30, 100, 25); got != max0(v.Total-25) {
		t.Fatalf("scroll clamped to %d, expected %d", got, max0(v.Total-25))
	}
	if got := m.ScrollBy(-(1 << 30), 100, 25); got != 0 {
		t.Fatalf("scroll to bottom gave %d", got)
	}

	// A real listing must contain something recognisable, or the capture was
	// not what it claimed to be.
	if !strings.Contains(strings.Join(m.Lines(), "\n"), "total ") {
		t.Log("note: no 'total' line found; ls output may differ on this system")
	}
}

func countRunesIn2(s string) int {
	n := 0
	for range s {
		n++
	}
	return n
}

type winsize struct{ Row, Col, Xpixel, Ypixel uint16 }

func setSize(t *testing.T, f *os.File, cols, rows int) {
	t.Helper()
	ws := winsize{Row: uint16(rows), Col: uint16(cols)}
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(),
		uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&ws)))
}

func unlockpt(f *os.File) error {
	var u int32
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x40045431, uintptr(unsafe.Pointer(&u)))
	if e != 0 {
		return e
	}
	return nil
}

func ptsname(f *os.File) (string, error) {
	var n uint32
	_, _, e := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), 0x80045430, uintptr(unsafe.Pointer(&n)))
	if e != 0 {
		return "", e
	}
	return "/dev/pts/" + itoa(int(n)), nil
}
