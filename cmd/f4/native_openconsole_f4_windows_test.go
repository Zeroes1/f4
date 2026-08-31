//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
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

// TestNativeOpenConsoleF4Live owns a real pinned ConPTY session from the f4
// package itself. It is intentionally opt-in because it starts cmd.exe and
// exercises the machine's pinned host, but unlike the replay diagnostic above
// it fails hard when that host is unavailable.
func TestNativeOpenConsoleF4Live(t *testing.T) {
	if os.Getenv("F4_NATIVE_OPENCONSOLE") != "1" {
		t.Skip("set F4_NATIVE_OPENCONSOLE=1 to run the live pinned OpenConsole gate")
	}
	pty, err := newNativeOpenConsolePTY(80, 25)
	if err != nil {
		t.Fatal(err)
	}
	pf := NewPanelsFrame()
	pf.ptyMutex.Lock()
	pf.pty = pty
	pf.termView.pty = pty
	pf.parser.pty = pty
	pf.ptyMutex.Unlock()
	defer pf.Close()
	long := "long: " + strings.Repeat("C", 257)
	command := fmt.Sprintf("echo __F4_NATIVE_LIVE_BEGIN__ & echo ascii: LIVE & echo repeat: SAME & echo repeat: SAME & echo repeat: SAME & ping -n 2 127.0.0.1 >nul & echo %s & ping -n 2 127.0.0.1 >nul & echo __F4_NATIVE_LIVE_END__", long)
	if err := pty.Run("cmd.exe", "/d", "/c", command); err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 127) // deliberately crosses arbitrary read boundaries
		resized := false
		total := 0
		for {
			n, readErr := pty.Read(buf)
			if n > 0 {
				total += n
				pf.consumeLocalOutput(pty, buf[:n])
				if !resized {
					// Resize while output is active, then restore the original size.
					pf.ResizeConsole(41, 25)
					pf.ResizeConsole(80, 25)
					resized = true
				}
			}
			if readErr != nil {
				t.Logf("live native bytes read=%d", total)
				readDone <- readErr
				return
			}
		}
	}()
	if err := pty.Wait(); err != nil {
		t.Fatal(err)
	}
	if code, err := pty.ExitCode(); err != nil || code != 0 {
		t.Fatalf("live child exit code=%d err=%v", code, err)
	}
	pty.shutdownHost()
	if err := <-readDone; err != nil && !errors.Is(err, os.ErrClosed) && !errors.Is(err, windows.ERROR_BROKEN_PIPE) && !errors.Is(err, windows.ERROR_HANDLE_EOF) {
		t.Fatal(err)
	}
	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}
	logBytes := strings.ReplaceAll(string(pf.termView.GetAllLogBytes()), "\r", "")
	t.Logf("live log bytes=%d gridHistory=%d viewport=%dx%d cursor=%d,%d", len(logBytes), len(pf.termView.GridHistory), pf.termView.Width, pf.termView.Height, pf.termView.CursorX, pf.termView.CursorY)
	assertLiveNativeLog(t, logBytes)
}

func TestNativeOpenConsoleF4WidthAndWhitespace(t *testing.T) {
	if os.Getenv("F4_NATIVE_OPENCONSOLE") != "1" {
		t.Skip("set F4_NATIVE_OPENCONSOLE=1 to run the live pinned OpenConsole gate")
	}
	for _, width := range []int{79, 80, 81, 161} { // N-1, N, N+1, 2N+1 edge
		width := width
		t.Run(fmt.Sprintf("width-%d", width), func(t *testing.T) {
			lines := []string{
				"__F4_NATIVE_WIDTH_BEGIN__",
				strings.Repeat("A", width-1),
				strings.Repeat("B", width),
				strings.Repeat("C", width+1),
				strings.Repeat("D", 2*width+1),
				"repeat: SAME", "repeat: SAME", "repeat: SAME",
				"    ", "",
				"__F4_NATIVE_WIDTH_END__",
			}
			command := "echo " + strings.Join(lines[:1], "") + " & ping -n 2 127.0.0.1 >nul"
			for _, line := range lines[1:] {
				if line == "    " {
					// `echo     ` is the cmd state query, not a spaces-only
					// line. The parenthesized form emits the payload literally.
					command += " & echo(    "
				} else if line == "" {
					command += " & echo."
				} else {
					command += " & echo " + line
				}
			}
			logBytes := runLiveNativeCommand(t, width, 25, command)
			clean := strings.ReplaceAll(logBytes, "\r", "")
			for _, want := range lines {
				if want == "    " || want == "" || want == "repeat: SAME" {
					continue // asserted below without trimming ambiguity
				}
				if len(linesEqual(clean, want)) != 1 {
					t.Fatalf("width %d lost or duplicated exact line %q", width, want)
				}
			}
			if len(linesContaining(clean, "repeat: SAME")) != 3 {
				t.Fatalf("width %d coalesced repeated lines", width)
			}
			long := strings.Repeat("D", 2*width+1)
			if got := linesContaining(clean, long); len(got) != 1 || got[0] != long {
				t.Fatalf("width %d changed 2N+1 line: %q", width, got)
			}
			if !strings.Contains(clean, "    ") {
				t.Fatalf("width %d lost whitespace-only line", width)
			}
			if !strings.Contains(clean, "\n\n") {
				t.Fatalf("width %d lost empty line", width)
			}
		})
	}
}

func TestNativeOpenConsoleF4RealCommands(t *testing.T) {
	if os.Getenv("F4_NATIVE_OPENCONSOLE") != "1" {
		t.Skip("set F4_NATIVE_OPENCONSOLE=1 to run the live pinned OpenConsole gate")
	}
	input := filepath.Join(t.TempDir(), "native-command-input.txt")
	if err := os.WriteFile(input, []byte("needle first\nsecond line\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	quoted := `"` + input + `"`
	cases := []struct {
		name string
		cmd  string
		want string
	}{
		{"dir", `dir /s /b C:\Windows\System32\kernel32.dll`, `C:\Windows\System32\kernel32.dll`},
		{"echo", `echo F4_REAL_ECHO`, `F4_REAL_ECHO`},
		{"type", `type ` + quoted, `needle first`},
		{"findstr", `findstr needle ` + quoted, `needle first`},
		{"powershell", `powershell -NoProfile -NonInteractive -Command "Write-Output F4_REAL_POWERSHELL"`, `F4_REAL_POWERSHELL`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			command := fmt.Sprintf("echo __F4_REAL_BEGIN__ & %s & echo __F4_REAL_END__", tc.cmd)
			logBytes := strings.ReplaceAll(runLiveNativeCommand(t, 80, 25, command), "\r", "")
			if strings.Count(logBytes, "__F4_REAL_BEGIN__") != 1 || strings.Count(logBytes, "__F4_REAL_END__") != 1 || strings.Index(logBytes, "__F4_REAL_BEGIN__") >= strings.Index(logBytes, "__F4_REAL_END__") {
				t.Fatalf("real command markers invalid: %q", logBytes)
			}
			if len(linesContaining(logBytes, tc.want)) != 1 {
				t.Fatalf("real %s output missing/duplicated %q: %q", tc.name, tc.want, logBytes)
			}
		})
	}
}

func runLiveNativeCommand(t *testing.T, width, height int, command string) string {
	t.Helper()
	pty, err := newNativeOpenConsolePTY(width, height)
	if err != nil {
		t.Fatal(err)
	}
	pf := NewPanelsFrame()
	pf.ptyMutex.Lock()
	pf.pty = pty
	pf.termView.pty = pty
	pf.parser.pty = pty
	pf.ptyMutex.Unlock()
	defer pf.Close()
	if err := pty.Run("cmd.exe", "/d", "/c", command); err != nil {
		t.Fatal(err)
	}
	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 113)
		resized := false
		for {
			n, readErr := pty.Read(buf)
			if n > 0 {
				pf.consumeLocalOutput(pty, buf[:n])
				if !resized {
					for _, next := range [][2]int{{1, height}, {width - 1, height}, {width + 1, height}, {width, height}} {
						pf.ResizeConsole(next[0], next[1])
					}
					resized = true
				}
			}
			if readErr != nil {
				readDone <- readErr
				return
			}
		}
	}()
	if err := pty.Wait(); err != nil {
		t.Fatal(err)
	}
	if code, err := pty.ExitCode(); err != nil || code != 0 {
		t.Fatalf("native child exit code=%d err=%v", code, err)
	}
	pty.shutdownHost()
	if err := <-readDone; err != nil && !errors.Is(err, os.ErrClosed) && !errors.Is(err, windows.ERROR_BROKEN_PIPE) && !errors.Is(err, windows.ERROR_HANDLE_EOF) {
		t.Fatal(err)
	}
	if err := pty.Close(); err != nil {
		t.Fatal(err)
	}
	return string(pf.termView.GetAllLogBytes())
}

func assertLiveNativeLog(t *testing.T, logBytes string) {
	t.Helper()
	begin, end := "__F4_NATIVE_LIVE_BEGIN__", "__F4_NATIVE_LIVE_END__"
	if strings.Count(logBytes, begin) != 1 || strings.Count(logBytes, end) != 1 || strings.Index(logBytes, begin) >= strings.Index(logBytes, end) {
		t.Fatalf("live markers missing, duplicated, or reordered: %q", logBytes)
	}
	if got := linesContaining(logBytes, "long: "); len(got) != 1 || got[0] != "long: "+strings.Repeat("C", 257) {
		t.Fatalf("live long line changed: %q", got)
	}
	if len(linesContaining(logBytes, "ascii: LIVE")) != 1 || len(linesContaining(logBytes, "repeat: SAME")) != 3 {
		t.Fatalf("live repeated payload was lost or coalesced: %q", logBytes)
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
