package vfs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// f4#1768 step 4: the client starts the elevated dispatcher with a fresh
// token, connects once it listens and then works through it. UAC is replaced
// by a dispatcher started in this process.
func TestSudoClientConnectsToTheElevatedDispatcher(t *testing.T) {
	oldLaunch := launchElevated
	t.Cleanup(func() { launchElevated = oldLaunch })
	served := make(chan error, 1)
	launchElevated = func(exe, sock, token string) error {
		if exe != "f4.exe" || len(token) != 2*sudoTokenBytes {
			t.Errorf("launched %q with token %q", exe, token)
		}
		go func() { served <- RunElevatedDispatcher(sock, token, ElevatedFileOps) }()
		return nil
	}
	// The socket path must stay short enough for AF_UNIX.
	t.Setenv("TMPDIR", "/tmp")

	c := &SudoClient{appPath: "f4.exe"}
	c.mu.Lock()
	err := c.connectElevatedLocked()
	c.mu.Unlock()
	if err != nil {
		t.Fatalf("connect: %v", err)
	}

	file := filepath.Join(t.TempDir(), "x.txt")
	if err := c.WriteAt(file, 0, []byte("abc"), true, 0o600); err != nil {
		t.Fatalf("WriteAt: %v", err)
	}
	item, err := c.Stat(file)
	if err != nil || item.Size != 3 {
		t.Fatalf("Stat = %+v, %v", item, err)
	}
	data, eof, err := c.ReadAt(file, 1, 10)
	if err != nil || string(data) != "bc" || !eof {
		t.Fatalf("ReadAt = %q eof %v, %v", data, eof, err)
	}
	if _, err := c.Stat(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat of a missing file = %v, want not-exist", err)
	}

	_ = c.conn.Close()
	select {
	case <-served:
	case <-time.After(5 * time.Second):
		t.Error("the dispatcher did not stop when its client went away")
	}
}

func TestSudoClientElevatedConnectFailures(t *testing.T) {
	oldLaunch, oldTimeout := launchElevated, elevatedConnectTimeout
	t.Cleanup(func() { launchElevated, elevatedConnectTimeout = oldLaunch, oldTimeout })
	t.Setenv("TMPDIR", "/tmp")

	launchElevated = func(string, string, string) error { return errors.New("UAC elevation was canceled") }
	c := &SudoClient{appPath: "f4.exe"}
	c.mu.Lock()
	err := c.connectElevatedLocked()
	c.mu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "canceled") {
		t.Errorf("a canceled prompt gave %v", err)
	}

	// UAC accepted, but nothing ever listens.
	launchElevated = func(string, string, string) error { return nil }
	elevatedConnectTimeout = 300 * time.Millisecond
	c.mu.Lock()
	err = c.connectElevatedLocked()
	c.mu.Unlock()
	if err == nil || c.conn != nil {
		t.Errorf("a dispatcher that never listened gave %v", err)
	}
}
