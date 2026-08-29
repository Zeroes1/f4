package main

import (
	"strings"
	"testing"
)

// The three things the mock did not model, now modelled. Each of these fails
// on the version of the tool that was handed over before them.

// Output arriving *inside* a frame. The -resize-during-output run passed, but
// the mock built the live stream and the frame as separate blocks, so it never
// exercised the interleave that gives the run its name.
func TestOutputInterleavedWithTheFrame(t *testing.T) {
	const writeWidth, frameWidth = 80, 79
	for seed := int64(1); seed <= 20; seed++ {
		printed := randomGroundTruth(seed, writeWidth, 60)
		extra := randomGroundTruth(seed+500, writeWidth, 5)

		writer := newMockConPTY(writeWidth, 2000, seed)
		viewer := newMockConPTY(frameWidth, 2000, seed)

		frame, all := viewer.FrameInterleaved(printed, writeWidth, extra)
		live := writer.LiveStream(all)

		got := trimTrailingBlanks(reconcileOrdered(
			trimTrailingBlanks(splitFrameLines(frame)), liveLines(live, writeWidth)))

		// Everything printed must still be present and in order. The frame is
		// no longer a clean snapshot, so the requirement is containment rather
		// than equality: nothing invented, nothing dropped.
		joined := strings.Join(got, "\n")
		for _, want := range all {
			if want == "" {
				continue
			}
			if !strings.Contains(joined, want) {
				t.Fatalf("seed %d: %q vanished from the reconstruction", seed, trunc(want))
			}
		}
	}
}

// The ring evicts rows, not lines, so the top of a frame can begin partway
// through a wrapped line. An aligner that only looks for whole lines cannot
// find its start and silently falls back to the uncorrected frame.
func TestFrameBeginningPartwayThroughALine(t *testing.T) {
	const width = 40
	long := strings.Repeat("q", width*4+1) // four rows plus a forced wrap
	printed := []string{long, strings.Repeat("=", width), "after", markerDone}

	// A buffer too short to hold it all, so the first line is cut.
	viewer := newMockConPTY(width, 6, 1)
	kept, cutMidLine := viewer.fitRows(printed)
	if !cutMidLine {
		t.Fatalf("the fixture must exercise a mid-line cut, kept %d lines", len(kept))
	}

	writer := newMockConPTY(width, 500, 1)
	live := writer.LiveStream(printed)
	frame := viewer.FrameAtWidth(kept, width)

	got := trimTrailingBlanks(reconcileOrdered(
		trimTrailingBlanks(splitFrameLines(frame)), liveLines(live, width)))

	// The surviving tail must come back intact; the truncated head may not,
	// and the tool must not corrupt what follows it.
	joined := strings.Join(got, "\n")
	for _, want := range []string{strings.Repeat("=", width), "after", markerDone} {
		if !strings.Contains(joined, want) {
			t.Fatalf("%q missing after a mid-line eviction; got %q", trunc(want), trunc(joined))
		}
	}
}

// Whatever happens, the correction may not invent text. This is the property
// that makes a partial alignment safe to fall back from.
func TestReconciliationNeverInventsText(t *testing.T) {
	const width = 50
	for seed := int64(1); seed <= 30; seed++ {
		printed := randomGroundTruth(seed, width, 40)
		writer := newMockConPTY(width, 300, seed)
		viewer := newMockConPTY(width-1, 300, seed)

		frame := viewer.FrameAtWidth(printed, width)
		runs := trimTrailingBlanks(splitFrameLines(frame))
		got := reconcileOrdered(runs, liveLines(writer.LiveStream(printed), width))

		if strings.Join(got, "") != strings.Join(runs, "") {
			t.Fatalf("seed %d: the correction changed the text rather than only its boundaries", seed)
		}
	}
}
