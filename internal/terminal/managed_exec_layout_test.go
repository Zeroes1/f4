package terminal

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestManagedForegroundCommandInDirectoryAlwaysEmitsCompletionOnCdFailure(t *testing.T) {
	got := ManagedForegroundCommandInDirectory("'/root/private'", "'printf ok'")
	if !strings.Contains(got, "if cd '/root/private'; then") {
		t.Fatalf("directory guard missing: %q", got)
	}
	if !strings.Contains(got, `printf "\033]133;C\007\033]133;D\007"`) {
		t.Fatalf("cd failure does not emit completion markers: %q", got)
	}
	if !strings.Contains(got, ManagedForegroundCommand("'printf ok'")) {
		t.Fatalf("managed command missing from success arm: %q", got)
	}
}

func TestManagedForegroundCommandInElevatedDirectoryFallsBackToSudo(t *testing.T) {
	got := ManagedForegroundCommandInElevatedDirectory("'/root/private'", "'printf ok'")
	for _, want := range []string{
		"if cd '/root/private'; then",
		ManagedForegroundCommand("'printf ok'"),
		"sudo -- sh -c 'cd -- \"$1\" && eval \"$2\"' f4-sudo '/root/private' 'printf ok'",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in %q", want, got)
		}
	}
}

// runElevatedWrapper runs the wrapper in a real POSIX shell whose sudo is the
// given stub script and returns the shell's combined output and exit status.
func runElevatedWrapper(t *testing.T, sudoStub, dir, cmd string) (string, int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell wrapper")
	}
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("no POSIX shell")
	}
	// The wrapper starts with "set +H", which dash rejects: run it in bash, the
	// shell f4 starts, and keep sh for the stub's own command.
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("no bash")
	}
	bin := t.TempDir()
	if sudoStub != "" {
		if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte(sudoStub), 0o755); err != nil { //nolint:gosec // the sudo stub must be executable
			t.Fatal(err)
		}
	}
	wrapper := ManagedForegroundCommandInElevatedDirectory("'"+dir+"'", shellQuoteForTest(cmd))
	c := exec.Command(bash, "-c", wrapper) //nolint:gosec // the wrapper under test, run in a throwaway environment
	// Only the stub directory: a real sudo on this machine must not be found.
	if err := os.Symlink(sh, filepath.Join(bin, "sh")); err != nil {
		t.Skip("cannot link the shell:", err)
	}
	c.Env = []string{"PATH=" + bin}
	out, err := c.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

// shellQuoteForTest single-quotes s for a POSIX shell.
func shellQuoteForTest(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func TestElevatedDirectoryRunsCommandThroughSudoWhenCdIsRefused(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("windows has no POSIX modes, and root can enter any directory, so the plain cd is never refused")
	}
	dir := filepath.Join(t.TempDir(), "private")
	if err := os.Mkdir(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // let the temp directory be removed
	// The stub stands in for sudo: it drops "--", makes the directory
	// enterable (what root's access amounts to) and runs the rest as given.
	// Arguments after "--": sh -c <script> f4-sudo <dir> <command>.
	chmod, err := exec.LookPath("chmod")
	if err != nil {
		t.Skip("no chmod")
	}
	stub := "#!/bin/sh\nshift\n" + chmod + " 700 \"$5\"\nexec \"$@\"\n"
	out, code := runElevatedWrapper(t, stub, dir, "printf ran-elevated")
	if !strings.Contains(out, "ran-elevated") || code != 0 {
		t.Fatalf("command did not run through sudo: code=%d out=%q", code, out)
	}
	if !strings.Contains(out, "\x1b]133;C\a") || !strings.Contains(out, "\x1b]133;D\a") {
		t.Fatalf("completion markers missing: %q", out)
	}
}

func TestElevatedDirectoryWithoutSudoKeepsCdFailureStatus(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	out, code := runElevatedWrapper(t, "", missing, "printf never")
	if strings.Contains(out, "never") || code == 0 {
		t.Fatalf("command ran without a directory: code=%d out=%q", code, out)
	}
	if !strings.Contains(out, "\x1b]133;C\a\x1b]133;D\a") {
		t.Fatalf("completion markers missing: %q", out)
	}
}
