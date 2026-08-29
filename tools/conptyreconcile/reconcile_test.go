package main

import (
	"strings"
	"testing"
	"unicode/utf16"
)

const W = 20

func frameOf(lines ...string) []byte {
	var sb strings.Builder
	sb.WriteString("\x1b[?25l\x1b[8;100;20t\x1b[H")
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\x1b[K\r\n")
	}
	sb.WriteString("\x1b[42;1H\x1b[?25h")
	return []byte(sb.String())
}

func TestSplitFrameLinesReadsTerminatorsAsBoundaries(t *testing.T) {
	got := splitFrameLines(frameOf("alpha", "beta", strings.Repeat("x", 55)))
	want := []string{"alpha", "beta", strings.Repeat("x", 55)}
	if len(got) != len(want) {
		t.Fatalf("got %d lines %q", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d: %q want %q", i, got[i], want[i])
		}
	}
}

func TestSplitFrameLinesKeepsALongLineWhole(t *testing.T) {
	long := strings.Repeat("y", W*3+7)
	got := splitFrameLines(frameOf(long))
	if len(got) != 1 || got[0] != long {
		t.Fatalf("a wrapped line must stay one logical line, got %q", got)
	}
}

func TestLiveLinesUsesACursorNotASeamRule(t *testing.T) {
	// The shape a real capture produced: a short line ended not with a
	// newline but with an absolute cursor position to the next row but one,
	// skipping a blank row. Seam rules read this as one continued line; a
	// cursor reads it as two lines and a blank.
	stream := []byte("first" + "\x1b[3;1H" + "third")
	got := liveLines(stream, 20)
	if len(got) != 3 || got[0].Text != "first" || got[1].Text != "" || got[2].Text != "third" {
		t.Fatalf("expected first / blank / third, got %d: %+v", len(got), got)
	}
}

func TestLiveLinesJoinsAWrappedLine(t *testing.T) {
	// Wrapping is decided by this code's own autowrap, exactly as conhost's
	// ROW::SetWrapForced is set when a write runs past the last column.
	long := strings.Repeat("z", 45)
	got := liveLines([]byte(long+"\r\n"), 20)
	if len(got) != 1 || got[0].Text != long {
		t.Fatalf("a wrapped line is one logical line, got %d: %+v", len(got), got)
	}
}

func TestLiveLinesHandlesAWidthChangeMidStream(t *testing.T) {
	before := strings.Repeat("a", 30)
	after := strings.Repeat("b", 30)
	stream := []byte(before + "\r\n" + "\x1b[8;100;40t" + after + "\r\n")
	got := liveLines(stream, 20)
	if len(got) != 2 || got[0].Text != before || got[1].Text != after {
		t.Fatalf("a size report must not corrupt the lines around it: %+v", got)
	}
}

func TestReconcileSplitsWhatConhostMerged(t *testing.T) {
	// The measured failure: a line of exactly W characters arrives in the
	// frame glued to the line after it.
	exact := strings.Repeat("=", W)
	next := "the following line"
	merged := exact + next

	breaks := liveLines([]byte(exact+"\r\n"+next+"\r\n"), W)
	got := reconcileOrdered([]string{merged}, breaks)

	if len(got) != 2 || got[0] != exact || got[1] != next {
		t.Fatalf("expected the merge to be undone, got %q", got)
	}
}

func TestReconcileLeavesAGenuineWrapAlone(t *testing.T) {
	// A wrapped line is longer than W but was never terminated at W, so the
	// live stream recorded nothing and no split may happen.
	long := strings.Repeat("q", W*2+5)
	breaks := liveLines([]byte(long+"\r\n"), W)
	got := reconcileOrdered([]string{long}, breaks)
	if len(got) != 1 || got[0] != long {
		t.Fatalf("a genuine wrap must survive, got %q", got)
	}
}

func TestReconcileHandlesIdenticalLines(t *testing.T) {
	// The objection this design has to answer: ten identical separator rows.
	// The decision is per content, so identical rows get identical treatment
	// and position never enters into it.
	sep := strings.Repeat("-", W)
	var live strings.Builder
	for i := 0; i < 10; i++ {
		live.WriteString(sep + "\r\n")
	}
	live.WriteString("tail\r\n")

	breaks := liveLines([]byte(live.String()), W)
	// The frame merged all ten separators and the tail into one run.
	merged := strings.Repeat(sep, 10) + "tail"
	got := reconcileOrdered([]string{merged}, breaks)

	if len(got) != 11 {
		t.Fatalf("expected 10 separators plus the tail, got %d: %q", len(got), got)
	}
	for i := 0; i < 10; i++ {
		if got[i] != sep {
			t.Fatalf("line %d is %q", i, got[i])
		}
	}
	if got[10] != "tail" {
		t.Fatalf("tail is %q", got[10])
	}
}

func TestReconcileSplitsAMultipleOfTheWidth(t *testing.T) {
	// A line of exactly 2W is two full rows and merges the same way.
	двойная := strings.Repeat("#", W*2)
	next := "after"
	breaks := liveLines([]byte(двойная+"\r\n"+next+"\r\n"), W)
	// Only the *last* row before the break is recorded, which is what the
	// splitter looks at.
	got := reconcileOrdered([]string{двойная + next}, breaks)
	if len(got) != 2 || got[0] != двойная || got[1] != next {
		t.Fatalf("got %q", got)
	}
}

func TestReconcileIsIdempotent(t *testing.T) {
	exact := strings.Repeat("=", W)
	next := "tail"
	breaks := liveLines([]byte(exact+"\r\n"+next+"\r\n"), W)
	once := reconcileOrdered([]string{exact + next}, breaks)
	twice := reconcileOrdered(once, breaks)
	if len(once) != len(twice) {
		t.Fatalf("second pass changed the result: %q then %q", once, twice)
	}
}

// A real capture began with the window title conhost sends,
// ESC ] 0 ; <path> BEL, and an OSC skipper that knew only about BEL-vs-ST
// mattered: the title text leaked into the first logical line and the whole
// alignment failed. Both terminators, and a truncated sequence, are pinned.
func TestLiveLinesSkipsTheWindowTitle(t *testing.T) {
	for _, term := range []string{"\x07", "\x1b\\"} {
		stream := []byte("\x1b]0;D:\\probe.exe" + term + "first line\r\nsecond\r\n")
		got := liveLines(stream, W)
		if len(got) != 2 || got[0].Text != "first line" || got[1].Text != "second" {
			t.Fatalf("terminator %q: got %v", term, got)
		}
	}
	// Unterminated: must not loop or panic, and must not resurrect the title.
	if got := liveLines([]byte("\x1b]0;never ends"), W); len(got) != 0 {
		t.Fatalf("an unterminated OSC should swallow the rest, got %v", got)
	}
}

func TestGridFeedIsIndependentOfReadBoundaries(t *testing.T) {
	row := newMsROW(20)
	col := 0
	for _, r := range utf16.Encode([]rune("prefix" + strings.Repeat("中", 7))) {
		state := msRowWriteState{text: []uint16{r}, columnBegin: col, columnLimit: 20}
		// This intentionally mirrors a series of one-code-unit PrintString calls.
		row.ReplaceText(&state)
		col = state.columnEnd
	}
	stream := []byte("prefix\x1b]0;window title\x1b\\" + strings.Repeat("中", 7) +
		"\x1b[8;30;17t\x1b[12;4Htail\r\n")

	whole := NewGrid(20)
	whole.Feed(stream)

	chunked := NewGrid(20)
	for _, chunk := range newMockConPTY(20, 30, 1).Chunks(stream, 1) {
		chunked.Feed(chunk)
	}

	got, want := chunked.LogicalLinesWithWidth(), whole.LogicalLinesWithWidth()
	if len(got) != len(want) {
		t.Fatalf("chunked feed returned %d lines, whole feed returned %d: got=%+v want=%+v", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d differs after one-byte chunking: got=%+v want=%+v", i, got[i], want[i])
		}
	}
}

func TestReconcileAcceptsMSWideCellFramePadding(t *testing.T) {
	const width = 120
	wide := strings.Repeat("中", width/2)
	next := "line 000004 short"
	live := liveLines([]byte("\x1b]0;probe\x07"+wide+"\r\n"+next+"\r\n"), width)
	frame := frameOf(wide + " " + next)

	got := reconcileOrdered(splitFrameLines(frame), live)
	if len(got) != 2 || got[0] != wide || got[1] != next {
		t.Fatalf("MS frame padding must not become part of either logical line: got %q", got)
	}
}
