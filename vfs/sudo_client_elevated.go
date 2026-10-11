package vfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// launchElevated starts the elevated dispatcher (LaunchElevatedDispatcher,
// UAC on Windows); a variable so a test can start one in-process instead.
var launchElevated = LaunchElevatedDispatcher

// elevatedConnectTimeout is how long Connect waits for the dispatcher to
// listen once UAC was asked: the user has to read and answer the prompt.
var elevatedConnectTimeout = 2 * time.Minute

// connectsByUAC reports that this platform elevates through UAC and the
// elevated dispatcher of f4#1768, not through sudo.
func connectsByUAC() bool { return runtime.GOOS == "windows" }

// connectElevatedLocked starts the elevated dispatcher with a fresh one-time
// token and connects to it (f4#1768, step 4). c.mu is held.
func (c *SudoClient) connectElevatedLocked() error {
	token, err := NewSudoToken()
	if err != nil {
		return err
	}
	sock := filepath.Join(os.TempDir(), fmt.Sprintf("f4-elevated-%d-%s.sock", os.Getpid(), token[:8]))
	if err := launchElevated(c.appPath, sock, token); err != nil {
		return err
	}
	deadline := time.Now().Add(elevatedConnectTimeout)
	for {
		conn, err := DialElevated(sock, token, 10*time.Second)
		if err == nil {
			c.conn, c.sockPath = conn, sock
			return nil
		}
		if errors.Is(err, ErrSudoAccessDenied) || time.Now().After(deadline) {
			return fmt.Errorf("the elevated dispatcher did not answer: %w", err)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// ReadAt reads up to size bytes of path at offset through the dispatcher;
// eof says the file ends there (CmdReadAt).
func (c *SudoClient) ReadAt(path string, offset int64, size int) (data []byte, eof bool, err error) {
	resp, _, err := c.SendRequest(SudoRequest{Cmd: CmdReadAt, Path: path, Offset: offset, Size: size})
	return resp.Data, resp.IsEOF, err
}

// WriteAt writes data into path at offset through the dispatcher, creating
// the file with mode when it is missing; truncate empties it first
// (CmdWriteAt).
func (c *SudoClient) WriteAt(path string, offset int64, data []byte, truncate bool, mode uint32) error {
	flags := 0
	if truncate {
		flags = SudoWriteTruncate
	}
	_, _, err := c.SendRequest(SudoRequest{Cmd: CmdWriteAt, Path: path, Offset: offset, Data: data, Flags: flags, Mode: mode})
	return err
}
