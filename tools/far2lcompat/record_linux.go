//go:build linux

package main

import (
	"bytes"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

// answers are what a real terminal sends back for the status reports.
var answers = []struct{ ask, reply string }{
	{"\x1b[5n", "\x1b[0n"},
	{"\x1b[6n", "\x1b[1;1R"},
	{"\x1b[c", "\x1b[?62;c"},
}

// Record runs argv in a 80x25 pseudo-terminal, answers the status reports,
// sends F10 and Enter after wait and returns everything the program wrote and
// the length of the recording at the moment the quit keys were sent.
func Record(argv []string, wait time.Duration) ([]byte, int, error) {
	mfd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, 0, err
	}
	master := os.NewFile(uintptr(mfd), "/dev/ptmx")
	defer func() { _ = master.Close() }()
	var n uint32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(mfd), unix.TIOCGPTN, uintptr(unsafe.Pointer(&n))); e != 0 {
		return nil, 0, e
	}
	var zero int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(mfd), unix.TIOCSPTLCK, uintptr(unsafe.Pointer(&zero))); e != 0 {
		return nil, 0, e
	}
	slave, err := os.OpenFile("/dev/pts/"+itoa(int(n)), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = slave.Close() }()
	ws := &unix.Winsize{Row: 25, Col: 80}
	if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, ws); err != nil {
		return nil, 0, err
	}
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // running the command given on the command line is the point of the tool
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLUMNS=80", "LINES=25")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		return nil, 0, err
	}
	_ = slave.Close()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()

	var mu sync.Mutex
	var rec bytes.Buffer
	var tail []byte // the end of the previous chunk: a request may be cut in two
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			k, err := master.Read(buf)
			if k > 0 {
				mu.Lock()
				rec.Write(buf[:k])
				window := append(append([]byte(nil), tail...), buf[:k]...)
				for _, a := range answers {
					for from := 0; ; {
						i := bytes.Index(window[from:], []byte(a.ask))
						if i < 0 {
							break
						}
						if from+i+len(a.ask) > len(tail) { // ends in the new bytes
							_, _ = master.Write([]byte(a.reply))
						}
						from += i + len(a.ask)
					}
				}
				if len(window) > 7 {
					tail = window[len(window)-7:]
				} else {
					tail = window
				}
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-time.After(wait):
	case <-done:
	}
	mu.Lock()
	mark := rec.Len()
	mu.Unlock()
	_, _ = master.Write([]byte("\x1b[21~\r"))
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
	// Let the reader drain what is left, then stop it.
	time.Sleep(100 * time.Millisecond)
	_ = master.SetReadDeadline(time.Now())
	<-readDone
	mu.Lock()
	defer mu.Unlock()
	return append([]byte(nil), rec.Bytes()...), mark, nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}
