package testutil

import "golang.org/x/sys/unix"

// mDNSResponderSocket is where macOS's resolver lives. Go on darwin asks
// libSystem to resolve names even without cgo, and libSystem keeps its
// connection to the responder, with a kernel control socket beside it, for
// the rest of the process: the first test to look up a host opens both.
const mDNSResponderSocket = "/var/run/mDNSResponder"

// systemSocket reports whether a socket with these addresses is one of the
// two libSystem keeps (see mDNSResponderSocket). Either may be nil.
func systemSocket(local, peer unix.Sockaddr) bool {
	for _, sa := range []unix.Sockaddr{local, peer} {
		switch a := sa.(type) {
		case *unix.SockaddrCtl:
			return true
		case *unix.SockaddrUnix:
			if a.Name == mDNSResponderSocket {
				return true
			}
		}
	}
	return false
}
