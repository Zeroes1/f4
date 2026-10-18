//go:build linux

package editor

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/unxed/f4/internal/gui"
	"github.com/unxed/f4/internal/terminal"
)

const editorCTTYHelperEnv = "F4_TEST_EDITOR_CTTY_HELPER"
const editorCTTYDetachedHelperEnv = "F4_TEST_EDITOR_CTTY_DETACHED_HELPER"

// TestExternalEditorCTTYHelper is not a test by itself: it is the "f4 running
// in an xterm" half of TestExternalEditorStartsOnOwnControllingTTY. It only
// acts when re-executed by that test.
func TestExternalEditorCTTYHelper(t *testing.T) {
	report := os.Getenv(editorCTTYHelperEnv)
	if report == "" {
		t.Skip("helper process, run by TestExternalEditorStartsOnOwnControllingTTY")
	}
	gui.Running = false
	if os.Getuid() == 0 {
		// Go asks the kernel to *steal* the terminal (TIOCSCTTY, arg 1), which
		// root may do and a regular user may not. Behave like a regular user.
		if err := dropToNobody(); err != nil {
			_ = os.WriteFile(report, []byte(err.Error()), 0o666)
			os.Exit(4)
		}
	}

	cmd := exec.Command("sh", "-c", "test -t 0 && test -c /dev/tty")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	ConfigureExternalEditorProcess(cmd)
	if err := cmd.Run(); err != nil {
		_ = os.WriteFile(report, []byte(err.Error()), 0o666)
		os.Exit(3)
	}
	os.Exit(0)
}

// TestExternalEditorDetachedCTTYHelper is the detached session-daemon half of
// TestExternalEditorStartsWithInheritedTTY. Its stdin is a terminal owned by
// the caller's session, but this process has no controlling terminal of its
// own, so TIOCGSID returns ENOTTY.
func TestExternalEditorDetachedCTTYHelper(t *testing.T) {
	report := os.Getenv(editorCTTYDetachedHelperEnv)
	if report == "" {
		t.Skip("helper process, run by TestExternalEditorStartsWithInheritedTTY")
	}
	gui.Running = false
	if os.Getuid() == 0 {
		if err := dropToNobody(); err != nil {
			_ = os.WriteFile(report, []byte(err.Error()), 0o666)
			os.Exit(4)
		}
	}

	cmd := exec.Command("sh", "-c", "test -t 0")
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	ConfigureExternalEditorProcess(cmd)
	if err := cmd.Run(); err != nil {
		_ = os.WriteFile(report, []byte(err.Error()), 0o666)
		os.Exit(3)
	}
	os.Exit(0)
}

// f4#1721: when f4 runs on a terminal of its own session (a plain xterm), a
// console editor must start on that same terminal. Giving it a new session
// plus TIOCSCTTY fails with "fork/exec ...: operation not permitted".
func TestExternalEditorStartsOnOwnControllingTTY(t *testing.T) {
	pty, err := terminal.NewPTY()
	if err != nil {
		t.Skipf("pseudo-terminal unavailable: %v", err)
	}
	defer pty.Close()

	// Directly under the world-writable temp dir, so that the helper can reach
	// it even after it drops to an unprivileged user.
	dir, err := os.MkdirTemp("", "f4-ctty-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	_ = os.Chmod(dir, 0o777)
	report := filepath.Join(dir, "report")
	// The helper becomes a session leader whose controlling terminal is the
	// PTY, exactly like f4 started from a shell in a terminal emulator.
	helper := exec.Command(os.Args[0], "-test.run=^TestExternalEditorCTTYHelper$")
	helper.Env = append(os.Environ(), editorCTTYHelperEnv+"="+report)
	helper.Stdin, helper.Stdout, helper.Stderr = pty.Slave, pty.Slave, pty.Slave
	helper.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := helper.Run(); err != nil {
		msg, _ := os.ReadFile(report)
		t.Fatalf("editor could not start on f4's own terminal: %v (%s)", err, msg)
	}
}

// f4#2076: the session daemon is detached from the user's session, but still
// has the user's terminal on stdin. It must not try to steal that terminal
// with Setsid+Setctty, which fails with EPERM.
func TestExternalEditorStartsWithInheritedTTY(t *testing.T) {
	pty, err := terminal.NewPTY()
	if err != nil {
		t.Skipf("pseudo-terminal unavailable: %v", err)
	}
	defer pty.Close()

	dir, err := os.MkdirTemp("", "f4-detached-ctty-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	_ = os.Chmod(dir, 0o777)
	report := filepath.Join(dir, "report")
	helper := exec.Command(os.Args[0], "-test.run=^TestExternalEditorDetachedCTTYHelper$")
	helper.Env = append(os.Environ(), editorCTTYDetachedHelperEnv+"="+report)
	helper.Stdin, helper.Stdout, helper.Stderr = pty.Slave, pty.Slave, pty.Slave
	// Match the session daemon: detach the process without taking ownership of
	// the terminal that its inherited file descriptors still reference.
	helper.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := helper.Run(); err != nil {
		msg, _ := os.ReadFile(report)
		t.Fatalf("external editor could not inherit the detached daemon's terminal: %v (%s)", err, msg)
	}
}

func dropToNobody() error {
	const nobody = 65534
	if err := syscall.Setgroups(nil); err != nil {
		return err
	}
	if err := syscall.Setgid(nobody); err != nil {
		return err
	}
	return syscall.Setuid(nobody)
}
