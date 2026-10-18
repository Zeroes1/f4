//go:build !windows

package testutil

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

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
		// Linux names the file behind a descriptor; elsewhere a socket can
		// still say what it is bound and connected to, and a file is only
		// its number and kind.
		path, _ := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", fd))
		if path == "" && kind == "socket" {
			path = socketName(fd)
		}
		out = append(out, openFile{fd: fd, kind: kind, path: path})
	}
	return out
}

// socketName describes a socket by its own address and its peer's, where
// it has them.
func socketName(fd int) string {
	var parts []string
	if sa, err := unix.Getsockname(fd); err == nil {
		parts = append(parts, sockaddrString(sa))
	}
	if sa, err := unix.Getpeername(fd); err == nil {
		parts = append(parts, "-> "+sockaddrString(sa))
	}
	return strings.Join(parts, " ")
}

func sockaddrString(sa unix.Sockaddr) string {
	switch a := sa.(type) {
	case *unix.SockaddrUnix:
		return "unix:" + a.Name
	case *unix.SockaddrInet4:
		return net.JoinHostPort(net.IP(a.Addr[:]).String(), strconv.Itoa(a.Port))
	case *unix.SockaddrInet6:
		return net.JoinHostPort(net.IP(a.Addr[:]).String(), strconv.Itoa(a.Port))
	}
	return fmt.Sprintf("%T", sa)
}
