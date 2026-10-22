//go:build linux

package main

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// answers are what a real terminal sends back for the status reports.
var answers = []struct{ ask, reply string }{
	{"\x1b[5n", "\x1b[0n"},
	{"\x1b[6n", "\x1b[1;1R"},
	{"\x1b[c", "\x1b[?62;c"},
}

// answerChunk writes the replies to the requests that end in chunk; tail is the
// end of the previous chunk, because a request may be cut in two. It returns the
// tail for the next call.
func answerChunk(w io.Writer, tail, chunk []byte, asks []struct{ ask, reply string }) []byte {
	window := append(append([]byte(nil), tail...), chunk...)
	for _, a := range asks {
		for from := 0; ; {
			i := bytes.Index(window[from:], []byte(a.ask))
			if i < 0 {
				break
			}
			if from+i+len(a.ask) > len(tail) { // ends in the new bytes
				_, _ = w.Write([]byte(a.reply))
			}
			from += i + len(a.ask)
		}
	}
	if len(window) > 7 {
		return window[len(window)-7:]
	}
	return window
}

// far2lAck is what a terminal with the far2l extensions answers to the
// activation request (docs/FAR2L_COMPAT.md, P-05), before the answer to the DSR.
var far2lAck = struct{ ask, reply string }{"\x1b_far2l1\x1b\\", "\x1b_far2lok\a"}

// Record runs argv in a 80x25 pseudo-terminal, answers the status reports,
// sends F10 and Enter after wait and returns everything the program wrote and
// the length of the recording at the moment the quit keys were sent.
func Record(argv []string, wait time.Duration, opt Options) (Recording, error) {
	asks := answers
	if opt.Far2l {
		asks = append([]struct{ ask, reply string }{far2lAck}, answers...)
	}
	master, slave, err := openPTY(25, 80)
	if err != nil {
		return Recording{}, err
	}
	defer func() { _ = master.Close() }()
	defer func() { _ = slave.Close() }()
	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // running the command given on the command line is the point of the tool
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLUMNS=80", "LINES=25")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		return Recording{}, err
	}
	_ = slave.Close()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()

	var mu sync.Mutex
	var rec bytes.Buffer
	var lastOut time.Time // when the program wrote last
	var tail []byte       // the end of the previous chunk: a request may be cut in two
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 4096)
		for {
			k, err := master.Read(buf)
			if k > 0 {
				mu.Lock()
				rec.Write(buf[:k])
				lastOut = time.Now()
				tail = answerChunk(master, tail, buf[:k], asks)
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case <-time.After(wait):
	case <-done:
	}
	// A program that is still starting does not read keys yet: the quit keys
	// wait until it has been quiet for a moment (a cold start takes seconds).
	for deadline := time.Now().Add(settleLimit); time.Now().Before(deadline); {
		mu.Lock()
		quiet := time.Since(lastOut) >= settleQuiet && rec.Len() >= opt.MinOutput
		mu.Unlock()
		select {
		case <-done:
			deadline = time.Time{}
		case <-time.After(100 * time.Millisecond):
		}
		if quiet {
			break
		}
	}
	mu.Lock()
	mark := rec.Len()
	mu.Unlock()
	f10, enter := quitKeys(opt.Far2l)
	_, _ = master.Write([]byte(f10))
	time.Sleep(700 * time.Millisecond) // the confirmation opens after F10
	_, _ = master.Write([]byte(enter))
	sent := time.Now()
	killed := false
	select {
	case <-done:
	case <-time.After(exitLimit):
		killed = true
		// SIGQUIT makes a Go program print the stacks of its goroutines, which
		// shows what it was waiting for.
		_ = cmd.Process.Signal(syscall.SIGQUIT)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
		_ = cmd.Process.Kill()
		<-done
	}
	took := time.Since(sent)
	// Let the reader drain what is left, then stop it.
	time.Sleep(100 * time.Millisecond)
	_ = master.SetReadDeadline(time.Now())
	<-readDone
	mu.Lock()
	defer mu.Unlock()
	return Recording{Data: append([]byte(nil), rec.Bytes()...), Mark: mark, ExitTook: took, Killed: killed}, nil
}
