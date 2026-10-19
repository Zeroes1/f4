//go:build !windows

package vfs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// askpassServerGoroutines counts the askpass servers alive now.
func askpassServerGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "(*SudoClient).serveAskpass")
}

// A Connect that fails used to leave its askpass server behind: a goroutine
// blocked in Accept on a socket nothing would ever connect to, one more per
// attempt (found by goleak over internal/app's tests). Here sudo cannot be
// found at all, so Connect fails before it starts anything else.
func TestSudoConnectThatFailsLeavesNoAskpassServer(t *testing.T) {
	if connectsByUAC() {
		t.Skip("UAC connections run no askpass server")
	}
	t.Setenv("PATH", t.TempDir())
	c := &SudoClient{appPath: filepath.Join(t.TempDir(), "f4")}

	before := askpassServerGoroutines()
	if err := c.Connect(); err == nil {
		t.Fatal("Connect succeeded with no sudo to run")
	}
	if c.askpass != nil {
		t.Error("the askpass listener is still held after a failed Connect")
	}
	if _, err := os.Lstat(getAskpassSocketPath(os.Getpid())); err == nil {
		t.Error("the askpass socket file is still there after a failed Connect")
	}
	deadline := time.Now().Add(2 * time.Second)
	for askpassServerGoroutines() > before {
		if !time.Now().Before(deadline) {
			t.Fatal("the askpass server outlived the failed Connect")
		}
		time.Sleep(time.Millisecond)
	}
}
