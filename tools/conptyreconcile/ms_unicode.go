// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Source-faithful Go transcription of the pinned Microsoft Terminal source:
// e9b4e2e18fb1b9cee6839969d42cd0f95d228926,
// src/types/CodepointWidthDetector.cpp and its public header.
// The range table below is mechanically extracted from that source.

package main

import (
	"encoding/binary"
	"errors"
	"sort"
	"unicode/utf16"
)

var errEmptyGlyph = errors.New("empty UTF-16 glyph")

type codepointWidth uint8

const (
	widthNarrow codepointWidth = iota
	widthWide
	widthAmbiguous
	widthInvalid
)

type unicodeRange struct {
	lowerBound uint32
	upperBound uint32
	width      codepointWidth
}

var pinnedUnicodeWidthRanges = [...]unicodeRange{
	{0xa1, 0xa1, widthAmbiguous},
	{0xa4, 0xa4, widthAmbiguous},
	{0xa7, 0xa8, widthAmbiguous},
	{0xaa, 0xaa, widthAmbiguous},
	{0xad, 0xae, widthAmbiguous},
	{0xb0, 0xb4, widthAmbiguous},
	{0xb6, 0xba, widthAmbiguous},
	{0xbc, 0xbf, widthAmbiguous},
	{0xc6, 0xc6, widthAmbiguous},
	{0xd0, 0xd0, widthAmbiguous},
	{0xd7, 0xd8, widthAmbiguous},
	{0xde, 0xe1, widthAmbiguous},
	{0xe6, 0xe6, widthAmbiguous},
	{0xe8, 0xea, widthAmbiguous},
	{0xec, 0xed, widthAmbiguous},
	{0xf0, 0xf0, widthAmbiguous},
	{0xf2, 0xf3, widthAmbiguous},
	{0xf7, 0xfa, widthAmbiguous},
	{0xfc, 0xfc, widthAmbiguous},
	{0xfe, 0xfe, widthAmbiguous},
	{0x101, 0x101, widthAmbiguous},
	{0x111, 0x111, widthAmbiguous},
	{0x113, 0x113, widthAmbiguous},
	{0x11b, 0x11b, widthAmbiguous},
	{0x126, 0x127, widthAmbiguous},
	{0x12b, 0x12b, widthAmbiguous},
	{0x131, 0x133, widthAmbiguous},
	{0x138, 0x138, widthAmbiguous},
	{0x13f, 0x142, widthAmbiguous},
	{0x144, 0x144, widthAmbiguous},
	{0x148, 0x14b, widthAmbiguous},
	{0x14d, 0x14d, widthAmbiguous},
	{0x152, 0x153, widthAmbiguous},
	{0x166, 0x167, widthAmbiguous},
	{0x16b, 0x16b, widthAmbiguous},
	{0x1ce, 0x1ce, widthAmbiguous},
	{0x1d0, 0x1d0, widthAmbiguous},
	{0x1d2, 0x1d2, widthAmbiguous},
	{0x1d4, 0x1d4, widthAmbiguous},
	{0x1d6, 0x1d6, widthAmbiguous},
	{0x1d8, 0x1d8, widthAmbiguous},
	{0x1da, 0x1da, widthAmbiguous},
	{0x1dc, 0x1dc, widthAmbiguous},
	{0x251, 0x251, widthAmbiguous},
	{0x261, 0x261, widthAmbiguous},
	{0x2c4, 0x2c4, widthAmbiguous},
	{0x2c7, 0x2c7, widthAmbiguous},
	{0x2c9, 0x2cb, widthAmbiguous},
	{0x2cd, 0x2cd, widthAmbiguous},
	{0x2d0, 0x2d0, widthAmbiguous},
	{0x2d8, 0x2db, widthAmbiguous},
	{0x2dd, 0x2dd, widthAmbiguous},
	{0x2df, 0x2df, widthAmbiguous},
	{0x300, 0x36f, widthAmbiguous},
	{0x391, 0x3a1, widthAmbiguous},
	{0x3a3, 0x3a9, widthAmbiguous},
	{0x3b1, 0x3c1, widthAmbiguous},
	{0x3c3, 0x3c9, widthAmbiguous},
	{0x401, 0x401, widthAmbiguous},
	{0x410, 0x44f, widthAmbiguous},
	{0x451, 0x451, widthAmbiguous},
	{0x1100, 0x115f, widthWide},
	{0x2010, 0x2010, widthAmbiguous},
	{0x2013, 0x2016, widthAmbiguous},
	{0x2018, 0x2019, widthAmbiguous},
	{0x201c, 0x201d, widthAmbiguous},
	{0x2020, 0x2022, widthAmbiguous},
	{0x2024, 0x2027, widthAmbiguous},
	{0x2030, 0x2030, widthAmbiguous},
	{0x2032, 0x2033, widthAmbiguous},
	{0x2035, 0x2035, widthAmbiguous},
	{0x203b, 0x203b, widthAmbiguous},
	{0x203e, 0x203e, widthAmbiguous},
	{0x2074, 0x2074, widthAmbiguous},
	{0x207f, 0x207f, widthAmbiguous},
	{0x2081, 0x2084, widthAmbiguous},
	{0x20ac, 0x20ac, widthAmbiguous},
	{0x2103, 0x2103, widthAmbiguous},
	{0x2105, 0x2105, widthAmbiguous},
	{0x2109, 0x2109, widthAmbiguous},
	{0x2113, 0x2113, widthAmbiguous},
	{0x2116, 0x2116, widthAmbiguous},
	{0x2121, 0x2122, widthAmbiguous},
	{0x2126, 0x2126, widthAmbiguous},
	{0x212b, 0x212b, widthAmbiguous},
	{0x2153, 0x2154, widthAmbiguous},
	{0x215b, 0x215e, widthAmbiguous},
	{0x2160, 0x216b, widthAmbiguous},
	{0x2170, 0x2179, widthAmbiguous},
	{0x2189, 0x2189, widthAmbiguous},
	{0x2190, 0x2199, widthAmbiguous},
	{0x21b8, 0x21b9, widthAmbiguous},
	{0x21d2, 0x21d2, widthAmbiguous},
	{0x21d4, 0x21d4, widthAmbiguous},
	{0x21e7, 0x21e7, widthAmbiguous},
	{0x2200, 0x2200, widthAmbiguous},
	{0x2202, 0x2203, widthAmbiguous},
	{0x2207, 0x2208, widthAmbiguous},
	{0x220b, 0x220b, widthAmbiguous},
	{0x220f, 0x220f, widthAmbiguous},
	{0x2211, 0x2211, widthAmbiguous},
	{0x2215, 0x2215, widthAmbiguous},
	{0x221a, 0x221a, widthAmbiguous},
	{0x221d, 0x2220, widthAmbiguous},
	{0x2223, 0x2223, widthAmbiguous},
	{0x2225, 0x2225, widthAmbiguous},
	{0x2227, 0x222c, widthAmbiguous},
	{0x222e, 0x222e, widthAmbiguous},
	{0x2234, 0x2237, widthAmbiguous},
	{0x223c, 0x223d, widthAmbiguous},
	{0x2248, 0x2248, widthAmbiguous},
	{0x224c, 0x224c, widthAmbiguous},
	{0x2252, 0x2252, widthAmbiguous},
	{0x2260, 0x2261, widthAmbiguous},
	{0x2264, 0x2267, widthAmbiguous},
	{0x226a, 0x226b, widthAmbiguous},
	{0x226e, 0x226f, widthAmbiguous},
	{0x2282, 0x2283, widthAmbiguous},
	{0x2286, 0x2287, widthAmbiguous},
	{0x2295, 0x2295, widthAmbiguous},
	{0x2299, 0x2299, widthAmbiguous},
	{0x22a5, 0x22a5, widthAmbiguous},
	{0x22bf, 0x22bf, widthAmbiguous},
	{0x2312, 0x2312, widthAmbiguous},
	{0x231a, 0x231b, widthWide},
	{0x2329, 0x232a, widthWide},
	{0x23e9, 0x23ec, widthWide},
	{0x23f0, 0x23f0, widthWide},
	{0x23f3, 0x23f3, widthWide},
	{0x2460, 0x24e9, widthAmbiguous},
	{0x24eb, 0x24ff, widthAmbiguous},
	{0x2500, 0x259f, widthNarrow}, // box-drawing and block elements require 1-cell alignment
	{0x25a0, 0x25a1, widthAmbiguous},
	{0x25a3, 0x25a9, widthAmbiguous},
	{0x25b2, 0x25b3, widthAmbiguous},
	{0x25b6, 0x25b7, widthAmbiguous},
	{0x25bc, 0x25bd, widthAmbiguous},
	{0x25c0, 0x25c1, widthAmbiguous},
	{0x25c6, 0x25c8, widthAmbiguous},
	{0x25cb, 0x25cb, widthAmbiguous},
	{0x25ce, 0x25d1, widthAmbiguous},
	{0x25e2, 0x25e5, widthAmbiguous},
	{0x25ef, 0x25ef, widthAmbiguous},
	{0x25fd, 0x25fe, widthWide},
	{0x2605, 0x2606, widthAmbiguous},
	{0x2609, 0x2609, widthAmbiguous},
	{0x260e, 0x260f, widthAmbiguous},
	{0x2614, 0x2615, widthWide},
	{0x261c, 0x261c, widthAmbiguous},
	{0x261e, 0x261e, widthAmbiguous},
	{0x2640, 0x2640, widthAmbiguous},
	{0x2642, 0x2642, widthAmbiguous},
	{0x2648, 0x2653, widthWide},
	{0x2660, 0x2661, widthAmbiguous},
	{0x2663, 0x2665, widthAmbiguous},
	{0x2667, 0x266a, widthAmbiguous},
	{0x266c, 0x266d, widthAmbiguous},
	{0x266f, 0x266f, widthAmbiguous},
	{0x267f, 0x267f, widthWide},
	{0x2693, 0x2693, widthWide},
	{0x269e, 0x269f, widthAmbiguous},
	{0x26a1, 0x26a1, widthWide},
	{0x26aa, 0x26ab, widthWide},
	{0x26bd, 0x26be, widthWide},
	{0x26bf, 0x26bf, widthAmbiguous},
	{0x26c4, 0x26c5, widthWide},
	{0x26c6, 0x26cd, widthAmbiguous},
	{0x26ce, 0x26ce, widthWide},
	{0x26cf, 0x26d3, widthAmbiguous},
	{0x26d4, 0x26d4, widthWide},
	{0x26d5, 0x26e1, widthAmbiguous},
	{0x26e3, 0x26e3, widthAmbiguous},
	{0x26e8, 0x26e9, widthAmbiguous},
	{0x26ea, 0x26ea, widthWide},
	{0x26eb, 0x26f1, widthAmbiguous},
	{0x26f2, 0x26f3, widthWide},
	{0x26f4, 0x26f4, widthAmbiguous},
	{0x26f5, 0x26f5, widthWide},
	{0x26f6, 0x26f9, widthAmbiguous},
	{0x26fa, 0x26fa, widthWide},
	{0x26fb, 0x26fc, widthAmbiguous},
	{0x26fd, 0x26fd, widthWide},
	{0x26fe, 0x26ff, widthAmbiguous},
	{0x2705, 0x2705, widthWide},
	{0x270a, 0x270b, widthWide},
	{0x2728, 0x2728, widthWide},
	{0x273d, 0x273d, widthAmbiguous},
	{0x274c, 0x274c, widthWide},
	{0x274e, 0x274e, widthWide},
	{0x2753, 0x2755, widthWide},
	{0x2757, 0x2757, widthWide},
	{0x2776, 0x277f, widthAmbiguous},
	{0x2795, 0x2797, widthWide},
	{0x27b0, 0x27b0, widthWide},
	{0x27bf, 0x27bf, widthWide},
	{0x2b1b, 0x2b1c, widthWide},
	{0x2b50, 0x2b50, widthWide},
	{0x2b55, 0x2b55, widthWide},
	{0x2b56, 0x2b59, widthAmbiguous},
	{0x2e80, 0x2e99, widthWide},
	{0x2e9b, 0x2ef3, widthWide},
	{0x2f00, 0x2fd5, widthWide},
	{0x2ff0, 0x2ffb, widthWide},
	{0x3000, 0x303e, widthWide},
	{0x3041, 0x3096, widthWide},
	{0x3099, 0x30ff, widthWide},
	{0x3105, 0x312f, widthWide},
	{0x3131, 0x318e, widthWide},
	{0x3190, 0x31e3, widthWide},
	{0x31f0, 0x321e, widthWide},
	{0x3220, 0x3247, widthWide},
	{0x3248, 0x324f, widthAmbiguous},
	{0x3250, 0x4dbf, widthWide},
	{0x4dc0, 0x4dff, widthNarrow}, // hexagrams are historically narrow
	{0x4e00, 0xa48c, widthWide},
	{0xa490, 0xa4c6, widthWide},
	{0xa960, 0xa97c, widthWide},
	{0xac00, 0xd7a3, widthWide},
	{0xe000, 0xf8ff, widthAmbiguous},
	{0xf900, 0xfaff, widthWide},
	{0xfe00, 0xfe0f, widthAmbiguous},
	{0xfe10, 0xfe19, widthWide},
	{0xfe20, 0xfe2f, widthNarrow}, // narrow combining ligatures (split into left/right halves, which take 2 columns together)
	{0xfe30, 0xfe52, widthWide},
	{0xfe54, 0xfe66, widthWide},
	{0xfe68, 0xfe6b, widthWide},
	{0xff01, 0xff60, widthWide},
	{0xffe0, 0xffe6, widthWide},
	{0xfffd, 0xfffd, widthAmbiguous},
	{0x16fe0, 0x16fe4, widthWide},
	{0x16ff0, 0x16ff1, widthWide},
	{0x17000, 0x187f7, widthWide},
	{0x18800, 0x18cd5, widthWide},
	{0x18d00, 0x18d08, widthWide},
	{0x1b000, 0x1b11e, widthWide},
	{0x1b150, 0x1b152, widthWide},
	{0x1b164, 0x1b167, widthWide},
	{0x1b170, 0x1b2fb, widthWide},
	{0x1f004, 0x1f004, widthWide},
	{0x1f0cf, 0x1f0cf, widthWide},
	{0x1f100, 0x1f10a, widthAmbiguous},
	{0x1f110, 0x1f12d, widthAmbiguous},
	{0x1f130, 0x1f169, widthAmbiguous},
	{0x1f170, 0x1f18d, widthAmbiguous},
	{0x1f18e, 0x1f18e, widthWide},
	{0x1f18f, 0x1f190, widthAmbiguous},
	{0x1f191, 0x1f19a, widthWide},
	{0x1f19b, 0x1f1ac, widthAmbiguous},
	{0x1f1e6, 0x1f202, widthWide},
	{0x1f210, 0x1f23b, widthWide},
	{0x1f240, 0x1f248, widthWide},
	{0x1f250, 0x1f251, widthWide},
	{0x1f260, 0x1f265, widthWide},
	{0x1f300, 0x1f320, widthWide},
	{0x1f32d, 0x1f335, widthWide},
	{0x1f337, 0x1f37c, widthWide},
	{0x1f37e, 0x1f393, widthWide},
	{0x1f3a0, 0x1f3ca, widthWide},
	{0x1f3cf, 0x1f3d3, widthWide},
	{0x1f3e0, 0x1f3f0, widthWide},
	{0x1f3f4, 0x1f3f4, widthWide},
	{0x1f3f8, 0x1f43e, widthWide},
	{0x1f440, 0x1f440, widthWide},
	{0x1f442, 0x1f4fc, widthWide},
	{0x1f4ff, 0x1f53d, widthWide},
	{0x1f54b, 0x1f54e, widthWide},
	{0x1f550, 0x1f567, widthWide},
	{0x1f57a, 0x1f57a, widthWide},
	{0x1f595, 0x1f596, widthWide},
	{0x1f5a4, 0x1f5a4, widthWide},
	{0x1f5fb, 0x1f64f, widthWide},
	{0x1f680, 0x1f6c5, widthWide},
	{0x1f6cc, 0x1f6cc, widthWide},
	{0x1f6d0, 0x1f6d2, widthWide},
	{0x1f6d5, 0x1f6d7, widthWide},
	{0x1f6eb, 0x1f6ec, widthWide},
	{0x1f6f4, 0x1f6fc, widthWide},
	{0x1f7e0, 0x1f7eb, widthWide},
	{0x1f90c, 0x1f93a, widthWide},
	{0x1f93c, 0x1f945, widthWide},
	{0x1f947, 0x1f978, widthWide},
	{0x1f97a, 0x1f9cb, widthWide},
	{0x1f9cd, 0x1f9ff, widthWide},
	{0x1fa70, 0x1fa74, widthWide},
	{0x1fa78, 0x1fa7a, widthWide},
	{0x1fa80, 0x1fa86, widthWide},
	{0x1fa90, 0x1faa8, widthWide},
	{0x1fab0, 0x1fab6, widthWide},
	{0x1fac0, 0x1fac2, widthWide},
	{0x1fad0, 0x1fad6, widthWide},
	{0x20000, 0x2fffd, widthWide},
	{0x30000, 0x3fffd, widthWide},
	{0xe0100, 0xe01ef, widthAmbiguous},
	{0xf0000, 0xffffd, widthAmbiguous},
	{0x100000, 0x10fffd, widthAmbiguous},
}

// widthDetector is the Go representation of CodepointWidthDetector.  The
// input type is []uint16 deliberately: the pinned implementation consumes a
// std::wstring_view, so the mock must not round-trip through UTF-8 before
// applying the table or the fallback cache.
type widthDetector struct {
	fallback func([]uint16) bool
	cache    map[string]bool
}

func newWidthDetector() *widthDetector {
	return &widthDetector{cache: make(map[string]bool)}
}

func (d *widthDetector) GetWidth(glyph []uint16) (codepointWidth, error) {
	if len(glyph) == 0 {
		return widthInvalid, errEmptyGlyph
	}
	if len(glyph) == 1 {
		width := getQuickCharWidth(glyph[0])
		if width != widthInvalid {
			if width == widthAmbiguous && d.fallback != nil {
				if d.checkFallbackViaCache(glyph) {
					return widthWide, nil
				}
				return widthAmbiguous, nil
			}
			return width, nil
		}
	}
	return d.lookupGlyphWidthWithCache(glyph), nil
}

func (d *widthDetector) IsWide(glyph []uint16) bool {
	width, err := d.GetWidth(glyph)
	if err != nil {
		return false
	}
	return width == widthWide
}

func (d *widthDetector) SetFallbackMethod(fallback func([]uint16) bool) {
	d.fallback = fallback
}

func (d *widthDetector) NotifyFontChanged() { clear(d.cache) }

func getQuickCharWidth(wch uint16) codepointWidth {
	if 0x20 <= wch && wch <= 0x7e {
		return widthNarrow
	}
	return widthInvalid
}

func (d *widthDetector) lookupGlyphWidthWithCache(glyph []uint16) (width codepointWidth) {
	// The pinned method is noexcept and catches failures from the table/cache
	// path, preferring Wide when it cannot determine a narrower answer.
	defer func() {
		if recover() != nil {
			width = widthWide
		}
	}()

	width = d.lookupGlyphWidth(glyph)
	if width == widthAmbiguous && d.fallback != nil {
		if d.checkFallbackViaCache(glyph) {
			return widthWide
		}
	}
	return width
}

func (d *widthDetector) lookupGlyphWidth(glyph []uint16) codepointWidth {
	if len(glyph) == 0 {
		return widthInvalid
	}
	codepoint, ok := extractCodepoint(glyph)
	if !ok {
		// _lookupGlyphWidthWithCache catches the C++ exception and returns
		// Wide.  Keep that fail-safe at this boundary.
		return widthWide
	}
	index := sort.Search(len(pinnedUnicodeWidthRanges), func(i int) bool {
		return pinnedUnicodeWidthRanges[i].upperBound >= codepoint
	})
	if index < len(pinnedUnicodeWidthRanges) {
		rangeValue := pinnedUnicodeWidthRanges[index]
		if codepoint >= rangeValue.lowerBound && codepoint <= rangeValue.upperBound {
			return rangeValue.width
		}
	}
	return widthNarrow
}

func (d *widthDetector) checkFallbackViaCache(glyph []uint16) bool {
	key := utf16Key(glyph)
	if result, ok := d.cache[key]; ok {
		return result
	}
	result := d.fallback(glyph)
	d.cache[key] = result
	return result
}

func extractCodepoint(glyph []uint16) (uint32, bool) {
	if len(glyph) == 1 {
		return uint32(glyph[0]), true
	}
	if len(glyph) < 2 {
		return 0, false
	}
	codepoint := (uint32(glyph[0]&0x3ff) << 10) | uint32(glyph[1]&0x3ff)
	return codepoint + 0x10000, true
}

func utf16Key(units []uint16) string {
	key := make([]byte, len(units)*2)
	for i, unit := range units {
		binary.LittleEndian.PutUint16(key[i*2:], unit)
	}
	return string(key)
}

func utf16Units(s string) []uint16 {
	decoded := []rune(s)
	result := make([]uint16, 0, len(decoded))
	for _, r := range decoded {
		if r <= 0xffff {
			result = append(result, uint16(r))
			continue
		}
		r -= 0x10000
		result = append(result, uint16(0xd800+(r>>10)), uint16(0xdc00+(r&0x3ff)))
	}
	return result
}

func runesFromUTF16(units []uint16) []rune { return utf16.Decode(units) }

func isLeadingSurrogate(unit uint16) bool { return unit >= 0xd800 && unit <= 0xdbff }
func isTrailingSurrogate(unit uint16) bool {
	return unit >= 0xdc00 && unit <= 0xdfff
}

// utf16ParseNext is Utf16Parser::ParseNext.  In particular, malformed
// leading/trailing units are skipped while searching, and an entirely broken
// input produces the replacement character.
func utf16ParseNext(units []uint16) []uint16 {
	for pos, unit := range units {
		if isLeadingSurrogate(unit) {
			next := pos + 1
			if next < len(units) && isTrailingSurrogate(units[next]) {
				return units[pos : pos+2]
			}
			continue
		}
		if isTrailingSurrogate(unit) {
			continue
		}
		return units[pos : pos+1]
	}
	return []uint16{0xfffd}
}

// utf16Parse is Utf16Parser::Parse.  It drops malformed surrogate fragments
// and groups only directly adjacent lead/trail pairs.
func utf16Parse(units []uint16) [][]uint16 {
	result := make([][]uint16, 0, len(units))
	sequence := make([]uint16, 0, 2)
	for _, unit := range units {
		if isLeadingSurrogate(unit) {
			sequence = sequence[:0]
			sequence = append(sequence, unit)
		} else if isTrailingSurrogate(unit) {
			if len(sequence) != 0 {
				sequence = append(sequence, unit)
				result = append(result, append([]uint16(nil), sequence...))
				sequence = sequence[:0]
			}
		} else {
			result = append(result, []uint16{unit})
		}
	}
	return result
}
