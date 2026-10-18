//go:build linux

package editor

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// canInheritTTY reports whether stdin is a terminal that the external editor
// can use without acquiring it as a controlling terminal.
func canInheritTTY(stdin io.Reader) bool {
	f, ok := stdin.(*os.File)
	if !ok {
		return false
	}
	rc, err := f.SyscallConn()
	if err != nil {
		return false
	}
	inherits := false
	// Control keeps the descriptor's blocking mode intact, unlike File.Fd.
	_ = rc.Control(func(fd uintptr) {
		sid, err := unix.IoctlGetInt(int(fd), unix.TIOCGSID)
		if err != nil {
			// TIOCGSID reports ENOTTY when this process is detached from the
			// session that owns stdin's terminal (as the session daemon is).
			// We cannot safely acquire that terminal with TIOCSCTTY: the kernel
			// rejects it with EPERM. Keep the terminal descriptors inherited.
			if errors.Is(err, unix.ENOTTY) {
				_, termErr := unix.IoctlGetTermios(int(fd), unix.TCGETS)
				inherits = termErr == nil
			}
			return
		}
		own, err := unix.Getsid(0)
		inherits = err == nil && sid == own
	})
	return inherits
}
