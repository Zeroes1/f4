package main

import (
	"unicode/utf16"
	"unicode/utf8"
)

// Widths are measured in cells, not bytes.
//
// Everything in this tool originally counted `len(s)`, which is right only for
// ASCII. It is wrong in two ways that matter here. A UTF-8 line of Cyrillic is
// twice as many bytes as cells, so "the length is a multiple of the width" --
// the rule that decides whether conhost merged a line -- fires at the wrong
// places; and wrapping by bytes cuts a character in half. conhost itself
// counts cells and gives a wide glyph two of them, with a separate flag for
// the padding cell it leaves when one will not fit at the end of a row
// (ROW::WasDoubleBytePadded).
//
// The mock printed CJK from the start; the model did not. This file is the
// model catching up.

// cellWidth is Microsoft's width, looked up in Microsoft's table. The
// hand-written range list that used to live here is gone: it was my
// approximation of a generated Unicode database, and conhost does not consult
// my approximation when it decides where a row ends.
// ambiguousWidth mirrors CodepointWidthDetector::_ambiguousWidth, whose
// default in CodepointWidthDetector.hpp is 1. It is a setting there and a
// setting here (§16): Cyrillic, Greek and the box-drawing characters are
// "ambiguous" in Unicode's East Asian Width, and a user with a CJK font may
// legitimately want them counted as two.
// The single copy of this setting lives in the port (mscwd.go); this alias
// keeps the old name for callers and tests.
func setAmbiguousWidth(w int) { msAmbiguousWidth = w }

// cellWidth is the width of one codepoint, obtained by running the ported
// CodepointWidthDetector::GraphemeNext (mscwd.go) over just that codepoint.
// The hand-reduced copy of that function that used to live here -- the
// per-codepoint "w = ucdToCharacterWidth(lead); if w == 3 ..." block -- was a
// reimplementation, not a port, and THE RULE forbids it.
func cellWidth(r rune) int {
	return cellLen(string(r))
}

// cellLen is the width of a string in cells.
func cellLen(s string) int {
	// Microsoft measures a string by walking grapheme clusters and summing
	// the width GraphemeNext reports for each, which is what msAmbiguousWidth,
	// U+FE0F and the clamp to 2 are already handled by. Summing per codepoint
	// would disagree with conhost on any cluster of more than one codepoint.
	str := utf16.Encode([]rune(s))
	n := 0
	var st msGraphemeState
	for {
		more := msGraphemeNext(&st, str)
		n += st.width
		if !more {
			break
		}
	}
	return n
}

// cutCells splits s after `cells` columns and returns both halves. A wide
// glyph that would straddle the boundary moves to the next row whole, exactly
// as conhost does -- the cell it vacates is padding, and reassembling the
// pieces must not invent a space there, which is why the padding is not
// represented in the text at all.
func cutCells(s string, cells int) (head, tail string) {
	if cells <= 0 {
		return "", s
	}
	used := 0
	for i, r := range s {
		w := cellWidth(r)
		if used+w > cells {
			return s[:i], s[i:]
		}
		used += w
		if used == cells {
			_, size := utf8.DecodeRuneInString(s[i:])
			return s[:i+size], s[i+size:]
		}
	}
	return s, ""
}

// takeRow splits off exactly one row's worth of text. It is the only place
// that has to know what to do when a single glyph is wider than the entire
// window: it cannot be split, so it takes a row and overhangs. Every caller
// goes through here, because the version where one caller had the guard and
// another did not was an infinite loop -- found by the fuzzer on a
// one-column window containing a wide character.
func takeRow(s string, width int) (head, tail string) {
	if s == "" {
		return "", ""
	}
	// The conhost port consumes UTF-16, so malformed UTF-8 cannot be
	// represented by its row. The public Wrap helper nevertheless promises
	// not to lose bytes from an arbitrary Go string; keep malformed input
	// opaque and use the same cell boundary for its valid portions.
	if !utf8.ValidString(s) {
		return takeRawRow(s, width)
	}
	// How much of a logical line fits in a row of `width` columns is not a
	// question to answer with a hand-written cell count: it is exactly what
	// ROW::ReplaceText decides, including the wide glyph that does not fit at
	// the end of a row and moves down whole (WasDoubleBytePadded). So this
	// writes the text into a ported ROW of that width and reads back how much
	// it consumed. The measurement is conhost's, not ours.
	str := utf16.Encode([]rune(s))
	row := newMsROW(width)
	state := msRowWriteState{text: str, columnLimit: width}
	row.ReplaceText(&state)
	consumed := len(str) - len(state.text)
	if consumed < 1 {
		// The glyph is wider than the row and can never be inserted; the
		// original throws it away (AdaptDispatch::_WriteToBuffer, the
		// textPositionBefore == textPositionAfter branch).
		consumed = msGraphemeLen(str)
	}
	head = string(utf16.Decode(str[:consumed]))
	tail = string(utf16.Decode(str[consumed:]))
	return head, tail
}

func takeRawRow(s string, width int) (head, tail string) {
	head, tail = cutCells(s, width)
	if head != "" {
		return head, tail
	}
	_, size := utf8.DecodeRuneInString(s)
	if size < 1 {
		size = 1
	}
	return s[:size], s[size:]
}

// cellRows is how many rows a string occupies at a given width.
func cellRows(s string, width int) int {
	if width < 1 {
		width = 1
	}
	if s == "" {
		return 1
	}
	rows := 0
	rest := s
	for rest != "" {
		_, rest = takeRow(rest, width)
		rows++
	}
	return rows
}

// fillsRowsExactly reports the condition under which conhost drops the
// terminator and merges a line into its successor: the line ends exactly on a
// row boundary. In cells, not bytes.
func fillsRowsExactly(s string, width int) bool {
	if width < 1 || s == "" {
		return false
	}
	// Whether conhost merges this line into the next one is not a property of
	// its length: it is ROW::WasWrapForced on the line's last row. conhost
	// sets that flag in exactly one place -- AdaptDispatch::_DoLineFeed with
	// wrapForced=true, which _WriteToBuffer calls when a write ran past the
	// last column (delayed EOL wrap). "length is a multiple of the width" was
	// this project's inference from that behaviour, and inferences are what
	// THE RULE forbids: it is wrong for any line whose last row ends in a
	// wide glyph that did not fit (WasDoubleBytePadded), among others.
	//
	// So: write the line into a ported buffer of that width, exactly as a
	// child process would, and ask the last row it occupies.
	t := newMsTerminal(width, msRowsForSafely(s, width))
	t.Feed([]byte(s))
	t.flushPendingText()
	b := t.disp.page.buffer
	y := t.disp.page.cursor.GetPosition().y
	if y < 0 || y >= b.Height() {
		return false
	}
	return b.GetRowByOffset(y).WasWrapForced() ||
		// The delayed EOL wrap has not been acted on yet when the write ends
		// exactly at the edge: the cursor sits pending at the last column and
		// the flag is set by the *next* glyph (or by the line feed that a
		// terminator would bring). Both mean the same thing for the merge.
		t.disp.page.cursor.GetDelayEOLWrap() != nil
}

// msRowsForSafely sizes the scratch buffer so nothing this line writes can
// scroll off it. A tool detail, not a Microsoft value.
func msRowsForSafely(s string, width int) int {
	n := cellLen(s)/width + 2
	if n < 2 {
		n = 2
	}
	return n
}
