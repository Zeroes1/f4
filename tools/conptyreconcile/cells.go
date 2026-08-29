package main

import "unicode/utf8"

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
var ambiguousWidth = 1

func cellWidth(r rune) int {
	// Microsoft's GraphemeNext, reduced to one codepoint:
	//
	//	auto w = ucdToCharacterWidth(lead);
	//	if (w == 3) { w = _ambiguousWidth; }
	//	if (cp == 0xFE0F) { w = 2; }
	//	...
	//	width = width > 2 ? 2 : width;
	//
	// The value 3 is *ambiguous*, not three columns. Reading it as a column
	// count made a Cyrillic line three times too wide, which moved every
	// merge boundary and broke the whole reconstruction -- exactly the class
	// of error that comes of writing this from memory instead of porting it.
	w := ucdToCharacterWidth(ucdLookup(r))
	if w == 3 {
		w = ambiguousWidth
	}
	// U+FE0F Variation Selector-16 turns an unqualified emoji into a
	// qualified one, which by convention is wide.
	if r == 0xFE0F {
		w = 2
	}
	if w > 2 {
		w = 2
	}
	return w
}

// cellLen is the width of a string in cells.
func cellLen(s string) int {
	n := 0
	for _, r := range s {
		n += cellWidth(r)
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
	head, tail = cutCells(s, width)
	if head == "" {
		_, size := utf8.DecodeRuneInString(s)
		if size < 1 {
			size = 1
		}
		return s[:size], s[size:]
	}
	return head, tail
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
	return cellLen(s)%width == 0
}
