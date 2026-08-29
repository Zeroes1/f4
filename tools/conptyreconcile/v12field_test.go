package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// TestV12AgainstFieldDump feeds the ground truth of a field run through the
// ported v1.12 write path and the ported VT renderer, and compares the bytes
// they produce against what the real conhost emitted in that run.
//
// This is the check that makes local work meaningful: if our transpiled host
// emits what the pinned host emitted, then debugging against it is debugging
// against conhost, and a Windows run is a confirmation rather than a
// discovery.
func TestV12AgainstFieldDump(t *testing.T) {
	const seed, width, lines = int64(1788002866976838800), 120, 150
	chunks := unescapeDump(t, fmt.Sprintf("testdata/conptydump-%d.txt", seed))
	var all []byte
	for _, c := range chunks {
		all = append(all, c...)
	}
	i := bytes.Index(all, []byte("\x1b[8;"))
	if i < 0 {
		t.Skip("no frame marker")
	}
	live := all[:i]

	printed := append(randomGroundTruth(seed, width, lines), markerDone)

	// Drive the ported host with the same text the child printed.
	screen := newV12Screen(width, 2000)
	for _, l := range printed {
		v12WriteCharsLegacy(screen, l+"\r\n", wcFlags{delayEolWrap: true}, nil)
	}

	// What the ported buffer holds, read as logical lines the way the wrap
	// flag defines them.
	var got []string
	var cur strings.Builder
	last := screen.buffer.cursor.GetPosition().y
	for y := 0; y <= last; y++ {
		row := screen.buffer.GetRowByOffset(y)
		cur.WriteString(row.text())
		if !row.WasWrapForced() {
			got = append(got, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		got = append(got, cur.String())
	}

	// The real host's live stream, read by the reconciler's own splitter.
	want := liveLines(live, width)

	for len(got) > 0 && got[len(got)-1] == "" {
		got = got[:len(got)-1]
	}
	if len(got) != len(want) {
		t.Fatalf("ported host produced %d logical lines, real conhost %d", len(got), len(want))
	}
	bad := 0
	for k := range got {
		if got[k] != want[k].Text {
			if bad < 3 {
				t.Errorf("line %d differs\n  ported %q\n  real   %q",
					k, clipRunes(got[k], 40), clipRunes(want[k].Text, 40))
			}
			bad++
		}
	}
	if bad > 0 {
		t.Fatalf("%d of %d lines differ", bad, len(got))
	}
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}
