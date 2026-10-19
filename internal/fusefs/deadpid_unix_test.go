//go:build !windows

package fusefs

import (
	"os/exec"
	"testing"
)

// deadPID runs something and reaps it. Unix hands PIDs out in sequence and
// comes back to one only after the counter wraps, so the PID stays free for
// the test.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Run(); err != nil {
		t.Skipf("cannot produce a dead pid here: %v", err)
	}
	return cmd.Process.Pid
}
