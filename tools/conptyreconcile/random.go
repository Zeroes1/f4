package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// A randomised, seeded fixture. The bug that shipped -- keying the correction
// on the frame width instead of the write width -- survived a fixed fixture
// because that fixture happened not to expose it. A generator whose shape
// changes every run does not have that property, and because it is seeded, a
// seed that fails on Windows replays here against the mock.

func randomGroundTruth(seed int64, width, n int) []string {
	r := rand.New(rand.NewSource(seed))
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		switch r.Intn(10) {
		case 0:
			// Exactly the width: the merge case.
			out = append(out, strings.Repeat(pick(r, "=-*#"), width))
		case 1:
			// An exact multiple: merges the same way, over several rows.
			out = append(out, strings.Repeat(pick(r, "xo+"), width*(1+r.Intn(3))))
		case 2:
			// One short, one over: the near misses either side.
			d := 1
			if r.Intn(2) == 0 {
				d = -1
			}
			out = append(out, strings.Repeat("y", width+d))
		case 3:
			// A run of identical lines, which content matching must survive.
			s := strings.Repeat("-", width)
			for k := 0; k < 1+r.Intn(5); k++ {
				out = append(out, s)
			}
		case 4:
			// Long and not a multiple.
			out = append(out, strings.Repeat("w", width+1+r.Intn(width*3)))
		case 5:
			out = append(out, "")
		case 6:
			// Non-ASCII, so the cell measurement is exercised rather than
			// assumed: Cyrillic is two bytes per cell, CJK is one cell of
			// width two, and a line built to fill rows exactly in *cells*
			// merges while its byte length is not a multiple of anything.
			out = append(out, cyrillicOfCells(width, r.Intn(2)+1))
		case 7:
			out = append(out, wideOfCells(width))
		default:
			out = append(out, strings.TrimRight(
				fmt.Sprintf("line %06d %s", i, strings.Repeat(pick(r, ".:_"), r.Intn(width/2))), " "))
		}
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func pick(r *rand.Rand, set string) string {
	return string(set[r.Intn(len(set))])
}

// cyrillicOfCells returns a Cyrillic line of exactly n*width cells, which
// merges in the frame while being twice as many bytes.
func cyrillicOfCells(width, n int) string {
	return strings.Repeat("\u0442", width*n)
}

// wideOfCells returns a line of double-width glyphs filling exactly one row.
// An odd width leaves one cell that no wide glyph fits into, so conhost pads
// it -- the WasDoubleBytePadded case -- and the line is one cell short of
// filling the row, which is itself worth exercising.
func wideOfCells(width int) string {
	return strings.Repeat("\u4e2d", width/2)
}
