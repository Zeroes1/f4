package terminal

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
)

// syncEnv is a terminal with a parser and a fake Pty, with the cursor on the
// top row so that the tests can read what was printed off row zero. A fresh
// terminal starts at the bottom (rule 2 in TERMINAL.md).
func syncEnv(t *testing.T) (*TerminalView, *AnsiParser, *mockPty) {
	t.Helper()
	tv := NewTerminalView(80, 24)
	Pty := &mockPty{}
	tv.Pty = Pty
	tv.SetCursor(0, 0)
	return tv, NewAnsiParser(tv, Pty), Pty
}

func syncRow(tv *TerminalView, row int) string {
	var sb strings.Builder
	for _, c := range tv.Lines[row] {
		sb.WriteRune(testutil.Rune(c.Char))
	}
	return strings.TrimRight(sb.String(), " ")
}

// A chunk that ends on the final byte of a query has to be answered there and
// then. The child sent CSI c and is now waiting: nothing else is coming that
// could release a byte held back for later.
func TestAnsiQueryAtChunkEndIsAnswered(t *testing.T) {
	for _, query := range []string{"\x1b[c", "\x1b[0c", "\x1b[?1;1S", "\x1b[16t"} {
		_, p, Pty := syncEnv(t)
		p.Process([]byte(query))
		if Pty.String() == "" {
			t.Errorf("%q went unanswered", query)
		}
	}
}

// The same, one byte at a time, which is how ConPTY fragments things.
func TestAnsiQuerySplitByteByByteIsAnswered(t *testing.T) {
	_, p, Pty := syncEnv(t)
	for _, b := range []byte("\x1b[c") {
		p.Process([]byte{b})
	}
	if got := Pty.String(); got != "\x1b[?62;4c" {
		t.Errorf("device attributes: got %q", got)
	}
}

// No byte of ordinary output may be held back either: a prompt that ends on a
// c would lose it until the next thing the child printed.
func TestAnsiTrailingTextIsNotWithheld(t *testing.T) {
	for _, text := range []string{"abc", "cd", "cd /d", "c"} {
		tv, p, _ := syncEnv(t)
		p.Process([]byte(text))
		if got := syncRow(tv, 0); got != text {
			t.Errorf("printed %q, screen shows %q", text, got)
		}
	}
}

// The command that keeps the panel and the shell in step must still be hidden,
// however the PTY chops it up.
func TestWindowsSyncExcisedAtEveryChunkBoundary(t *testing.T) {
	const stream = "ok\r\ncd /d \"C:\\\\tmp\" & rem f4_sync\r\ndone"
	for split := 0; split <= len(stream); split++ {
		tv, p, _ := syncEnv(t)
		p.Process([]byte(stream[:split]))
		p.Process([]byte(stream[split:]))

		var screen strings.Builder
		for row := 0; row < 4; row++ {
			screen.WriteString(syncRow(tv, row))
			screen.WriteByte('\n')
		}
		got := screen.String()
		if strings.Contains(got, "f4_sync") || strings.Contains(got, "cd /d") {
			t.Fatalf("split at %d leaked the technical command:\n%s", split, got)
		}
		if !strings.Contains(got, "ok") || !strings.Contains(got, "done") {
			t.Fatalf("split at %d lost real output:\n%s", split, got)
		}
	}
}

// A cd the user typed is not the technical one and stays on the screen.
func TestWindowsSyncLeavesRealCommandsAlone(t *testing.T) {
	tv, p, _ := syncEnv(t)
	p.Process([]byte("cd /d \"C:\\\\tmp\" & dir\r\n"))
	if got := syncRow(tv, 0); !strings.Contains(got, "dir") {
		t.Errorf("the user's command must survive: %q", got)
	}
}

// A parser that tracks the lines f4 types cuts the prefix from their echo
// only. Far Manager, run from f4, repaints the console's own copy of those
// lines when it hides a panel; the same text there is output, and cutting it
// out of the middle of the row moved the panel drawn after it forty columns
// to the left (#1376).
func TestWindowsSyncTrackedCutsOnlyTheAnnouncedEcho(t *testing.T) {
	tv, p, _ := syncEnv(t)
	p.TrackWindowsSyncEcho()

	// The echo of a line f4 typed loses its technical prefix.
	p.ExpectWindowsSyncEcho()
	p.Process([]byte("C:\\FAR>cd /d \"C:\\FAR\" & Far.exe\r\n"))
	if got := syncRow(tv, 0); got != "C:\\FAR>Far.exe" {
		t.Fatalf("typed line echo = %q, want the prefix cut", got)
	}

	// A repaint of that row by the program is drawn as it is, and what
	// follows it on the row stays in its column.
	row := "C:\\FAR>cd /d \"C:\\FAR\" & Far.exe"
	p.Process([]byte("\x1b[2;1H" + row + "\x1b[2;41HPANEL"))
	if got := syncRow(tv, 1); got != row+strings.Repeat(" ", 40-len(row))+"PANEL" {
		t.Errorf("repainted row = %q, want it unchanged with PANEL at column 41", got)
	}
	p.Process([]byte("\x1b[3;1HC:\\F4>cd /d \"C:\\FAR\" & rem f4_sync\r\n"))
	if got := syncRow(tv, 2); !strings.Contains(got, "rem f4_sync") {
		t.Errorf("repainted sync line = %q, want it left on the screen", got)
	}
}

// Each announced echo is cut once, so a second copy of the same line in one
// chunk -- the program's repaint right behind the echo -- is kept.
func TestWindowsSyncTrackedEchoIsCutOnce(t *testing.T) {
	tv, p, _ := syncEnv(t)
	p.TrackWindowsSyncEcho()
	p.ExpectWindowsSyncEcho()
	p.Process([]byte("cd /d \"C:\\tmp\" & rem f4_sync\r\nok\r\ncd /d \"C:\\tmp\" & rem f4_sync\r\n"))
	if got := syncRow(tv, 0); got != "ok" {
		t.Errorf("row 0 = %q, want the echo cut and ok on its row", got)
	}
	if got := syncRow(tv, 1); !strings.Contains(got, "rem f4_sync") {
		t.Errorf("row 1 = %q, want the second copy kept", got)
	}
}
