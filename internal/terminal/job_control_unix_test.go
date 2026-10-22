//go:build !windows

package terminal

import (
	"os"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// SIGTSTP must not leave the user's shell with f4's terminal modes
// (docs/FAR2L_COMPAT.md, P-02 (b); unxed/f4#2154). The tests send the signal
// to the test process itself; it is caught, so nothing stops.

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for i := 0; i < 500; i++ {
		if cond() {
			return
		}
		time.Sleep(4 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestJobControlGivesTheTerminalBackStopsAndTakesItAgain(t *testing.T) {
	events := make(chan string, 8)
	stop := installJobControl(
		func() { events <- "suspend" },
		func() { events <- "resume" },
		func() { events <- "stop" },
	)
	defer stop()

	if err := syscall.Kill(os.Getpid(), syscall.SIGTSTP); err != nil {
		t.Fatal(err)
	}
	var got []string
	for len(got) < 3 {
		select {
		case e := <-events:
			got = append(got, e)
		case <-time.After(2 * time.Second):
			t.Fatalf("got only %v", got)
		}
	}
	if got[0] != "suspend" || got[1] != "stop" || got[2] != "resume" {
		t.Fatalf("order of events: %v, want suspend, stop, resume", got)
	}
}

func TestJobControlIsOffAfterStop(t *testing.T) {
	events := make(chan string, 8)
	stop := installJobControl(func() { events <- "suspend" }, func() {}, func() {})
	stop()
	// another handler keeps SIGTSTP from stopping the test process
	other := installJobControl(func() {}, func() {}, func() {})
	defer other()
	_ = syscall.Kill(os.Getpid(), syscall.SIGTSTP)
	select {
	case e := <-events:
		t.Fatalf("a stopped handler still ran: %s", e)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestForwardSignalHandsSIGTSTPToTheDaemon(t *testing.T) {
	type call struct {
		pid int
		sig syscall.Signal
	}
	calls := make(chan call, 4)
	stop := forwardSignal(4242, syscall.SIGTSTP, func(pid int, sig syscall.Signal) error {
		calls <- call{pid, sig}
		return nil
	})
	defer stop()
	_ = syscall.Kill(os.Getpid(), syscall.SIGTSTP)
	select {
	case c := <-calls:
		if c.pid != 4242 || c.sig != syscall.SIGTSTP {
			t.Fatalf("forwarded %+v, want SIGTSTP to 4242", c)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SIGTSTP of the client was not forwarded")
	}
}

func TestDaemonJobControlDetachesOnlyAnAttachedClient(t *testing.T) {
	var running atomic.Bool
	detached := make(chan struct{}, 4)
	stop := watchDaemonJobControl(running.Load, func() { detached <- struct{}{} })
	defer stop()

	_ = syscall.Kill(os.Getpid(), syscall.SIGTSTP)
	select {
	case <-detached:
		t.Fatal("SIGTSTP with no client attached detached something")
	case <-time.After(150 * time.Millisecond):
	}

	running.Store(true)
	_ = syscall.Kill(os.Getpid(), syscall.SIGTSTP)
	waitFor(t, "the client to be detached", func() bool { return len(detached) > 0 })
}
