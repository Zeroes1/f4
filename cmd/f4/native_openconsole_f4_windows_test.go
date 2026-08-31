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
		// Replay the native event sequence, including resize events. Feeding one
		// final raw blob would only test parsing after the fact and would never
		// exercise Windows reflow at the point where the host changed width.
		for _, event := range session.Events {
			switch event.Kind {
			case 2: // streamObservedOutput
				pf.consumeLocalOutput(pty, event.Bytes)
			case 4: // streamResize
				pf.ResizeConsole(event.Width, event.Height)
			}
		}
		logBytes := string(pf.termView.GetAllLogBytes())
		pf.Close()
		assertNativeF4Log(t, logBytes, report.ExpectedInput, session.InitialWidth, session.InitialHeight)
	}
}

func assertNativeF4Log(t *testing.T, logBytes string, expectedInput []byte, width, height int) {
	t.Helper()
	begin := "__F4_NATIVE_PROBE_BEGIN__"
	end := "__F4_NATIVE_PROBE_END__"
	if strings.Count(logBytes, begin) != 1 || strings.Count(logBytes, end) != 1 {
		t.Fatalf("native markers duplicated/lost for %dx%d: begin=%d end=%d", width, height, strings.Count(logBytes, begin), strings.Count(logBytes, end))
	}
	if strings.Index(logBytes, begin) >= strings.Index(logBytes, end) {
		t.Fatalf("native marker order is wrong for %dx%d", width, height)
	}
	expected := string(expectedInput)
	if !strings.Contains(expected, "\x1b[?1049h") || !strings.Contains(expected, "\x1b[?1049l") {
		t.Fatal("expected_input does not contain both alternate-screen transitions")
	}
	expectedLong := expectedLine(expected, "long: ")
	if expectedLong != "long: "+strings.Repeat("C", 257) {
		t.Fatalf("unexpected long-line oracle: %q", expectedLong)
	}
	clean := strings.ReplaceAll(logBytes, "\r", "")
	longLines := linesContaining(clean, "long: ")
	if len(longLines) != 1 || longLines[0] != expectedLong || strings.Count(clean, expectedLong) != 1 {
		t.Fatalf("long logical line changed for %dx%d: lines=%q", width, height, longLines)
	}
	checks := []struct {
		prefix string
		want   string
	}{
		{"ascii: ", expectedLine(expected, "ascii: ")},
		{"width-edge: ", expectedLine(expected, "width-edge: ")},
		{"unicode: ", expectedLine(expected, "unicode: ")},
		{"rewritten", "rewritten"},
		{"cursor: ", "twosor: one"},
		{"tabs: ", "tabs:   X       Y"},
		{"alternate-begin", "alternate-begin"},
	}
	for _, check := range checks {
		if check.want == "" {
			t.Fatalf("missing expected payload line %q", check.prefix)
		}
		if len(linesEqual(clean, check.want)) != 1 || (check.prefix != "rewritten" && !strings.Contains(clean, check.want)) {
			t.Fatalf("payload line %q was lost/changed for %dx%d", check.prefix, width, height)
		}
	}
	if len(linesContaining(clean, "repeat: SAME")) != 3 {
		t.Fatalf("repeated payload lines were coalesced/lost for %dx%d", width, height)
	}
	if strings.Contains(clean, "alt-screen") || strings.Contains(clean, "alternate-end") {
		t.Fatalf("alternate-screen contents leaked into primary history for %dx%d", width, height)
	}
}

func expectedLine(payload, prefix string) string {
	for _, line := range strings.Split(strings.ReplaceAll(payload, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, prefix) {
			return line
		}
	}
	return ""
}

func linesContaining(logBytes, needle string) []string {
	var result []string
	for _, line := range strings.Split(logBytes, "\n") {
		if strings.Contains(line, needle) {
			result = append(result, line)
		}
	}
	return result
}

func linesEqual(logBytes, want string) []string {
	var result []string
	for _, line := range strings.Split(logBytes, "\n") {
		if line == want {
			result = append(result, line)
		}
	}
	return result
}
