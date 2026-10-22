//go:build linux

package main

import (
	"os"
	"strconv"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// openPTY opens a pseudo-terminal of the given size and returns both ends.
func openPTY(rows, cols uint16) (master, slave *os.File, err error) {
	mfd, err := unix.Open("/dev/ptmx", unix.O_RDWR|unix.O_NOCTTY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, nil, err
	}
	master = os.NewFile(uintptr(mfd), "/dev/ptmx")
	var n uint32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(mfd), unix.TIOCGPTN, uintptr(unsafe.Pointer(&n))); e != 0 {
		_ = master.Close()
		return nil, nil, e
	}
	var zero int32
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(mfd), unix.TIOCSPTLCK, uintptr(unsafe.Pointer(&zero))); e != 0 {
		_ = master.Close()
		return nil, nil, e
	}
	slave, err = os.OpenFile("/dev/pts/"+strconv.Itoa(int(n)), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		_ = master.Close()
		return nil, nil, err
	}
	if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: rows, Col: cols}); err != nil {
		_ = master.Close()
		_ = slave.Close()
		return nil, nil, err
	}
	return master, slave, nil
}
