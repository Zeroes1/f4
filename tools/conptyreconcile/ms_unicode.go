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
	unicodeRange{0xa1, 0xa1, widthAmbiguous},
	unicodeRange{0xa4, 0xa4, widthAmbiguous},
	unicodeRange{0xa7, 0xa8, widthAmbiguous},
	unicodeRange{0xaa, 0xaa, widthAmbiguous},
	unicodeRange{0xad, 0xae, widthAmbiguous},
	unicodeRange{0xb0, 0xb4, widthAmbiguous},
	unicodeRange{0xb6, 0xba, widthAmbiguous},
	unicodeRange{0xbc, 0xbf, widthAmbiguous},
	unicodeRange{0xc6, 0xc6, widthAmbiguous},
	unicodeRange{0xd0, 0xd0, widthAmbiguous},
	unicodeRange{0xd7, 0xd8, widthAmbiguous},
	unicodeRange{0xde, 0xe1, widthAmbiguous},
	unicodeRange{0xe6, 0xe6, widthAmbiguous},
	unicodeRange{0xe8, 0xea, widthAmbiguous},
	unicodeRange{0xec, 0xed, widthAmbiguous},
	unicodeRange{0xf0, 0xf0, widthAmbiguous},
	unicodeRange{0xf2, 0xf3, widthAmbiguous},
	unicodeRange{0xf7, 0xfa, widthAmbiguous},
	unicodeRange{0xfc, 0xfc, widthAmbiguous},
	unicodeRange{0xfe, 0xfe, widthAmbiguous},
	unicodeRange{0x101, 0x101, widthAmbiguous},
	unicodeRange{0x111, 0x111, widthAmbiguous},
	unicodeRange{0x113, 0x113, widthAmbiguous},
	unicodeRange{0x11b, 0x11b, widthAmbiguous},
	unicodeRange{0x126, 0x127, widthAmbiguous},
	unicodeRange{0x12b, 0x12b, widthAmbiguous},
	unicodeRange{0x131, 0x133, widthAmbiguous},
	unicodeRange{0x138, 0x138, widthAmbiguous},
	unicodeRange{0x13f, 0x142, widthAmbiguous},
	unicodeRange{0x144, 0x144, widthAmbiguous},
	unicodeRange{0x148, 0x14b, widthAmbiguous},
	unicodeRange{0x14d, 0x14d, widthAmbiguous},
	unicodeRange{0x152, 0x153, widthAmbiguous},
	unicodeRange{0x166, 0x167, widthAmbiguous},
	unicodeRange{0x16b, 0x16b, widthAmbiguous},
	unicodeRange{0x1ce, 0x1ce, widthAmbiguous},
	unicodeRange{0x1d0, 0x1d0, widthAmbiguous},
	unicodeRange{0x1d2, 0x1d2, widthAmbiguous},
	unicodeRange{0x1d4, 0x1d4, widthAmbiguous},
	unicodeRange{0x1d6, 0x1d6, widthAmbiguous},
	unicodeRange{0x1d8, 0x1d8, widthAmbiguous},
	unicodeRange{0x1da, 0x1da, widthAmbiguous},
	unicodeRange{0x1dc, 0x1dc, widthAmbiguous},
	unicodeRange{0x251, 0x251, widthAmbiguous},
	unicodeRange{0x261, 0x261, widthAmbiguous},
	unicodeRange{0x2c4, 0x2c4, widthAmbiguous},
	unicodeRange{0x2c7, 0x2c7, widthAmbiguous},
	unicodeRange{0x2c9, 0x2cb, widthAmbiguous},
	unicodeRange{0x2cd, 0x2cd, widthAmbiguous},
	unicodeRange{0x2d0, 0x2d0, widthAmbiguous},
	unicodeRange{0x2d8, 0x2db, widthAmbiguous},
	unicodeRange{0x2dd, 0x2dd, widthAmbiguous},
	unicodeRange{0x2df, 0x2df, widthAmbiguous},
	unicodeRange{0x300, 0x36f, widthAmbiguous},
	unicodeRange{0x391, 0x3a1, widthAmbiguous},
	unicodeRange{0x3a3, 0x3a9, widthAmbiguous},
	unicodeRange{0x3b1, 0x3c1, widthAmbiguous},
	unicodeRange{0x3c3, 0x3c9, widthAmbiguous},
	unicodeRange{0x401, 0x401, widthAmbiguous},
	unicodeRange{0x410, 0x44f, widthAmbiguous},
	unicodeRange{0x451, 0x451, widthAmbiguous},
	unicodeRange{0x1100, 0x115f, widthWide},
	unicodeRange{0x2010, 0x2010, widthAmbiguous},
	unicodeRange{0x2013, 0x2016, widthAmbiguous},
	unicodeRange{0x2018, 0x2019, widthAmbiguous},
	unicodeRange{0x201c, 0x201d, widthAmbiguous},
	unicodeRange{0x2020, 0x2022, widthAmbiguous},
	unicodeRange{0x2024, 0x2027, widthAmbiguous},
	unicodeRange{0x2030, 0x2030, widthAmbiguous},
	unicodeRange{0x2032, 0x2033, widthAmbiguous},
	unicodeRange{0x2035, 0x2035, widthAmbiguous},
	unicodeRange{0x203b, 0x203b, widthAmbiguous},
	unicodeRange{0x203e, 0x203e, widthAmbiguous},
	unicodeRange{0x2074, 0x2074, widthAmbiguous},
	unicodeRange{0x207f, 0x207f, widthAmbiguous},
	unicodeRange{0x2081, 0x2084, widthAmbiguous},
	unicodeRange{0x20ac, 0x20ac, widthAmbiguous},
	unicodeRange{0x2103, 0x2103, widthAmbiguous},
	unicodeRange{0x2105, 0x2105, widthAmbiguous},
	unicodeRange{0x2109, 0x2109, widthAmbiguous},
	unicodeRange{0x2113, 0x2113, widthAmbiguous},
	unicodeRange{0x2116, 0x2116, widthAmbiguous},
	unicodeRange{0x2121, 0x2122, widthAmbiguous},
	unicodeRange{0x2126, 0x2126, widthAmbiguous},
	unicodeRange{0x212b, 0x212b, widthAmbiguous},
	unicodeRange{0x2153, 0x2154, widthAmbiguous},
	unicodeRange{0x215b, 0x215e, widthAmbiguous},
	unicodeRange{0x2160, 0x216b, widthAmbiguous},
	unicodeRange{0x2170, 0x2179, widthAmbiguous},
	unicodeRange{0x2189, 0x2189, widthAmbiguous},
	unicodeRange{0x2190, 0x2199, widthAmbiguous},
	unicodeRange{0x21b8, 0x21b9, widthAmbiguous},
	unicodeRange{0x21d2, 0x21d2, widthAmbiguous},
	unicodeRange{0x21d4, 0x21d4, widthAmbiguous},
	unicodeRange{0x21e7, 0x21e7, widthAmbiguous},
	unicodeRange{0x2200, 0x2200, widthAmbiguous},
	unicodeRange{0x2202, 0x2203, widthAmbiguous},
	unicodeRange{0x2207, 0x2208, widthAmbiguous},
	unicodeRange{0x220b, 0x220b, widthAmbiguous},
	unicodeRange{0x220f, 0x220f, widthAmbiguous},
	unicodeRange{0x2211, 0x2211, widthAmbiguous},
	unicodeRange{0x2215, 0x2215, widthAmbiguous},
	unicodeRange{0x221a, 0x221a, widthAmbiguous},
	unicodeRange{0x221d, 0x2220, widthAmbiguous},
	unicodeRange{0x2223, 0x2223, widthAmbiguous},
	unicodeRange{0x2225, 0x2225, widthAmbiguous},
	unicodeRange{0x2227, 0x222c, widthAmbiguous},
	unicodeRange{0x222e, 0x222e, widthAmbiguous},
	unicodeRange{0x2234, 0x2237, widthAmbiguous},
	unicodeRange{0x223c, 0x223d, widthAmbiguous},
	unicodeRange{0x2248, 0x2248, widthAmbiguous},
	unicodeRange{0x224c, 0x224c, widthAmbiguous},
	unicodeRange{0x2252, 0x2252, widthAmbiguous},
	unicodeRange{0x2260, 0x2261, widthAmbiguous},
	unicodeRange{0x2264, 0x2267, widthAmbiguous},
	unicodeRange{0x226a, 0x226b, widthAmbiguous},
	unicodeRange{0x226e, 0x226f, widthAmbiguous},
	unicodeRange{0x2282, 0x2283, widthAmbiguous},
	unicodeRange{0x2286, 0x2287, widthAmbiguous},
	unicodeRange{0x2295, 0x2295, widthAmbiguous},
	unicodeRange{0x2299, 0x2299, widthAmbiguous},
	unicodeRange{0x22a5, 0x22a5, widthAmbiguous},
	unicodeRange{0x22bf, 0x22bf, widthAmbiguous},
	unicodeRange{0x2312, 0x2312, widthAmbiguous},
	unicodeRange{0x231a, 0x231b, widthWide},
	unicodeRange{0x2329, 0x232a, widthWide},
	unicodeRange{0x23e9, 0x23ec, widthWide},
	unicodeRange{0x23f0, 0x23f0, widthWide},
	unicodeRange{0x23f3, 0x23f3, widthWide},
	unicodeRange{0x2460, 0x24e9, widthAmbiguous},
	unicodeRange{0x24eb, 0x24ff, widthAmbiguous},
	unicodeRange{0x2500, 0x259f, widthNarrow}, // box-drawing and block elements require 1-cell alignment
	unicodeRange{0x25a0, 0x25a1, widthAmbiguous},
	unicodeRange{0x25a3, 0x25a9, widthAmbiguous},
	unicodeRange{0x25b2, 0x25b3, widthAmbiguous},
	unicodeRange{0x25b6, 0x25b7, widthAmbiguous},
	unicodeRange{0x25bc, 0x25bd, widthAmbiguous},
	unicodeRange{0x25c0, 0x25c1, widthAmbiguous},
	unicodeRange{0x25c6, 0x25c8, widthAmbiguous},
	unicodeRange{0x25cb, 0x25cb, widthAmbiguous},
	unicodeRange{0x25ce, 0x25d1, widthAmbiguous},
	unicodeRange{0x25e2, 0x25e5, widthAmbiguous},
	unicodeRange{0x25ef, 0x25ef, widthAmbiguous},
	unicodeRange{0x25fd, 0x25fe, widthWide},
	unicodeRange{0x2605, 0x2606, widthAmbiguous},
	unicodeRange{0x2609, 0x2609, widthAmbiguous},
	unicodeRange{0x260e, 0x260f, widthAmbiguous},
	unicodeRange{0x2614, 0x2615, widthWide},
	unicodeRange{0x261c, 0x261c, widthAmbiguous},
	unicodeRange{0x261e, 0x261e, widthAmbiguous},
	unicodeRange{0x2640, 0x2640, widthAmbiguous},
	unicodeRange{0x2642, 0x2642, widthAmbiguous},
	unicodeRange{0x2648, 0x2653, widthWide},
	unicodeRange{0x2660, 0x2661, widthAmbiguous},
	unicodeRange{0x2663, 0x2665, widthAmbiguous},
	unicodeRange{0x2667, 0x266a, widthAmbiguous},
	unicodeRange{0x266c, 0x266d, widthAmbiguous},
	unicodeRange{0x266f, 0x266f, widthAmbiguous},
	unicodeRange{0x267f, 0x267f, widthWide},
	unicodeRange{0x2693, 0x2693, widthWide},
	unicodeRange{0x269e, 0x269f, widthAmbiguous},
	unicodeRange{0x26a1, 0x26a1, widthWide},
	unicodeRange{0x26aa, 0x26ab, widthWide},
	unicodeRange{0x26bd, 0x26be, widthWide},
	unicodeRange{0x26bf, 0x26bf, widthAmbiguous},
	unicodeRange{0x26c4, 0x26c5, widthWide},
	unicodeRange{0x26c6, 0x26cd, widthAmbiguous},
	unicodeRange{0x26ce, 0x26ce, widthWide},
	unicodeRange{0x26cf, 0x26d3, widthAmbiguous},
	unicodeRange{0x26d4, 0x26d4, widthWide},
	unicodeRange{0x26d5, 0x26e1, widthAmbiguous},
	unicodeRange{0x26e3, 0x26e3, widthAmbiguous},
	unicodeRange{0x26e8, 0x26e9, widthAmbiguous},
	unicodeRange{0x26ea, 0x26ea, widthWide},
	unicodeRange{0x26eb, 0x26f1, widthAmbiguous},
	unicodeRange{0x26f2, 0x26f3, widthWide},
	unicodeRange{0x26f4, 0x26f4, widthAmbiguous},
	unicodeRange{0x26f5, 0x26f5, widthWide},
	unicodeRange{0x26f6, 0x26f9, widthAmbiguous},
	unicodeRange{0x26fa, 0x26fa, widthWide},
	unicodeRange{0x26fb, 0x26fc, widthAmbiguous},
	unicodeRange{0x26fd, 0x26fd, widthWide},
	unicodeRange{0x26fe, 0x26ff, widthAmbiguous},
	unicodeRange{0x2705, 0x2705, widthWide},
	unicodeRange{0x270a, 0x270b, widthWide},
	unicodeRange{0x2728, 0x2728, widthWide},
	unicodeRange{0x273d, 0x273d, widthAmbiguous},
	unicodeRange{0x274c, 0x274c, widthWide},
	unicodeRange{0x274e, 0x274e, widthWide},
	unicodeRange{0x2753, 0x2755, widthWide},
	unicodeRange{0x2757, 0x2757, widthWide},
	unicodeRange{0x2776, 0x277f, widthAmbiguous},
	unicodeRange{0x2795, 0x2797, widthWide},
	unicodeRange{0x27b0, 0x27b0, widthWide},
	unicodeRange{0x27bf, 0x27bf, widthWide},
	unicodeRange{0x2b1b, 0x2b1c, widthWide},
	unicodeRange{0x2b50, 0x2b50, widthWide},
	unicodeRange{0x2b55, 0x2b55, widthWide},
	unicodeRange{0x2b56, 0x2b59, widthAmbiguous},
	unicodeRange{0x2e80, 0x2e99, widthWide},
	unicodeRange{0x2e9b, 0x2ef3, widthWide},
	unicodeRange{0x2f00, 0x2fd5, widthWide},
	unicodeRange{0x2ff0, 0x2ffb, widthWide},
	unicodeRange{0x3000, 0x303e, widthWide},
	unicodeRange{0x3041, 0x3096, widthWide},
	unicodeRange{0x3099, 0x30ff, widthWide},
	unicodeRange{0x3105, 0x312f, widthWide},
	unicodeRange{0x3131, 0x318e, widthWide},
	unicodeRange{0x3190, 0x31e3, widthWide},
	unicodeRange{0x31f0, 0x321e, widthWide},
	unicodeRange{0x3220, 0x3247, widthWide},
	unicodeRange{0x3248, 0x324f, widthAmbiguous},
	unicodeRange{0x3250, 0x4dbf, widthWide},
	unicodeRange{0x4dc0, 0x4dff, widthNarrow}, // hexagrams are historically narrow
	unicodeRange{0x4e00, 0xa48c, widthWide},
	unicodeRange{0xa490, 0xa4c6, widthWide},
	unicodeRange{0xa960, 0xa97c, widthWide},
	unicodeRange{0xac00, 0xd7a3, widthWide},
	unicodeRange{0xe000, 0xf8ff, widthAmbiguous},
	unicodeRange{0xf900, 0xfaff, widthWide},
	unicodeRange{0xfe00, 0xfe0f, widthAmbiguous},
	unicodeRange{0xfe10, 0xfe19, widthWide},
	unicodeRange{0xfe20, 0xfe2f, widthNarrow}, // narrow combining ligatures (split into left/right halves, which take 2 columns together)
	unicodeRange{0xfe30, 0xfe52, widthWide},
	unicodeRange{0xfe54, 0xfe66, widthWide},
	unicodeRange{0xfe68, 0xfe6b, widthWide},
	unicodeRange{0xff01, 0xff60, widthWide},
	unicodeRange{0xffe0, 0xffe6, widthWide},
	unicodeRange{0xfffd, 0xfffd, widthAmbiguous},
	unicodeRange{0x16fe0, 0x16fe4, widthWide},
	unicodeRange{0x16ff0, 0x16ff1, widthWide},
	unicodeRange{0x17000, 0x187f7, widthWide},
	unicodeRange{0x18800, 0x18cd5, widthWide},
	unicodeRange{0x18d00, 0x18d08, widthWide},
	unicodeRange{0x1b000, 0x1b11e, widthWide},
	unicodeRange{0x1b150, 0x1b152, widthWide},
	unicodeRange{0x1b164, 0x1b167, widthWide},
	unicodeRange{0x1b170, 0x1b2fb, widthWide},
	unicodeRange{0x1f004, 0x1f004, widthWide},
	unicodeRange{0x1f0cf, 0x1f0cf, widthWide},
	unicodeRange{0x1f100, 0x1f10a, widthAmbiguous},
	unicodeRange{0x1f110, 0x1f12d, widthAmbiguous},
	unicodeRange{0x1f130, 0x1f169, widthAmbiguous},
	unicodeRange{0x1f170, 0x1f18d, widthAmbiguous},
	unicodeRange{0x1f18e, 0x1f18e, widthWide},
	unicodeRange{0x1f18f, 0x1f190, widthAmbiguous},
	unicodeRange{0x1f191, 0x1f19a, widthWide},
	unicodeRange{0x1f19b, 0x1f1ac, widthAmbiguous},
	unicodeRange{0x1f1e6, 0x1f202, widthWide},
	unicodeRange{0x1f210, 0x1f23b, widthWide},
	unicodeRange{0x1f240, 0x1f248, widthWide},
	unicodeRange{0x1f250, 0x1f251, widthWide},
	unicodeRange{0x1f260, 0x1f265, widthWide},
	unicodeRange{0x1f300, 0x1f320, widthWide},
	unicodeRange{0x1f32d, 0x1f335, widthWide},
	unicodeRange{0x1f337, 0x1f37c, widthWide},
	unicodeRange{0x1f37e, 0x1f393, widthWide},
	unicodeRange{0x1f3a0, 0x1f3ca, widthWide},
	unicodeRange{0x1f3cf, 0x1f3d3, widthWide},
	unicodeRange{0x1f3e0, 0x1f3f0, widthWide},
	unicodeRange{0x1f3f4, 0x1f3f4, widthWide},
	unicodeRange{0x1f3f8, 0x1f43e, widthWide},
	unicodeRange{0x1f440, 0x1f440, widthWide},
	unicodeRange{0x1f442, 0x1f4fc, widthWide},
	unicodeRange{0x1f4ff, 0x1f53d, widthWide},
	unicodeRange{0x1f54b, 0x1f54e, widthWide},
	unicodeRange{0x1f550, 0x1f567, widthWide},
	unicodeRange{0x1f57a, 0x1f57a, widthWide},
	unicodeRange{0x1f595, 0x1f596, widthWide},
	unicodeRange{0x1f5a4, 0x1f5a4, widthWide},
	unicodeRange{0x1f5fb, 0x1f64f, widthWide},
	unicodeRange{0x1f680, 0x1f6c5, widthWide},
	unicodeRange{0x1f6cc, 0x1f6cc, widthWide},
	unicodeRange{0x1f6d0, 0x1f6d2, widthWide},
	unicodeRange{0x1f6d5, 0x1f6d7, widthWide},
	unicodeRange{0x1f6eb, 0x1f6ec, widthWide},
	unicodeRange{0x1f6f4, 0x1f6fc, widthWide},
	unicodeRange{0x1f7e0, 0x1f7eb, widthWide},
	unicodeRange{0x1f90c, 0x1f93a, widthWide},
	unicodeRange{0x1f93c, 0x1f945, widthWide},
	unicodeRange{0x1f947, 0x1f978, widthWide},
	unicodeRange{0x1f97a, 0x1f9cb, widthWide},
	unicodeRange{0x1f9cd, 0x1f9ff, widthWide},
	unicodeRange{0x1fa70, 0x1fa74, widthWide},
	unicodeRange{0x1fa78, 0x1fa7a, widthWide},
	unicodeRange{0x1fa80, 0x1fa86, widthWide},
	unicodeRange{0x1fa90, 0x1faa8, widthWide},
	unicodeRange{0x1fab0, 0x1fab6, widthWide},
	unicodeRange{0x1fac0, 0x1fac2, widthWide},
	unicodeRange{0x1fad0, 0x1fad6, widthWide},
	unicodeRange{0x20000, 0x2fffd, widthWide},
	unicodeRange{0x30000, 0x3fffd, widthWide},
	unicodeRange{0xe0100, 0xe01ef, widthAmbiguous},
	unicodeRange{0xf0000, 0xffffd, widthAmbiguous},
	unicodeRange{0x100000, 0x10fffd, widthAmbiguous},
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

func (d *widthDetector) lookupGlyphWidthWithCache(glyph []uint16) codepointWidth {
	width := d.lookupGlyphWidth(glyph)
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
