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
