package main

import (
	"strings"
	"testing"
)

// TestV12WrapFlagMatchesTheDumps drives the ported v1.12 write path with the
// shapes the field dumps contain and checks the wrap flag the buffer ends up
// holding. Nothing here compares against a rule this project wrote down: the
// expectations are what the ported code must produce for the merge described
// in P13/§17 to happen at all.
func TestV12WrapFlagMatchesTheDumps(t *testing.T) {
	const width = 120

	cases := []struct {
		name    string
		text    string
		wantRow int
		wrapped bool
	}{
		{"exact width fills the row", strings.Repeat("o", width), 0, true},
		{"short line does not wrap", strings.Repeat("o", width-1), 0, false},
		{"one over wraps the first row", strings.Repeat("o", width+1), 0, true},
		{"wide glyphs to the edge", strings.Repeat("中", width/2), 0, true},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := newV12Screen(width, 200)
			v12WriteCharsLegacy(s, c.text, wcFlags{delayEolWrap: true}, nil)
			if got := s.buffer.GetRowByOffset(c.wantRow).WasWrapForced(); got != c.wrapped {
				t.Fatalf("row %d wrapForced = %v, want %v", c.wantRow, got, c.wrapped)
			}
		})
	}
}

// TestV12NewlineClearsWrap: an explicit newline after a full row is what makes
// conhost's merge visible or not, and it is the single place the flag is
// cleared.
func TestV12NewlineClearsWrap(t *testing.T) {
	const width = 120
	s := newV12Screen(width, 200)
	v12WriteCharsLegacy(s, strings.Repeat("o", width), wcFlags{delayEolWrap: true}, nil)
	if !s.buffer.GetRowByOffset(0).WasWrapForced() {
		t.Fatal("a row filled to the edge must be marked wrapped")
	}
	v12WriteCharsLegacy(s, "\r\n", wcFlags{delayEolWrap: true}, nil)
	// The cursor was delayed on the last column of row 0; the CR/LF moves it
	// to row 1 and clears row 1's flag, NOT row 0's -- which is exactly why
	// the two logical lines stay merged in the buffer.
	if !s.buffer.GetRowByOffset(0).WasWrapForced() {
		t.Fatal("row 0 must stay wrapped across the newline: this is the merge of P13")
	}
}
