//go:build !windows

package terminal

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
)

// Job control (docs/FAR2L_COMPAT.md, P-02 (b); unxed/f4#2154). The terminal is
// in raw mode, so Ctrl+Z is a key and never a signal; but a SIGTSTP can still
// arrive (`kill -TSTP`, a job-control script, a terminal multiplexer), and the
// default action would stop f4 with the alternate screen, the mouse reports,
// bracketed paste and the keyboard protocols still on in the user's shell.
//
// Two shapes of process need two answers:
//
//   - The thin client the user starts (runClient) only waits; the daemon owns
//     its terminal through descriptors passed over a socket and keeps reading
//     it. A stopped client would leave the daemon drawing under the shell's
//     prompt and competing with the shell for the keys, so the client hands
//     SIGTSTP to the daemon, which detaches the client exactly as the
//     Background action does: the terminal is given back in order, the client
//     returns, and the session lives on in the daemon (run f4 to come back).
//   - A process that owns the terminal itself (runAttachedSession) gives it
//     back, stops with SIGSTOP, and takes it again on SIGCONT.

// daemonClientRunning is true while the daemon draws for an attached client.
var daemonClientRunning atomic.Bool

// forwardJobControlToDaemon hands SIGTSTP of the client to the daemon with
// the given pid. The returned function stops the forwarding.
func forwardJobControlToDaemon(serverPID int) (stop func()) {
	return forwardSignal(serverPID, syscall.SIGTSTP, syscall.Kill)
}

func forwardSignal(pid int, sig syscall.Signal, kill func(int, syscall.Signal) error) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, sig)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				_ = kill(pid, sig)
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}

// watchDaemonJobControl makes the daemon answer SIGTSTP by detaching the
// client that is attached (stopClient), and ignore it when there is none.
func watchDaemonJobControl(running func() bool, stopClient func()) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTSTP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				if running() {
					stopClient()
				}
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}

// installLocalJobControl is for a process that owns the terminal itself: on
// SIGTSTP it gives the terminal back (suspend), stops with SIGSTOP -- which
// cannot be caught, so the shell sees an ordinary stopped job -- and takes
// the terminal again when the shell continues it (resume).
func installLocalJobControl(suspend, resume func()) (stop func()) {
	return installJobControl(suspend, resume, func() { _ = syscall.Kill(os.Getpid(), syscall.SIGSTOP) })
}

func installJobControl(suspend, resume func(), stopSelf func()) (stop func()) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTSTP)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-ch:
				suspend()
				stopSelf() // returns when the process is continued
				resume()
			case <-done:
				return
			}
		}
	}()
	return func() {
		signal.Stop(ch)
		close(done)
	}
}
