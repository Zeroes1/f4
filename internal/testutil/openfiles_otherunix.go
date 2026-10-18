//go:build !windows && !darwin

package testutil

import "golang.org/x/sys/unix"

// systemSocket: no socket the system keeps open for the process is known
// here; see the darwin file for the ones libSystem holds.
func systemSocket(local, peer unix.Sockaddr) bool { return false }
