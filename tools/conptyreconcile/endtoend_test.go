package main

import (
	"fmt"
	"strings"
	"testing"
)

// Ground truth: a set of logical lines chosen so that every shape the
// measurements found is present -- ordinary lines, a line of exactly the
// width, a line of exactly twice the width, near misses at W-1 and W+1, a
// long line that genuinely wraps, and a run of identical separators, which is
// the case content-matching has to survive.
func groundTruth(width, n int) []string {
	out := make([]string, 0, n+16)
	for i := 1; i <= n; i++ {
		switch i % 7 {
		case 0:
			out = append(out, strings.Repeat("=", width)) // exact: merges
		case 1:
			out = append(out, fmt.Sprintf("line %06d short", i))
		case 2:
			out = append(out, strings.Repeat("x", width*2)) // exact multiple: merges
		case 3:
			out = append(out, strings.Repeat("y", width-1)) // near miss
		case 4:
			out = append(out, strings.Repeat("z", width+1)) // near miss
		case 5:
			out = append(out, strings.Repeat("w", width*3+7)) // genuine wrap
		default:
			// No trailing whitespace: a frame erases to end of line, so
			// trailing spaces are not recoverable from it by anyone. That is
			// a property of the medium, not of this correction.
			out = append(out, strings.TrimRight(
				fmt.Sprintf("payload %06d %s", i, strings.Repeat("-", i%40)), " "))
		}
	}
	// Ten identical separators in a row, each exactly the width.
	for i := 0; i < 10; i++ {
		out = append(out, strings.Repeat("-", width))
	}
	out = append(out, "tail after the separators")
	return out
}

func runCase(t *testing.T, width, height, lines int, seed int64, scrolling bool) (recovered, truth []string) {
	t.Helper()
	m := newMockConPTY(width, height, seed)
	all := groundTruth(width, lines)

	var live []byte
	if scrolling {
		live = m.LiveStreamScrolling(all)
	} else {
		live = m.LiveStream(all)
	}
	frame := m.Frame(all)

	// Feed both through jittered chunks and reassemble, which is what a real
	// reader does. A parser that only works on whole buffers fails here.
	var liveBuf, frameBuf []byte
	for _, c := range m.Chunks(live, 97) {
		liveBuf = append(liveBuf, c...)
	}
	for _, c := range m.Chunks(frame, 313) {
		frameBuf = append(frameBuf, c...)
	}

	// The frame's blank filler rows are trimmed before reconciling: they are
	// buffer padding, not printed lines, and the live sequence has no
	// counterpart for them.
	ll := liveLines(liveBuf, width)
	frameLines := trimTrailingBlanks(splitFrameLines(frameBuf))
	recovered = reconcileOrdered(frameLines, ll)

	// Ground truth is what survived the ring, with trailing blanks removed.
	truth = m.fit(all)
	return trimTrailingBlanks(recovered), truth
}

func compare(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: recovered %d logical lines, expected %d", name, len(got), len(want))
	}
	n := len(got)
	if len(want) < n {
		n = len(want)
	}
	bad := 0
	for i := 0; i < n; i++ {
		if got[i] != want[i] {
			bad++
			if bad <= 3 {
				t.Errorf("%s: line %d\n  got  %q\n  want %q", name, i, trunc(got[i]), trunc(want[i]))
			}
		}
	}
	if bad > 3 {
		t.Errorf("%s: %d lines differ in total", name, bad)
	}
}

func TestReconciliationRecoversGroundTruthNoScroll(t *testing.T) {
	got, want := runCase(t, 120, 2000, 300, 1, false)
	compare(t, "no-scroll", got, want)
}

func TestReconciliationRecoversGroundTruthWithOverflow(t *testing.T) {
	// More content than the buffer holds, so the ring evicts from the top.
	got, want := runCase(t, 120, 500, 900, 2, false)
	compare(t, "overflow", got, want)
}

func TestReconciliationSurvivesASplittingLiveStream(t *testing.T) {
	// §17: while the buffer scrolls the live stream splits long lines. The
	// correction must not be misled by that -- a split long line must not
	// become a recorded boundary that then cuts a genuine wrap.
	got, want := runCase(t, 120, 2000, 300, 3, true)
	compare(t, "scrolling-live-stream", got, want)
}

func TestReconciliationAcrossManyWidths(t *testing.T) {
	for _, w := range []int{40, 80, 119, 120, 200} {
		got, want := runCase(t, w, 1000, 200, int64(w), false)
		compare(t, fmt.Sprintf("width=%d", w), got, want)
	}
}

func TestWithoutTheCorrectionTheFrameIsWrong(t *testing.T) {
	// The control: the same frame read without the live-stream boundaries
	// must lose lines, or the test above proves nothing.
	width, height := 120, 2000
	m := newMockConPTY(width, height, 9)
	all := groundTruth(width, 300)
	frameLines := trimTrailingBlanks(splitFrameLines(m.Frame(all)))
	truth := m.fit(all)
	if len(frameLines) >= len(truth) {
		t.Fatalf("expected the uncorrected frame to have merged lines: %d vs %d",
			len(frameLines), len(truth))
	}
	t.Logf("uncorrected frame carries %d logical lines where the truth has %d "+
		"-- %d boundaries lost to P13", len(frameLines), len(truth), len(truth)-len(frameLines))
}

func TestChunkingDoesNotChangeTheResult(t *testing.T) {
	// Jitter must be irrelevant to the outcome; if it is not, the parser is
	// depending on buffer boundaries it will not get in the field.
	var first []string
	for seed := int64(1); seed <= 8; seed++ {
		got, want := runCase(t, 120, 1000, 150, seed, false)
		compare(t, fmt.Sprintf("seed=%d", seed), got, want)
		if first == nil {
			first = got
			continue
		}
		if len(first) != len(got) {
			t.Fatalf("seed %d produced %d lines, seed 1 produced %d", seed, len(got), len(first))
		}
	}
}
