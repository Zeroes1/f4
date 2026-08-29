package main

import (
	"strings"
	"testing"
)

// Width in cells, not bytes. Every one of these fails on the byte-based
// version this file replaced.

func TestCellLenCountsColumnsNotBytes(t *testing.T) {
	cases := []struct {
		s     string
		cells int
	}{
		{"abc", 3},
		{"тест", 4}, // Cyrillic: 8 bytes, 4 cells
		{"中文", 4},   // CJK: 6 bytes, 4 cells
		{"a中b", 4},
		{"", 0},
	}
	for _, c := range cases {
		if got := cellLen(c.s); got != c.cells {
			t.Errorf("cellLen(%q) = %d, want %d (bytes: %d)", c.s, got, c.cells, len(c.s))
		}
	}
}

func TestFillsRowsExactlyUsesCells(t *testing.T) {
	// Ten Cyrillic letters are 10 cells and 20 bytes. At width 10 the line
	// fills its row exactly and conhost merges it; a byte-based test would
	// see 20 and merge at width 20 instead -- wrong in both directions.
	line := strings.Repeat("т", 10)
	if !fillsRowsExactly(line, 10) {
		t.Fatal("10 Cyrillic cells fill a 10-column row")
	}
	if fillsRowsExactly(line, 20) {
		t.Fatal("a byte-based reading would merge here, and must not")
	}
}

func TestCutCellsNeverSplitsACharacter(t *testing.T) {
	for _, s := range []string{"тест строка", "中文中文", "aбв中g"} {
		for w := 1; w <= cellLen(s)+2; w++ {
			head, tail := cutCells(s, w)
			if head+tail != s {
				t.Fatalf("cutCells(%q, %d) lost text", s, w)
			}
			if !isValidUTF8(head) || !isValidUTF8(tail) {
				t.Fatalf("cutCells(%q, %d) split a character", s, w)
			}
			if cellLen(head) > w {
				t.Fatalf("cutCells(%q, %d) head is %d cells", s, w, cellLen(head))
			}
		}
	}
}

func TestWideGlyphMovesWholeToTheNextRow(t *testing.T) {
	// A double-width glyph that will not fit in the last column moves down
	// whole; conhost marks the cell it leaves as padding. The padding is not
	// text, so reassembling the rows must not invent a space.
	s := "中中中" // three glyphs, six cells
	head, tail := cutCells(s, 5)
	if cellLen(head) != 4 || head+tail != s {
		t.Fatalf("expected two glyphs in the head, got %q + %q", head, tail)
	}
}

func TestWrapIsCellCorrectForNonASCII(t *testing.T) {
	lines := []string{strings.Repeat("т", 25), strings.Repeat("中", 10), "plain"}
	const w = 10
	rows := Wrap(lines, w)
	if got := len(rows); got != RowsFor(lines, w) {
		t.Fatalf("Wrap made %d rows, RowsFor said %d", got, RowsFor(lines, w))
	}
	for _, r := range rows {
		if cellLen(r.Text) > w {
			t.Fatalf("row %q is %d cells at width %d", r.Text, cellLen(r.Text), w)
		}
	}
	// 25 Cyrillic cells at width 10 is three rows; 10 CJK glyphs are 20 cells,
	// so two rows; plus one for "plain".
	if len(rows) != 3+2+1 {
		t.Fatalf("expected 6 rows, got %d", len(rows))
	}
}

func TestCoordinatesAreCellsOnTheScreenAndBytesInTheMirror(t *testing.T) {
	m := NewMirror()
	m.Append("абвгд")
	v := m.Slice(80, 5)

	// Screen column 3 is the fourth letter, byte offset 6.
	p, ok := v.ScreenToMirror(3, 0)
	if !ok || p.Offset != 6 {
		t.Fatalf("column 3 of Cyrillic should be byte offset 6, got %+v", p)
	}
	col, row, ok := v.MirrorToScreen(p)
	if !ok || col != 3 || row != 0 {
		t.Fatalf("mapping back gave (%d,%d) ok=%v", col, row, ok)
	}
}

func isValidUTF8(s string) bool {
	for _, r := range s {
		if r == '\uFFFD' && !strings.Contains(s, "\uFFFD") {
			return false
		}
	}
	return true
}

func TestPipelineWithNonASCIIContent(t *testing.T) {
	// The end-to-end property, on content that is not ASCII at all.
	const writeWidth, frameWidth = 60, 59
	printed := []string{
		strings.Repeat("т", writeWidth), // exactly one row in cells: merges
		"после слияния",
		strings.Repeat("中", writeWidth/2), // wide glyphs, exactly one row
		"хвост",
		markerDone,
	}
	writer := newMockConPTY(writeWidth, 500, 1)
	viewer := newMockConPTY(frameWidth, 500, 1)

	got := trimTrailingBlanks(reconcileOrdered(
		trimTrailingBlanks(splitFrameLines(viewer.FrameAtWidth(printed, writeWidth))),
		liveLines(writer.LiveStream(printed), writeWidth)))

	if len(got) != len(printed) {
		t.Fatalf("recovered %d lines, expected %d: %q", len(got), len(printed), got)
	}
	for i := range printed {
		if got[i] != printed[i] {
			t.Fatalf("line %d:\n got  %q\n want %q", i, got[i], printed[i])
		}
	}
}

// The port is only worth anything if it agrees with Microsoft's table on the
// cases that decide where conhost wraps. These are checked against the values
// the trie actually returns, not against my expectations of Unicode.
func TestWidthsComeFromMicrosoftsTable(t *testing.T) {
	cases := []struct {
		r     rune
		cells int
		why   string
	}{
		{'a', 1, "ASCII"},
		{'т', 1, "Cyrillic is East Asian Ambiguous; conhost's default is 1"},
		{'中', 2, "CJK unified is wide"},
		{'あ', 2, "Hiragana is wide"},
		{'\u0301', 0, "a combining acute adds no column"},
		{'\uFF21', 2, "fullwidth Latin A"},
		{'\u00A0', 1, "no-break space"},
	}
	for _, c := range cases {
		if got := cellWidth(c.r); got != c.cells {
			t.Errorf("cellWidth(%q) = %d, want %d (%s)", c.r, got, c.cells, c.why)
		}
	}
}

func TestAmbiguousWidthIsASettingAsItIsInConhost(t *testing.T) {
	// CodepointWidthDetector has SetAmbiguousWidth; a user with a CJK font
	// wants Cyrillic counted as two, and conhost lets them say so.
	old := ambiguousWidth
	defer func() { ambiguousWidth = old }()

	ambiguousWidth = 2
	if got := cellWidth('т'); got != 2 {
		t.Fatalf("with ambiguous width 2, Cyrillic should be 2 cells, got %d", got)
	}
}

func TestVariationSelectorMakesAnEmojiWide(t *testing.T) {
	// Microsoft's rule, ported verbatim: U+FE0F is given width 2.
	if got := cellWidth(0xFE0F); got != 2 {
		t.Fatalf("U+FE0F should be wide, got %d", got)
	}
}

func TestWidthIsClampedToTwo(t *testing.T) {
	for r := rune(0); r < 0x11000; r += 7 {
		if w := cellWidth(r); w < 0 || w > 2 {
			t.Fatalf("cellWidth(U+%04X) = %d, outside 0..2", r, w)
		}
	}
}

func TestLookupCoversTheWholeCodepointRange(t *testing.T) {
	// The trie is indexed arithmetically; a codepoint outside the range must
	// not index out of bounds.
	for _, r := range []rune{-1, 0, 0x10FFFF, 0x110000, 0x7FFFFFFF} {
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("U+%X panicked: %v", r, rec)
				}
			}()
			cellWidth(r)
		}()
	}
}
