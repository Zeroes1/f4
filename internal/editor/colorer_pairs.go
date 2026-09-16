package editor

import (
	"unicode/utf8"

	colorer "github.com/unxed/colorer4go"
)

// colorerPairToken is one token of a pair, on its line.
type colorerPairToken struct {
	line int
	pair colorer.Pair
}

// colorerPairMatch is Colorer's PairMatch: the pair token under the cursor
// and, when found, the one that balances it.
type colorerPairMatch struct {
	start colorerPairToken
	end   colorerPairToken
	found bool
	// top is set when the cursor is on a pair start, so the match lies after
	// it; otherwise it lies before.
	top bool
}

// matchColorerPair is BaseEditor::getPairMatch followed by
// BaseEditor::searchPair, walking the pairs of each line (colorer4go's
// ParseLinePairs) instead of Colorer's line regions; only pair regions move
// the balance, so the result is the same.
//
// pos is a rune offset on line, and a token matches when it lies within
// [Start, End] — End included, as in getPairMatch, so the cursor just after a
// bracket still finds it; the last such token of the line wins. The search
// stays within lines [first, last]. pairsAt reports a line's pairs and false
// for a line that has not been parsed: Colorer's regions are always complete,
// an unparsed line here may hold the match, so the search stops without one.
//
// It reports false when there is no pair token under the cursor.
func matchColorerPair(line, pos, first, last int, pairsAt func(int) ([]colorer.Pair, bool)) (colorerPairMatch, bool) {
	pairs, ok := pairsAt(line)
	if !ok {
		return colorerPairMatch{}, false
	}
	idx := -1
	for i, p := range pairs {
		if pos >= p.Start && pos <= p.End {
			idx = i
		}
	}
	if idx < 0 {
		return colorerPairMatch{}, false
	}

	m := colorerPairMatch{start: colorerPairToken{line, pairs[idx]}, top: pairs[idx].Opens}
	balance := -1
	if m.top {
		balance = 1
	}
	lno, i := line, idx
	for {
		if balance > 0 {
			i++
			for i >= len(pairs) {
				lno++
				if lno > last {
					return m, true
				}
				if pairs, ok = pairsAt(lno); !ok {
					return m, true
				}
				i = 0
			}
		} else {
			i--
			for i < 0 {
				lno--
				if lno < first {
					return m, true
				}
				if pairs, ok = pairsAt(lno); !ok {
					return m, true
				}
				i = len(pairs) - 1
			}
		}
		if pairs[i].Opens {
			balance++
		} else {
			balance--
		}
		if balance == 0 {
			m.end = colorerPairToken{lno, pairs[i]}
			m.found = true
			return m, true
		}
	}
}

// colorerPairOverlay is the pair under the cursor as drawn: the tokens to
// paint, by line.
type colorerPairOverlay map[int][]colorer.Pair

// cachedPairs are the pairs of a line whose colours are cached. A line with
// no cached colours has not been parsed, which is not the same as having no
// pairs.
func (ch *ColorerHighlighter) cachedPairs(idx int) ([]colorer.Pair, bool) {
	if _, ok := ch.attrCache[idx]; !ok {
		return nil, false
	}
	return ch.pairCache[idx], true
}

// pairOverlay finds the pair under the cursor within the visible lines [first,
// last], as FarColorer's searchLocalPair does when it redraws: the token under
// the cursor is painted even when its match is not on screen. cursorByte is a
// byte offset on text, the cursor line.
func (ch *ColorerHighlighter) pairOverlay(cursorLine, cursorByte int, text string, first, last int) colorerPairOverlay {
	if ch == nil || ch.session == nil || ch.closed || ch.disabled {
		return nil
	}
	if cursorByte > len(text) {
		cursorByte = len(text)
	}
	if cursorByte < 0 {
		cursorByte = 0
	}
	pos := utf8.RuneCountInString(text[:cursorByte])
	m, ok := matchColorerPair(cursorLine, pos, first, last, ch.cachedPairs)
	if !ok {
		return nil
	}
	overlay := colorerPairOverlay{m.start.line: {m.start.pair}}
	if m.found {
		overlay[m.end.line] = append(overlay[m.end.line], m.end.pair)
	}
	return overlay
}

// apply paints the overlay's tokens on line idx over its syntax colours, each
// with the pair's own colour style. attrs is the cached slice and is not
// modified; a line without tokens gets attrs back.
func (o colorerPairOverlay) apply(idx int, attrs []uint64) []uint64 {
	pairs := o[idx]
	if len(pairs) == 0 || attrs == nil {
		return attrs
	}
	out := append([]uint64(nil), attrs...)
	for _, p := range pairs {
		start, end, _ := colorerRegionRunes(p.Start, p.End, len(out))
		style := colorer.RegionDefine{Fore: p.Fore, Back: p.Back, Style: p.Style, IsForeSet: p.IsForeSet, IsBackSet: p.IsBackSet}
		for i := start; i < end; i++ {
			out[i] = applyColorerStyle(out[i], &style)
		}
	}
	return out
}
