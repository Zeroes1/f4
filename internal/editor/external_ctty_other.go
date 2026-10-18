//go:build !windows && !freebsd && !linux

package editor

import "io"

// canInheritTTY is only implemented for Linux, where f4#1721 was
// reported (TIOCGSID is not available everywhere). Elsewhere the editor keeps
// getting its own session, as before.
func canInheritTTY(io.Reader) bool { return false }
