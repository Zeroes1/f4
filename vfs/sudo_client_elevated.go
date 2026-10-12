package vfs

import (
	"errors"
	"fmt"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
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

// confirmElevation asks the user whether to retry an operation as
// administrator before UAC is asked; a variable so a test can answer it.
var confirmElevation = askElevationOnUI

// ErrElevationDeclined: the user did not agree to elevate.
var ErrElevationDeclined = errors.New("elevation declined")

// askElevationOnUI shows the question on the UI goroutine and waits for the
// answer, at most elevatedConnectTimeout; like the sudo password dialog, it
// must not be called on the UI goroutine itself.
func askElevationOnUI() bool {
	if vtui.FrameManager == nil {
		return false
	}
	answer := make(chan bool, 1)
	globalSudoClient.RunOnUI(func() {
		dlg := vtui.ShowMessageEx(i18n.Msg("Elevation.Title"), i18n.Msg("Elevation.Text"),
			[]string{i18n.Msg("Elevation.Continue"), vtui.Msg("vtui.Cancel")}, vtui.MessageWarn)
		dlg.OnResult = func(code int) {
			select {
			case answer <- code == 0:
			default:
			}
		}
	})
	select {
	case ok := <-answer:
		return ok
	case <-time.After(elevatedConnectTimeout):
		return false
	}
}

// connectsByUAC reports that this platform elevates through UAC and the
// elevated dispatcher of f4#1768, not through sudo.
func connectsByUAC() bool { return runtime.GOOS == "windows" }

// connectElevatedLocked starts the elevated dispatcher with a fresh one-time
// token and connects to it (f4#1768, step 4). c.mu is held.
func (c *SudoClient) connectElevatedLocked() error {
	if !c.consented {
		if !confirmElevation() {
			return ErrElevationDeclined
		}
		c.consented = true
	}
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
