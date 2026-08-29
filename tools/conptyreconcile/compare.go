package main

import "strings"

// Comparison helpers, portable so the unit tests can use them.

func trimTrailingBlanks(in []string) []string {
	out := in
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func tailOf(all []string, n int) []string {
	if n >= len(all) || n <= 0 {
		return all
	}
	return all[len(all)-n:]
}

func diffCount(got, want []string) int {
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	bad := len(got) - n
	if d := len(want) - n; d > bad {
		bad = d
	}
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			bad++
		}
	}
	return bad
}

func trunc(s string) string {
	if len(s) > 70 {
		return s[:70] + "..."
	}
	return s
}

func max0(n int) int {
	if n < 0 {
		return 0
	}
	return n
}
