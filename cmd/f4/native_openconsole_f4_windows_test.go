//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type nativeOpenConsoleReplayPTY struct{}

func (nativeOpenConsoleReplayPTY) Read([]byte) (int, error)       { return 0, os.ErrClosed }
func (nativeOpenConsoleReplayPTY) Write(data []byte) (int, error) { return len(data), nil }
func (nativeOpenConsoleReplayPTY) Close() error                   { return nil }
func (nativeOpenConsoleReplayPTY) SetSize(int, int)               {}
func (nativeOpenConsoleReplayPTY) Wait() error                    { return nil }
func (nativeOpenConsoleReplayPTY) Run(string, ...string) error    { return nil }
func (nativeOpenConsoleReplayPTY) IsBusy() bool                   { return false }

func TestNativeOpenConsoleF4Pipeline(t *testing.T) {
	if os.Getenv("F4_NATIVE_OPENCONSOLE") != "1" {
		t.Skip("set F4_NATIVE_OPENCONSOLE=1 to run the pinned native OpenConsole integration gate")
	}
	reportPath := filepath.Join(t.TempDir(), "native-openconsole-f4.json")
	report, err := runNativeOpenConsoleF4Probe(reportPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Sessions) == 0 {
		t.Fatal("native probe returned no sessions")
	}

	for _, session := range report.Sessions {
		pf := NewPanelsFrame()
		pty := nativeOpenConsoleReplayPTY{}
		pf.ptyMutex.Lock()
		pf.pty = pty
		pf.termView.pty = pty
		pf.parser.pty = pty
		pf.ptyMutex.Unlock()
		pf.termView.Resize(session.InitialWidth, session.InitialHeight)
		pf.consumeLocalOutput(pty, session.RawOutput)
		logBytes := string(pf.termView.GetAllLogBytes())
		pf.Close()
		if !strings.Contains(logBytes, "__F4_NATIVE_PROBE_BEGIN__") || !strings.Contains(logBytes, "__F4_NATIVE_PROBE_END__") {
			t.Fatalf("f4 pipeline lost native markers for %dx%d: %q", session.InitialWidth, session.InitialHeight, logBytes)
		}
		if !strings.Contains(logBytes, "long: ") {
			t.Fatalf("f4 pipeline lost long-line payload for %dx%d", session.InitialWidth, session.InitialHeight)
		}
	}
}
