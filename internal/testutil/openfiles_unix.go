//go:build !windows

package testutil

import (
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// openFiles lists the regular files and sockets this process has open, by
// probing every descriptor number up to the soft limit. Only those two kinds
// count: a pipe, a pseudo-terminal, /dev/urandom or the runtime's kqueue and
// epoll descriptors are opened once by the runtime or the standard library
// and kept, so they would make every run look like a leak.
func openFiles() []openFile {
	limit := 4096
	var rl unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &rl); err == nil && rl.Cur < uint64(limit) {
		limit = int(rl.Cur) // #nosec G115 -- below 4096 by the comparison above
	}
	var out []openFile
	for fd := 0; fd < limit; fd++ {
		var st unix.Stat_t
		if err := unix.Fstat(fd, &st); err != nil {
			continue
		}
		var kind string
		switch st.Mode & unix.S_IFMT {
		case unix.S_IFREG:
			kind = "file"
		case unix.S_IFSOCK:
			kind = "socket"
		default:
			continue
		}
		// Linux names the file behind a descriptor; elsewhere the number
		// and the kind are all there is.
		path, _ := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
		out = append(out, openFile{fd: fd, kind: kind, path: path})
	}
	return out
}
