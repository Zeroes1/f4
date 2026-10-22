//go:build windows

package unpack

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

// holdRunning makes this test binary stand in for the f4 that runs an
// update: TestHelperHoldRunning keeps it running while a test replaces it.
const holdRunning = "F4_UNPACK_TEST_HOLD_RUNNING"

func TestHelperHoldRunning(t *testing.T) {
	if os.Getenv(holdRunning) != "1" {
		t.Skip("only runs as the program another test keeps running")
	}
	time.Sleep(5 * time.Minute)
}

// #2172: an update renames the running f4.exe to f4.exe.old, writes the new
// f4.exe, and cannot delete f4.exe.old afterwards: Windows does not delete
// the file of a running program, for an administrator either. That refusal
// went to the elevated dispatcher, whose question no window shows in
// `f4 --update`, and the update stood still for two minutes before going on.
func TestWriteFileSafeLeavesTheRunningProgramWithoutAskingForElevation(t *testing.T) {
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "f4.exe")
	copyFile(t, self, target)

	cmd := exec.Command(target, "-test.run=^TestHelperHoldRunning$") // #nosec G204 -- a copy of this test binary.
	cmd.Env = append(os.Environ(), holdRunning+"=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// As f4 sets it up for every process, `f4 --update` included.
	old := vfs.SetSudoClientForTest(nil)
	t.Cleanup(func() { vfs.SetSudoClientForTest(old) })
	vfs.InitSudoClient(target, "")

	done := make(chan error, 1)
	go func() { done <- writeFileSafe(target, strings.NewReader("new f4"), 0755) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("writeFileSafe() = %v, want the new build written", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("writeFileSafe is still waiting after 30s: the running program's .old went to elevation")
	}

	if got, _ := os.ReadFile(target); string(got) != "new f4" {
		t.Errorf("%s = %q, want the new build", target, got)
	}
	if _, err := os.Stat(target + ".old"); err != nil {
		t.Errorf("the running program's file is gone: %v", err)
	}
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src) // #nosec G304 -- the test binary itself.
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst) // #nosec G304 -- a path inside t.TempDir().
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}
