package main

import (
	"strings"
	"testing"
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

func TestLiveLinesTagsEachLineWithTheWidthInForce(t *testing.T) {
	exact := strings.Repeat("=", W)
	stream := []byte(exact + "\r\n" + "short\r\n" + "\x1b[8;100;40t" + strings.Repeat("q", 40) + "\r\n")
	got := liveLines(stream, W)
	if len(got) != 3 {
		t.Fatalf("expected three lines, got %d: %v", len(got), got)
	}
	if got[0].Width != W || got[1].Width != W {
		t.Fatalf("lines before the size report keep the old width: %v", got[:2])
	}
	if got[2].Width != 40 {
		t.Fatalf("a size report must change the width in force, got %d", got[2].Width)
	}
}

func TestLiveLinesRejoinsAScrollSeam(t *testing.T) {
	// A CRLF followed by an absolute cursor position is a scroll seam, not a
	// line ending -- measured behaviour, see the research doc.
	long := strings.Repeat("z", W*2+3)
	stream := []byte(long[:W] + "\r\n\x1b[499;120H" + long[W:] + "\r\n")
	got := liveLines(stream, W)
	if len(got) != 1 || got[0].Text != long {
		t.Fatalf("the split line must be rejoined, got %v", got)
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
