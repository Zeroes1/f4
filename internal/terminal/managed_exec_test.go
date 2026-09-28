package terminal

import (
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// waitForPTYCondition drains whatever the PTY prints into out while polling
// cond, until cond is true or timeout elapses. It returns cond's final
// value, so callers can tell a satisfied wait from a timed-out one.
func waitForPTYCondition(p *PTY, out *strings.Builder, timeout time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 4096)
	for {
		if cond() {
			return true
		}
		_ = p.Master.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		if n, err := p.Master.Read(buf); n > 0 {
			out.Write(buf[:n])
		} else if err != nil && !errors.Is(err, os.ErrDeadlineExceeded) {
			return cond()
		}
		if time.Now().After(deadline) {
			return cond()
		}
	}
}

// TestManagedForegroundCommand_JobControlStopLeavesNoDMarker pins down,
// against a real PTY and a real interactive shell (not a mock), the
// mechanism behind f4 #1603: a foreground command run from f4's own command
// line is wrapped by ManagedForegroundCommand in OSC 133 C/D markers so f4
// knows when it is done (internal/panel/frame.go's BeginManagedExecution /
// endExecution, gated on the D marker via shellBusyChanged).
//
// If the wrapped command is job-control *stopped* instead of finishing --
// Ctrl+Z, i.e. a real SIGTSTP delivered through the PTY exactly as f4's own
// key dispatch already does (see the CI run cited in the #1603 comment
// thread) -- the interactive shell abandons the rest of that compound
// command and returns straight to its own prompt: the D marker never runs.
// This test confirms that end-to-end:
//
//  1. the D marker is indeed never printed after the stop, and
//  2. PTY.IsBusy() -- the TIOCGPGRP check pty_linux.go/pty_darwin.go/
//     pty_bsd.go already use, independent of any marker -- correctly
//     notices the shell reclaiming the terminal's foreground process group,
//     flipping back to false on its own.
//
// (2) is the empirical groundwork f4 #1603's real fix needs: today
// PanelsFrame.IsPtyBusy() ORs this same IsBusy() with pf.Executing, so a
// stuck-true pf.Executing (no D marker ever arriving) wins regardless. A
// fix has to notice IsBusy() going false while pf.Executing is still true
// and treat that as "the job stopped", but doing that safely needs a
// separate step: right after BeginManagedExecution() there is a real
// window where pf.Executing is already true but the wrapped command hasn't
// forked yet, so IsBusy() also reads false there -- debouncing that race is
// left to the follow-up (see the #1603 comment thread), not this test.
func TestManagedForegroundCommand_JobControlStopLeavesNoDMarker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("PTY job control is a Unix concept; not meaningful on Windows")
	}
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}

	p, err := NewPTY()
	if err != nil {
		t.Fatalf("NewPTY: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	// No args: this is exactly how GetSystemShell()'s result gets started in
	// internal/panel/frame.go (InitPTY -> p.Run(shell)) -- bash detects it is
	// interactive itself (tty on stdin/stdout, no -c) and turns job control
	// (monitor mode) on, which is the whole mechanism under test.
	if err := p.Run("bash"); err != nil {
		t.Fatalf("Run(bash): %v", err)
	}

	// "sleep 30" stands in for the reported python3 REPL: a foreground
	// program that blocks instead of returning, the shape #1603 is about.
	wire := " " + ManagedForegroundCommand("'sleep 30'") + "\r"
	if _, err := p.Write([]byte(wire)); err != nil {
		t.Fatalf("writing the managed command: %v", err)
	}

	var out strings.Builder
	if !waitForPTYCondition(p, &out, 5*time.Second, func() bool {
		return strings.Contains(out.String(), "\x1b]133;C\x07")
	}) {
		t.Fatalf("never saw the C marker; PTY output so far: %q", out.String())
	}
	if !waitForPTYCondition(p, &out, 5*time.Second, p.IsBusy) {
		t.Fatalf("sleep never became the foreground job (IsBusy never went true); PTY output so far: %q", out.String())
	}

	// Ctrl+Z: the exact control byte f4's own key dispatch already writes to
	// the master for this key (confirmed reaching the PTY in the CI run
	// cited in the #1603 comment thread).
	if _, err := p.Write([]byte{0x1a}); err != nil {
		t.Fatalf("sending Ctrl+Z: %v", err)
	}

	if !waitForPTYCondition(p, &out, 5*time.Second, func() bool { return !p.IsBusy() }) {
		t.Fatalf("shell never reclaimed the terminal after Ctrl+Z (IsBusy stayed true); PTY output so far: %q", out.String())
	}

	// Give the shell a little longer to print anything else it's going to,
	// including -- if the premise here were wrong -- a belated D marker.
	waitForPTYCondition(p, &out, 500*time.Millisecond, func() bool { return false })

	if strings.Contains(out.String(), "\x1b]133;D\x07") {
		t.Fatalf("D marker was printed after a job-control stop; f4 #1603's premise (pf.Executing never clears) does not hold here. PTY output: %q", out.String())
	}
}
