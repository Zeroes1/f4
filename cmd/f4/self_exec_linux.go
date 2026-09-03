//go:build linux

package main

import (
	"os"

	"github.com/go-webgpu/goffi/ffi"
)

// goffiUniversalGuard is the variable goffi's universal ("Profile U") bridge
// adds to the environment of the copy of the process it re-execs through the
// host dynamic loader. Its presence says two things: this process reached a
// libc that way, and a child that inherits the variable will not be given one,
// because the bridge in the child reads the guard and returns instead of
// re-execing.
//
// The name is goffi's, and a rename there would turn the check below into a
// no-op rather than into a wrong answer: f4 would go back to starting children
// by path, which is the behaviour that made them die before main.
const goffiUniversalGuard = "GOFFI_UNIVERSAL_REEXEC"

// universalHostLoader reports the host dynamic loader and libc SONAME that
// copies of this process must be started through, and whether that applies at
// all. It does only when this process itself came up that way; an ordinary
// dynamic or static build wants none of it.
func universalHostLoader() (loader, libc string, ok bool) {
	if os.Getenv(goffiUniversalGuard) == "" {
		return "", "", false
	}
	// The same table the bridge used, through goffi's public accessors: an
	// empty answer means the host runs a libc goffi does not recognise, and
	// then there was no re-exec to imitate.
	loader, libc = ffi.HostLoader(), ffi.HostLibC()
	if loader == "" || libc == "" {
		return "", "", false
	}
	return loader, libc, true
}
