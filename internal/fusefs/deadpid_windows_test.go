package fusefs

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

// deadPID returns the PID of a process that has exited, and keeps it that
// process's for the rest of the test. Windows hands a freed PID out again
// quickly - a reaped child's PID named a stranger by the time it was looked
// up (#2102 CI) - but not while a handle to the exited process is open. The
// handle is taken before the child is waited for, while exec still holds
// its own, so it is certain to be this child's.
func deadPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("cmd", "/c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot produce a dead pid here: %v", err)
	}
	pid := cmd.Process.Pid
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid)) //nolint:gosec // G115: a PID exec just gave us
	if err != nil {
		_ = cmd.Wait()
		t.Fatalf("cannot hold the child's PID: %v", err)
	}
	t.Cleanup(func() { _ = windows.CloseHandle(h) })
	if err := cmd.Wait(); err != nil {
		t.Fatalf("child: %v", err)
	}
	return pid
}

// A process that has exited is dead, though a handle to it keeps it there to
// be opened.
func TestProcessExistsSeesThroughAHeldHandle(t *testing.T) {
	if processExists(deadPID(t)) {
		t.Fatal("an exited process must not count as running")
	}
}
