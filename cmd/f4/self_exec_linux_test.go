//go:build linux

package main

import (
	"os"
	"reflect"
	"testing"
)

// With the guard set, f4 must build the same argv goffi's bridge builds:
// loader, --preload, host libc, the image argv[0] names, then our arguments.
func TestSelfExecArgvUniversal(t *testing.T) {
	t.Setenv(goffiUniversalGuard, "1")

	loader, libc, ok := universalHostLoader()
	if !ok {
		t.Skip("host runs a libc goffi does not recognise; nothing to imitate")
	}

	name, argv := selfExecArgv("/usr/bin/f4", []string{"--server", "/tmp/f4.sock"})
	if name != loader {
		t.Errorf("program = %q, want the host loader %q", name, loader)
	}
	want := []string{"--preload", libc, os.Args[0], "--server", "/tmp/f4.sock"}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("args = %q, want %q", argv, want)
	}
}

// No guard, no detour: an ordinary build must not be sent through a loader.
func TestUniversalHostLoaderNeedsGuard(t *testing.T) {
	t.Setenv(goffiUniversalGuard, "")
	if loader, libc, ok := universalHostLoader(); ok {
		t.Errorf("universalHostLoader() = %q, %q, true without the guard set", loader, libc)
	}
}
