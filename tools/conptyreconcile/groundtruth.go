package main

import (
	"fmt"
	"strings"
)

// groundTruthLines is the same list the unit tests use, so a failure on
// Windows reproduces against the mock without Windows. `width` is passed in
// through the -long flag so the child needs no extra plumbing.
func groundTruthLines(width, n int) []string {
	out := make([]string, 0, n+16)
	for i := 1; i <= n; i++ {
		switch i % 7 {
		case 0:
			out = append(out, strings.Repeat("=", width))
		case 1:
			out = append(out, fmt.Sprintf("line %06d short", i))
		case 2:
			out = append(out, strings.Repeat("x", width*2))
		case 3:
			out = append(out, strings.Repeat("y", width-1))
		case 4:
			out = append(out, strings.Repeat("z", width+1))
		case 5:
			out = append(out, strings.Repeat("w", width*3+7))
		default:
			out = append(out, strings.TrimRight(
				fmt.Sprintf("payload %06d %s", i, strings.Repeat("-", i%40)), " "))
		}
	}
	for i := 0; i < 10; i++ {
		out = append(out, strings.Repeat("-", width))
	}
	out = append(out, "tail after the separators")
	return out
}

// markerDone is the end-of-output marker the child prints after the fixture.
// It is part of what the tool must recover, so it belongs in the expected list
// as well -- a real capture once reported one line short because it did not.
const markerDone = "~DONE~"
