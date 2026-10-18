//go:build windows

package vfs

import (
	"github.com/unxed/vtui"
	"net"
	"os"
)

func RunSudoAskpass() {
	// Not used on Windows
	os.Exit(1)
}

// listenAskpass and serveAskpass are not used on Windows: Connect goes
// through UAC there and never reaches them.
func listenAskpass(string) (net.Listener, error) { return nil, nil }

func (c *SudoClient) serveAskpass(net.Listener) {}

// RunOnUI runs fn on the UI goroutine: the "retry as administrator"
// question before UAC (f4#1768).
func (c *SudoClient) RunOnUI(fn func()) {
	if vtui.FrameManager != nil {
		vtui.FrameManager.PostTask(fn)
	}
}
