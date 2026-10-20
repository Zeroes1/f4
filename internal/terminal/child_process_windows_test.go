package terminal

import (
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

// A process created before the shell names the shell's PID as its parent
// only because its real parent died and the PID was handed to the shell: it
// is not the shell's child, and must not hold the shell's prompt.
func TestChildOfShellIgnoresAProcessOlderThanTheShell(t *testing.T) {
	const shell = int64(1_000_000)
	cases := []struct {
		name                   string
		child                  int64
		shellKnown, childKnown bool
		want                   bool
	}{
		{"created after the shell", shell + 1, true, true, true},
		{"created with the shell", shell, true, true, true},
		{"older than the shell", shell - 1, true, true, false},
		{"shell time unknown", shell - 1, false, true, true},
		{"child time unknown", shell - 1, true, false, true},
	}
	for _, c := range cases {
		if got := childOfShell(shell, c.shellKnown, c.child, c.childKnown); got != c.want {
			t.Errorf("%s: childOfShell = %v, want %v", c.name, got, c.want)
		}
	}
}

// The creation time of a real process can be read, both through a handle
// and by PID.
func TestProcessCreationTimeOfThisProcess(t *testing.T) {
	byPID, ok := openProcessCreationTime(uint32(os.Getpid())) //nolint:gosec // G115: our own PID
	if !ok || byPID <= 0 {
		t.Fatalf("creation time by PID = %d, %v", byPID, ok)
	}
	byHandle, ok := processCreationTime(windows.CurrentProcess())
	if !ok || byHandle != byPID {
		t.Fatalf("creation time by handle = %d, %v; by PID %d", byHandle, ok, byPID)
	}
}
