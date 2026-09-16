package editor

import (
	"testing"

	colorer "github.com/unxed/colorer4go"
)

// pairLines builds a pairsAt over lines written as text: '(' is a pair start
// and ')' a pair end, at their rune offsets. A line that is nil is unparsed.
func pairLines(lines ...*string) func(int) ([]colorer.Pair, bool) {
	return func(idx int) ([]colorer.Pair, bool) {
		if idx < 0 || idx >= len(lines) || lines[idx] == nil {
			return nil, false
		}
		var pairs []colorer.Pair
		for i, r := range []rune(*lines[idx]) {
			if r == '(' || r == ')' {
				pairs = append(pairs, colorer.Pair{Start: i, End: i + 1, Opens: r == '('})
			}
		}
		return pairs, true
	}
}

func str(s string) *string { return &s }

func TestMatchColorerPair_ForwardAcrossLines(t *testing.T) {
	at := pairLines(str("f(a, (b)"), str("  c"), str(")")) // the first '(' closes on line 2
	m, ok := matchColorerPair(0, 1, 0, 2, at)
	if !ok || !m.top || !m.found {
		t.Fatalf("got %+v, %v; want a found match below", m, ok)
	}
	if m.end.line != 2 || m.end.pair.Start != 0 {
		t.Errorf("end = line %d col %d, want line 2 col 0", m.end.line, m.end.pair.Start)
	}
}

func TestMatchColorerPair_BackwardFromAnEnd(t *testing.T) {
	at := pairLines(str("x (y"), str("(z) )"))
	m, ok := matchColorerPair(1, 4, 0, 1, at)
	if !ok || m.top || !m.found || m.end.line != 0 || m.end.pair.Start != 2 {
		t.Fatalf("got %+v, %v; want the '(' on line 0 col 2", m, ok)
	}
}

// getPairMatch includes the end offset, so the cursor right after a bracket
// finds it; with two adjacent tokens the later one wins.
func TestMatchColorerPair_CursorAfterTheToken(t *testing.T) {
	at := pairLines(str("()"))
	m, ok := matchColorerPair(0, 1, 0, 0, at)
	if !ok || m.top || m.start.pair.Start != 1 {
		t.Fatalf("got %+v, %v; want the ')' at col 1, the last token covering col 1", m, ok)
	}
	if !m.found || m.end.pair.Start != 0 {
		t.Errorf("end = %+v, want the '(' at col 0", m.end)
	}
}

func TestMatchColorerPair_StopsAtTheWindowAndUnparsedLines(t *testing.T) {
	at := pairLines(str("("), str(""), str(")"))
	if m, ok := matchColorerPair(0, 0, 0, 1, at); !ok || m.found {
		t.Errorf("window ending on line 1: got %+v, %v; want the start without a match", m, ok)
	}
	unparsed := pairLines(str("("), nil, str(")"))
	if m, ok := matchColorerPair(0, 0, 0, 2, unparsed); !ok || m.found {
		t.Errorf("unparsed line 1: got %+v, %v; want the start without a match", m, ok)
	}
	if _, ok := matchColorerPair(0, 3, 0, 2, at); ok {
		t.Error("a cursor on no pair token found one")
	}
}

func TestColorerPairOverlay_PaintsCopies(t *testing.T) {
	attrs := []uint64{1, 1, 1}
	overlay := colorerPairOverlay{0: {{Start: 1, End: 2, Opens: true, Back: 0x123456, IsBackSet: true}}}
	out := overlay.apply(0, attrs)
	if attrs[1] != 1 {
		t.Fatal("the cached attributes were modified")
	}
	if out[1] == attrs[1] || out[0] != attrs[0] || out[2] != attrs[2] {
		t.Errorf("out = %v; want only rune 1 repainted", out)
	}
	if got := overlay.apply(1, attrs); &got[0] != &attrs[0] {
		t.Error("a line without tokens was copied")
	}
}
