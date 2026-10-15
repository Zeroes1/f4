package vfs

import (
	"path/filepath"
	"testing"
	"time"
)

type fakeElevationTimer struct{}

func (fakeElevationTimer) Stop() bool { return true }

// f4#1768 step 6a (akunitsyn: Far drops the rights, f4 kept them until it
// exits): the elevated dispatcher is closed after ElevationIdle without a
// request, and the next operation asks the user again. A request in the
// meantime keeps it. The clock and the timer are driven by hand; UAC is a
// dispatcher started in this process.
func TestElevatedDispatcherIsClosedWhenIdle(t *testing.T) {
	oldLaunch, oldConfirm, oldUAC := launchElevated, confirmElevation, connectsByUAC
	oldAfter, oldNow, oldIdle := elevationAfterFunc, elevationNow, ElevationIdle
	t.Cleanup(func() {
		launchElevated, confirmElevation, connectsByUAC = oldLaunch, oldConfirm, oldUAC
		elevationAfterFunc, elevationNow, ElevationIdle = oldAfter, oldNow, oldIdle
	})
	t.Setenv("TMPDIR", "/tmp") // the socket path must stay short enough for AF_UNIX
	connectsByUAC = func() bool { return true }
	served := make(chan error, 4)
	launches, asked := 0, 0
	launchElevated = func(exe, sock, token string) error {
		launches++
		go func() { served <- RunElevatedDispatcher(sock, token, ElevatedFileOps) }()
		return nil
	}
	confirmElevation = func() bool {
		asked++
		return true
	}
	now := time.Unix(1_000_000, 0)
	elevationNow = func() time.Time { return now }
	var fire func()
	var armedFor time.Duration
	elevationAfterFunc = func(d time.Duration, f func()) elevationTimer {
		armedFor, fire = d, f
		return fakeElevationTimer{}
	}
	ElevationIdle = func() time.Duration { return 2 * time.Minute }

	c := &SudoClient{appPath: "f4.exe"}
	file := filepath.Join(t.TempDir(), "x.txt")
	if err := c.WriteAt(file, 0, []byte("a"), true, 0o600); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	if asked != 1 || launches != 1 || fire == nil || armedFor != 2*time.Minute {
		t.Fatalf("first request: asked %d, launched %d, timer for %v", asked, launches, armedFor)
	}

	// A request 90 s later: the timer armed at the first one comes 30 s
	// after it, sees it and waits the 90 s that are left.
	first := fire
	now = now.Add(90 * time.Second)
	if _, err := c.Stat(file); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	now = now.Add(30 * time.Second)
	first()
	if c.conn == nil || !c.consented || armedFor != 90*time.Second {
		t.Fatalf("closed %v, consent %v, timer for %v: want it kept and the timer for the 90 s left",
			c.conn == nil, c.consented, armedFor)
	}

	// Two minutes without a request: closed, the dispatcher exits, the
	// consent is gone.
	now = now.Add(90 * time.Second)
	fire()
	if c.conn != nil || c.consented {
		t.Fatalf("after the idle time: closed %v, consent %v", c.conn == nil, c.consented)
	}
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Fatal("the dispatcher did not exit when it was closed")
	}

	// The next operation asks again and starts a new dispatcher.
	if _, err := c.Stat(file); err != nil {
		t.Fatalf("Stat after the idle close: %v", err)
	}
	if asked != 2 || launches != 2 {
		t.Fatalf("after the idle close: asked %d, launched %d; want the question and UAC again", asked, launches)
	}

	// 0 keeps the dispatcher until f4 exits: no timer is armed.
	ElevationIdle = func() time.Duration { return 0 }
	fire = nil
	if _, err := c.Stat(file); err != nil || fire != nil {
		t.Fatalf("with 0: Stat %v, a timer was armed %v", err, fire != nil)
	}
	_ = c.conn.Close()
}
