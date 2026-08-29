// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of microsoft/terminal src/types/CodepointWidthDetector.cpp
// (commit 079d1cc423336c89c1e220701c94b320cecb603a), Graphemes mode.
// The trie tables and ucdLookup/ucdGraphemeJoins/ucdGraphemeDone/
// ucdToCharacterWidth live in ucd.go, ported from the same file.
//
// Mechanical transformations required by Go, and nothing else:
//   - wchar_t*            -> index into a []uint16 (utf16 code units)
//   - `goto fetchNext`    -> the skipToFetch flag below; Go forbids a goto
//     that jumps into a block. The flag reproduces the exact same control
//     flow: on a stored state, skip the initial decode and the first width
//     accumulation, entering the loop at the fetch of the trailing unit.
//   - `state = ~state`    -> state = ^state (identical semantics on int)

package main

// GraphemeState is struct GraphemeState from CodepointWidthDetector.hpp.
// beg/len are indices into the string passed to msGraphemeNext, in utf16
// code units, mirroring the pointer+length pair of the original.
type msGraphemeState struct {
	beg   int
	len   int
	width int

	_state int
	_last  int
}

// _ambiguousWidth mirrors CodepointWidthDetector::_ambiguousWidth, default 1
// (CodepointWidthDetector.hpp). It is a setting there and a setting here.
var msAmbiguousWidth = 1

// utf16NextOrFFFD, ported. `it` and `end` are indices into str; the decoded
// codepoint goes to *out and the index past the codepoint is returned.
func utf16NextOrFFFD(str []uint16, it, end int, out *rune) int {
	c := rune(str[it])
	it++

	// Is any surrogate?
	if (c & 0xF800) == 0xD800 {
		c1 := c
		c = 0xfffd

		// Is leading surrogate and not at end?
		if (c1&0x400) == 0 && it != end {
			c2 := rune(str[it])
			// Is also trailing surrogate!
			if (c2 & 0xFC00) == 0xDC00 {
				c = (c1 << 10) - 0x35FDC00 + c2
				it++
			}
		}
	}

	*out = c
	return it
}

// resetIfOutOfRange, ported: returns `reset` if `ptr` is outside [beg, end).
func resetIfOutOfRange(beg, end, reset, ptr int) int {
	ret := ptr
	if ptr < beg {
		ret = reset
	}
	if ptr > end {
		ret = reset
	}
	return ret
}

// msGraphemeNext is CodepointWidthDetector::_graphemeNext (Graphemes mode).
// Returns false when the end of the string has been reached.
func msGraphemeNext(s *msGraphemeState, str []uint16) bool {
	beg := 0
	end := len(str)
	clusterBeg := s.beg + s.len
	width := s.width
	state := s._state
	lead := s._last

	// If it's a new string argument, we'll restart at the new string's beginning.
	clusterBeg = resetIfOutOfRange(beg, end, beg, clusterBeg)

	clusterEnd := clusterBeg

	// Skip if we're already at the end.
	if clusterEnd < end {
		var cp rune

		// If a previous parsing of a grapheme cluster got interrupted because we reached the end of the string,
		// we'll have stored the parser state in `s._state` so that we can continue parsing within the new string.
		// The problem is that a `state` of zero is also a valid state parameter for `ucdGraphemeJoins`.
		// Thus, we're storing `s._state` bit-flipped so that we can differentiate between it being unset (0) and
		// storing a previous state of 0 (0xffff...).
		gotState := state != 0
		state = ^state
		skipToFetch := gotState

		if !gotState {
			clusterEnd = utf16NextOrFFFD(str, clusterEnd, end, &cp)
			lead = ucdLookup(cp)
			width = 0
			state = 0
		}

		for {
			if !skipToFetch {
				{
					w := ucdToCharacterWidth(lead)
					if w == 3 {
						w = msAmbiguousWidth
					}

					// U+FE0F Variation Selector-16 is used to turn unqualified Emojis into qualified ones.
					// By convention, this turns them from being ambiguous width (= narrow) into wide ones.
					// We achieve this here by explicitly giving this codepoint a wide width.
					// Later down below we'll clamp width back to <= 2.
					if cp == 0xFE0F {
						w = 2
					}

					width += w
				}

				// If we're at the end of the string, we'll break out of the loop, but leave
				// `state` and `lead` as-is, so that we can continue parsing in the next string.
				if clusterEnd >= end {
					break
				}
			}
			skipToFetch = false

			// fetchNext:
			clusterEndNext := utf16NextOrFFFD(str, clusterEnd, end, &cp)
			trail := ucdLookup(cp)

			state = ucdGraphemeJoins(state, lead, trail)
			if ucdGraphemeDone(state) {
				// We'll later do `state = ~state` which will result in `state == 0`.
				state = ^0
				lead = 0
				break
			}

			clusterEnd = clusterEndNext
			lead = trail
		}

		state = ^state
		if width > 2 {
			width = 2
		}

		s.beg = clusterBeg
		s.len = clusterEnd - clusterBeg
		s.width = width
		s._state = state
		s._last = lead
	}

	return clusterEnd < end
}
