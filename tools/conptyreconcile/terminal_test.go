package main

import (
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// reflow
// ---------------------------------------------------------------------------

func TestWrapRoundTripsAtEveryWidth(t *testing.T) {
	lines := []string{"short", strings.Repeat("x", 250), "", "exactly ten", strings.Repeat("y", 40)}
	for _, w := range []int{1, 3, 10, 39, 40, 41, 250, 1000} {
		rows := Wrap(lines, w)
		if got := len(rows); got != RowsFor(lines, w) {
			t.Fatalf("width %d: Wrap made %d rows, RowsFor said %d", w, got, RowsFor(lines, w))
		}
		// Reassembling the rows of each line must give the line back.
		var rebuilt []string
		cur, curLine := "", -1
		for _, r := range rows {
			if r.Line != curLine {
				if curLine >= 0 {
					rebuilt = append(rebuilt, cur)
				}
				cur, curLine = "", r.Line
			}
			cur += r.Text
		}
		rebuilt = append(rebuilt, cur)
		if len(rebuilt) != len(lines) {
			t.Fatalf("width %d: rebuilt %d lines from %d", w, len(rebuilt), len(lines))
		}
		for i := range lines {
			if rebuilt[i] != lines[i] {
				t.Fatalf("width %d line %d: %q != %q", w, i, rebuilt[i], lines[i])
			}
		}
	}
}

func TestEmptyLineStillOccupiesARow(t *testing.T) {
	if n := RowsFor([]string{"", "", ""}, 80); n != 3 {
		t.Fatalf("three blank lines should be three rows, got %d", n)
	}
}

// ---------------------------------------------------------------------------
// the slice
// ---------------------------------------------------------------------------

func TestSliceShowsTheBottomOfTheHistory(t *testing.T) {
	m := NewMirror()
	for i := 1; i <= 100; i++ {
		m.Append(fmt.Sprintf("line %d", i))
	}
	v := m.Slice(80, 10)
	if len(v.Rows) != 10 {
		t.Fatalf("expected 10 rows, got %d", len(v.Rows))
	}
	if v.Rows[9].Text != "line 100" {
		t.Fatalf("the last visible row should be the newest line, got %q", v.Rows[9].Text)
	}
	if v.Rows[0].Text != "line 91" {
		t.Fatalf("the first visible row should be line 91, got %q", v.Rows[0].Text)
	}
}

func TestSliceWithLessHistoryThanTheWindow(t *testing.T) {
	m := NewMirror()
	m.Append("a", "b")
	v := m.Slice(80, 25)
	if len(v.Rows) != 2 || v.Top != 0 {
		t.Fatalf("a short history must not be padded or shifted: %+v", v)
	}
}

func TestSliceCountsWrappedRowsNotLines(t *testing.T) {
	m := NewMirror()
	m.Append(strings.Repeat("x", 200)) // three rows at width 80
	m.Append("tail")
	v := m.Slice(80, 2)
	if len(v.Rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(v.Rows))
	}
	if v.Rows[1].Text != "tail" || v.Rows[0].Line != 0 {
		t.Fatalf("the window should end on the tail, preceded by the last row of the wrapped line: %+v", v.Rows)
	}
}

// ---------------------------------------------------------------------------
// scrolling
// ---------------------------------------------------------------------------

func TestScrollIsClampedAtBothEnds(t *testing.T) {
	m := NewMirror()
	for i := 0; i < 50; i++ {
		m.Append(fmt.Sprintf("l%02d", i))
	}
	if got := m.ScrollBy(1000, 80, 10); got != 40 {
		t.Fatalf("scrolling past the top should stop at 40, got %d", got)
	}
	if got := m.ScrollBy(-1000, 80, 10); got != 0 {
		t.Fatalf("scrolling past the bottom should stop at 0, got %d", got)
	}
}

func TestNarrowingKeepsTheScrollWithinRange(t *testing.T) {
	// Narrowing makes the same text taller, so a scroll position valid at one
	// width can be out of range at another. It must be clamped, not wrapped.
	m := NewMirror()
	for i := 0; i < 20; i++ {
		m.Append(strings.Repeat("z", 100))
	}
	m.ScrollBy(1000, 200, 5) // few rows at a wide width
	wide := m.ScrollPosition()
	m.Slice(20, 5) // many rows at a narrow width
	if m.ScrollPosition() > RowsFor(m.Lines(), 20)-5 {
		t.Fatalf("scroll %d is out of range after narrowing", m.ScrollPosition())
	}
	_ = wide
}

func TestReplaceReturnsToTheBottom(t *testing.T) {
	m := NewMirror()
	for i := 0; i < 50; i++ {
		m.Append("x")
	}
	m.ScrollBy(20, 80, 10)
	m.Replace([]string{"a", "b"})
	if m.ScrollPosition() != 0 {
		t.Fatalf("a new frame should leave the viewport at the bottom, got %d", m.ScrollPosition())
	}
}

// ---------------------------------------------------------------------------
// coordinates
// ---------------------------------------------------------------------------

func TestScreenAndMirrorCoordinatesRoundTrip(t *testing.T) {
	m := NewMirror()
	m.Append("short", strings.Repeat("q", 175), "", "tail")
	for _, w := range []int{10, 40, 80} {
		v := m.Slice(w, 30)
		for row := range v.Rows {
			for col := 0; col <= len(v.Rows[row].Text); col++ {
				p, ok := v.ScreenToMirror(col, row)
				if !ok {
					t.Fatalf("width %d: (%d,%d) did not map", w, col, row)
				}
				gotCol, gotRow, ok := v.MirrorToScreen(p)
				if !ok {
					t.Fatalf("width %d: %v did not map back", w, p)
				}
				// A position at the end of one row is also the start of the
				// next; either answer is correct as long as it maps to the
				// same place in the mirror.
				back, _ := v.ScreenToMirror(gotCol, gotRow)
				if back != p {
					t.Fatalf("width %d: %v -> (%d,%d) -> %v", w, p, gotCol, gotRow, back)
				}
			}
		}
	}
}

func TestClickPastTheEndOfALineClampsToIt(t *testing.T) {
	m := NewMirror()
	m.Append("abc")
	v := m.Slice(80, 5)
	p, ok := v.ScreenToMirror(70, 0)
	if !ok || p.Offset != 3 {
		t.Fatalf("a click in the padding should land at the line end, got %+v ok=%v", p, ok)
	}
}

func TestCoordinatesRejectPointsOutsideTheWindow(t *testing.T) {
	m := NewMirror()
	m.Append("a")
	v := m.Slice(80, 5)
	if _, ok := v.ScreenToMirror(0, 4); ok {
		t.Fatal("a row past the content must not map")
	}
	if _, ok := v.ScreenToMirror(-1, 0); ok {
		t.Fatal("a negative column must not map")
	}
	if _, _, ok := v.MirrorToScreen(Position{Line: 99}); ok {
		t.Fatal("a line the viewport does not show must not map")
	}
}

// ---------------------------------------------------------------------------
// geometry and the detector
// ---------------------------------------------------------------------------

func TestGeometryTellsTheTruthForFullScreenPrograms(t *testing.T) {
	if g := GeometryFor(120, 30, 32000, false); g.Height != 32000 {
		t.Fatalf("ordinary output should get the tall console, got %+v", g)
	}
	if g := GeometryFor(120, 30, 32000, true); g.Height != 30 {
		t.Fatalf("a full-screen program must be told the real size, got %+v", g)
	}
	if g := GeometryFor(120, 30, 10, false); g.Height != 30 {
		t.Fatalf("a history shallower than the window is pointless, got %+v", g)
	}
}

func TestSafeHistoryRowsRespectsTheMeasuredCeiling(t *testing.T) {
	// §17: beyond roughly 32 million cells the host stops responding. The cap
	// is a setting; this only enforces it.
	if got := SafeHistoryRows(4000, 32000, 16_000_000); got != 4000 {
		t.Fatalf("4000 columns against a 16M cap allows 4000 rows, got %d", got)
	}
	if got := SafeHistoryRows(120, 32000, 16_000_000); got != 32000 {
		t.Fatalf("120 columns is nowhere near the cap, got %d", got)
	}
}

func TestFullScreenDetectorLayers(t *testing.T) {
	var f FullScreenState
	if on, _ := f.Active(); on {
		t.Fatal("nothing should be active initially")
	}

	f.Feed([]byte("output \x1b[?1049h more"))
	if on, why := f.Active(); !on || why != "alternate screen" {
		t.Fatalf("the alt-screen enter should fire: %v %q", on, why)
	}
	f.Feed([]byte("\x1b[?1049l back"))
	if on, _ := f.Active(); on {
		t.Fatal("the alt-screen leave should clear it")
	}

	// The deterministic layer wins and is independent of the stream, which is
	// what makes it usable on 10.0.22000, where 1049 never appears.
	f.SetProcess(true)
	if on, why := f.Active(); !on || why != "process" {
		t.Fatalf("the process layer should fire: %v %q", on, why)
	}
}

func TestDetectorIsNotFooledByASplitSequence(t *testing.T) {
	var f FullScreenState
	f.Feed([]byte("\x1b[?10"))
	f.Feed([]byte("49h"))
	if on, _ := f.Active(); on {
		t.Fatal("a sequence split across reads must not fire; missing it is safe, guessing is not")
	}
}

// ---------------------------------------------------------------------------
// the whole pipeline, on the mock
// ---------------------------------------------------------------------------

func TestPipelineFromFrameToVisibleSlice(t *testing.T) {
	const writeWidth, frameWidth, height = 120, 119, 4000
	for seed := int64(1); seed <= 25; seed++ {
		printed := append(randomGroundTruth(seed, writeWidth, 150), markerDone)

		writer := newMockConPTY(writeWidth, height, seed)
		viewer := newMockConPTY(frameWidth, height, seed)

		recovered := trimTrailingBlanks(reconcileOrdered(
			trimTrailingBlanks(splitFrameLines(viewer.FrameAtWidth(printed, writeWidth))),
			liveLines(writer.LiveStream(printed), writeWidth)))

		m := NewMirror()
		m.Replace(recovered)

		want := trimTrailingBlanks(printed)
		if len(m.Lines()) != len(want) {
			t.Fatalf("seed %d: mirror holds %d lines, expected %d", seed, len(m.Lines()), len(want))
		}

		// The visible slice at the real window size must be the tail of the
		// mirror wrapped to the window width -- the property f4 draws from.
		const winW, winH = 100, 25
		v := m.Slice(winW, winH)
		all := Wrap(want, winW)
		if len(v.Rows) != winH {
			t.Fatalf("seed %d: %d visible rows, expected %d", seed, len(v.Rows), winH)
		}
		for i := range v.Rows {
			if v.Rows[i].Text != all[len(all)-winH+i].Text {
				t.Fatalf("seed %d row %d: %q != %q", seed, i,
					trunc(v.Rows[i].Text), trunc(all[len(all)-winH+i].Text))
			}
		}
	}
}

// ---------------------------------------------------------------------------
// fuzzing
// ---------------------------------------------------------------------------

// FuzzWrapPreservesText: whatever the lines and the width, wrapping must lose
// nothing. A reflow that drops a character is worse than one that wraps badly.
func FuzzWrapPreservesText(f *testing.F) {
	f.Add("hello\nworld", 5)
	f.Add("", 1)
	f.Add(strings.Repeat("x", 300), 7)

	f.Fuzz(func(t *testing.T, blob string, width int) {
		if width < 1 || width > 4096 {
			width = 13
		}
		lines := strings.Split(blob, "\n")
		rows := Wrap(lines, width)

		if len(rows) != RowsFor(lines, width) {
			t.Fatalf("row count disagrees with RowsFor")
		}
		var sb strings.Builder
		for _, r := range rows {
			// Cells, not bytes -- and one exception, stated rather than
			// silently allowed: a single glyph wider than the window occupies
			// its own row and overflows it, because it cannot be split.
			if c := cellLen(r.Text); c > width && utf8.RuneCountInString(r.Text) > 1 {
				t.Fatalf("a row of %d cells exceeds the width %d", c, width)
			}
			sb.WriteString(r.Text)
		}
		if sb.String() != strings.Join(lines, "") {
			t.Fatalf("wrapping changed the text")
		}
	})
}

// FuzzSliceAndCoordinatesStayInRange: the viewport and the mapping must never
// panic and never point outside the mirror, whatever the geometry.
func FuzzSliceAndCoordinatesStayInRange(f *testing.F) {
	f.Add("a\nb\nc", 80, 25, 0)
	f.Add(strings.Repeat("q", 500), 3, 1, 7)
	f.Add("", 1, 1, -5)

	f.Fuzz(func(t *testing.T, blob string, width, height, scroll int) {
		if width < 1 || width > 4096 {
			width = 40
		}
		if height < 1 || height > 4096 {
			height = 10
		}
		lines := strings.Split(blob, "\n")

		m := NewMirror()
		m.Replace(lines)
		m.ScrollBy(scroll, width, height)
		v := m.Slice(width, height)

		if len(v.Rows) > height {
			t.Fatalf("slice returned %d rows for a window of %d", len(v.Rows), height)
		}
		if v.Top < 0 || v.Top > v.Total {
			t.Fatalf("top %d out of range for %d rows", v.Top, v.Total)
		}
		for row := range v.Rows {
			for _, col := range []int{0, len(v.Rows[row].Text), width * 2} {
				p, ok := v.ScreenToMirror(col, row)
				if !ok {
					continue
				}
				if p.Line < 0 || p.Line >= len(lines) {
					t.Fatalf("mapped to line %d of %d", p.Line, len(lines))
				}
				if p.Offset < 0 || p.Offset > len(lines[p.Line]) {
					t.Fatalf("mapped to offset %d in a line of %d", p.Offset, len(lines[p.Line]))
				}
			}
		}
	})
}

// FuzzDetectorNeverPanics: the stream is arbitrary bytes and the detector runs
// on every chunk, so it is on the hottest path in the terminal.
func FuzzDetectorNeverPanics(f *testing.F) {
	f.Add([]byte("\x1b[?1049h"))
	f.Add([]byte("\x1b[?1049"))
	f.Add([]byte{0x1b})

	f.Fuzz(func(t *testing.T, chunk []byte) {
		var s FullScreenState
		s.Feed(chunk)
		s.Active()
	})
}
