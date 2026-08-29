package main

import "testing"

// The mock is only worth anything if it reproduces what was measured. These
// pin the two numbers that are checkable without Windows: an empty buffer
// costs five bytes per row, and a frame carries one terminator per logical
// line that does not exactly fill its rows.
func TestMockEmptyBufferCostsFiveBytesPerRow(t *testing.T) {
	for _, h := range []int{500, 2000, 32000} {
		m := newMockConPTY(120, h, 1)
		f := m.Frame(nil)
		// header + h terminators + tail; the per-row term is the measured 5.
		perRow := 5 * h
		if len(f) < perRow || len(f) > perRow+64 {
			t.Fatalf("height %d: frame is %d bytes, expected %d plus a small header/tail",
				h, len(f), perRow)
		}
	}
}

func TestMockMergesAnExactWidthLineLikeConhostDoes(t *testing.T) {
	m := newMockConPTY(20, 100, 1)
	exact := "12345678901234567890" // exactly 20
	f := string(m.Frame([]string{exact, "next"}))
	if !contains(f, exact+"next") {
		t.Fatal("an exact-width line must arrive glued to its successor, as measured in §17")
	}
}

func contains(h, n string) bool {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return true
		}
	}
	return false
}
