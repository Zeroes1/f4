package main

import (
	"strings"
	"testing"
)

// Every branch of the report, exercised here rather than on a tester's
// machine. The previous version of this program was handed over without a
// single one of its exit paths having been run, and it died on startup.

func joined(lines []string) string { return strings.Join(lines, "\n") }

func TestReportPassesOnAGoodCapture(t *testing.T) {
	// Written at 120, the frame taken at 119 -- the shape of a real run.
	const writeWidth, frameWidth, height, lines = 120, 119, 2000, 200
	writer := newMockConPTY(writeWidth, height, 7)
	viewer := newMockConPTY(frameWidth, height, 7)
	// analyseStreams expects the child's end marker, so the capture must
	// contain it -- the same shape a real run produces.
	printed := append(groundTruthLines(writeWidth, lines), markerDone)

	got := analyseStreams(writer.LiveStream(printed),
		viewer.FrameAtWidth(printed, writeWidth), writeWidth, lines, writeWidth)
	if !strings.Contains(joined(got), "PASS") {
		t.Fatalf("expected PASS, got:\n%s", joined(got))
	}
}

func TestCollectedFieldSeedReplaysAgainstAuditedMock(t *testing.T) {
	// This is the fresh seed printed by conptydump-2000.txt. Keep it as an
	// end-to-end regression: the field failure was caused by global reflow of
	// wide cells in a 120-to-119 repaint, not by the synthetic fixtures.
	const seed int64 = 1787996042561810700
	const writeWidth, frameWidth, height, lines = 120, 119, 2000, 150
	printed := append(randomGroundTruth(seed, writeWidth, lines), markerDone)

	writer := newMockConPTY(writeWidth, height, seed)
	viewer := newMockConPTY(frameWidth, height, seed)
	liveChunks := writer.Chunks(writer.LiveStream(printed), 97)
	frameChunks := viewer.Chunks(viewer.FrameAtWidth(printed, writeWidth), 313)

	var frame []byte
	for _, chunk := range frameChunks {
		frame = append(frame, chunk...)
	}
	got := trimTrailingBlanks(reconcileOrdered(
		trimTrailingBlanks(splitFrameLines(frame)),
		liveLinesFromChunks(liveChunks, writeWidth), frameWidth))
	want := trimTrailingBlanks(printed)
	if len(got) != len(want) {
		t.Fatalf("field seed recovered %d lines, expected %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("field seed line %d: got %q, want %q", i, trunc(got[i]), trunc(want[i]))
		}
	}
}

func TestReportSaysNoFrameWhenTheResizeEmitsNothing(t *testing.T) {
	// The post-#17510 emitter case: a resize produces no repaint. The tool
	// must say so plainly instead of reporting zero matches.
	const width, lines = 120, 50
	m := newMockConPTY(width, 500, 3)
	got := analyseStreams(m.LiveStream(groundTruthLines(width, lines)), nil, width, lines, width)
	j := joined(got)
	if !strings.Contains(j, "NO FRAME") {
		t.Fatalf("expected the NO FRAME branch, got:\n%s", j)
	}
	if strings.Contains(j, "PASS") || strings.Contains(j, "FAIL") {
		t.Fatalf("NO FRAME must not also claim a verdict:\n%s", j)
	}
}

func TestReportFailsAndNamesTheFirstMismatch(t *testing.T) {
	// A live stream that recorded nothing leaves the frame's merges in place,
	// which must be reported as a failure with a concrete line.
	const writeWidth, frameWidth, lines = 120, 119, 200
	viewer := newMockConPTY(frameWidth, 2000, 11)
	truth := groundTruthLines(writeWidth, lines)

	got := analyseStreams(nil, viewer.FrameAtWidth(truth, writeWidth), writeWidth, lines, writeWidth)
	j := joined(got)
	if !strings.Contains(j, "FAIL") {
		t.Fatalf("expected FAIL without any live stream, got:\n%s", j)
	}
	if !strings.Contains(j, "first mismatch at line") {
		t.Fatalf("a failure must name a line:\n%s", j)
	}
}

func TestReportNeverPanicsOnGarbage(t *testing.T) {
	// Whatever arrives, the tool prints a report and returns. It may say
	// anything, but it may not crash: that is the failure mode that cost a
	// trip with a USB stick.
	cases := [][2][]byte{
		{nil, nil},
		{[]byte{}, []byte{}},
		{[]byte("\x1b["), []byte("\x1b[")},
		{[]byte("\x1b[?25l\x1b[H"), []byte("\x1b[?25l\x1b[8;")},
		{[]byte(strings.Repeat("\x1b", 100)), []byte(strings.Repeat("\r\n", 100))},
		{[]byte("no escapes at all"), []byte("plain text with no terminators")},
	}
	for i, c := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("case %d panicked: %v", i, r)
				}
			}()
			if out := analyseStreams(c[0], c[1], 120, 10, 120); len(out) == 0 {
				t.Fatalf("case %d produced no report", i)
			}
		}()
	}
}

func TestReportHandlesAWidthOfOne(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("width 1 panicked: %v", r)
		}
	}()
	analyseStreams([]byte("x\r\n"), []byte("x\x1b[K\r\n"), 1, 5, 4)
}

// The regression that a real capture found and the mock did not: a frame taken
// at one width over lines written at another. The correction must key on the
// write width; keying on the frame width finds no candidates at all and
// silently changes nothing.
func TestFrameTakenAtADifferentWidthThanTheLinesWereWritten(t *testing.T) {
	const writeWidth, frameWidth, lines = 120, 119, 200
	truth := groundTruthLines(writeWidth, lines)

	writer := newMockConPTY(writeWidth, 2000, 5)
	viewer := newMockConPTY(frameWidth, 2000, 5)

	live := writer.LiveStream(truth)
	frame := viewer.FrameAtWidth(truth, writeWidth)

	// Keying on the frame width: the failure that shipped. It does not merely
	// find nothing -- it finds the wrong thing. The fixture's "near miss" line
	// of writeWidth-1 characters is an exact multiple of the frame width, so a
	// spurious candidate is recorded and can cut a line that never merged.
	wrongBreaks := liveLines(live, frameWidth)
	wrong := trimTrailingBlanks(reconcileOrdered(trimTrailingBlanks(splitFrameLines(frame)), wrongBreaks, frameWidth))
	if len(wrong) == len(truth) {
		t.Fatalf("keying on the frame width must not recover the truth by accident")
	}

	// Keying on the write width: correct.
	breaks := liveLines(live, writeWidth)
	fixed := trimTrailingBlanks(reconcileOrdered(trimTrailingBlanks(splitFrameLines(frame)), breaks, frameWidth))
	if len(fixed) != len(truth) {
		t.Fatalf("recovered %d lines, expected %d", len(fixed), len(truth))
	}
	for i := range truth {
		if fixed[i] != truth[i] {
			t.Fatalf("line %d:\n got  %s\n want %s", i, trunc(fixed[i]), trunc(truth[i]))
		}
	}
}

// Randomised rounds against the mock. The same generator and seeds run on
// Windows, so a seed that fails there is reproduced here without Windows.
func TestRandomisedRoundsAgainstTheMock(t *testing.T) {
	const writeWidth, frameWidth = 120, 119
	for seed := int64(1); seed <= 60; seed++ {
		truth := randomGroundTruth(seed, writeWidth, 120)
		writer := newMockConPTY(writeWidth, 4000, seed)
		viewer := newMockConPTY(frameWidth, 4000, seed)

		breaks := liveLines(writer.LiveStream(truth), writeWidth)
		frame := viewer.FrameAtWidth(truth, writeWidth)
		got := trimTrailingBlanks(reconcileOrdered(trimTrailingBlanks(splitFrameLines(frame)), breaks, frameWidth))

		want := trimTrailingBlanks(truth)
		if len(got) != len(want) {
			t.Fatalf("seed %d: recovered %d lines, expected %d", seed, len(got), len(want))
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("seed %d line %d:\n got  %s\n want %s", seed, i, trunc(got[i]), trunc(want[i]))
			}
		}
	}
}

// The window is resized while the command is still printing: lines before the
// resize were written at one width, lines after it at another, and whether a
// line merged depends on which. Fixing on a single width gets the second half
// wrong; the tracking form reads the width out of the size report ConPTY sends.
func TestResizeWhileOutputIsStillArriving(t *testing.T) {
	const w1, w2 = 120, 100
	for seed := int64(1); seed <= 25; seed++ {
		first := randomGroundTruth(seed, w1, 60)
		second := randomGroundTruth(seed+1000, w2, 60)

		before := newMockConPTY(w1, 4000, seed)
		after := newMockConPTY(w2, 4000, seed)

		// The live stream: output at w1, then the size report that announces
		// w2, then output at w2.
		live := append([]byte(nil), before.LiveStream(first)...)
		live = append(live, []byte("\x1b[8;4000;100t")...)
		live = append(live, after.LiveStream(second)...)

		all := append(append([]string{}, first...), second...)
		// The final frame is taken at w2, but the first half was written at w1.
		frame := after.FrameAtWidths(all, len(first), w1, w2)

		breaks := liveLines(live, w1)
		got := trimTrailingBlanks(reconcileOrdered(trimTrailingBlanks(splitFrameLines(frame)), breaks, w2))

		fixedFixed := liveLines(live, w1)
		naive := trimTrailingBlanks(reconcileOrdered(trimTrailingBlanks(splitFrameLines(frame)), fixedFixed, w2))

		want := trimTrailingBlanks(all)
		if len(got) != len(want) {
			t.Fatalf("seed %d: tracking recovered %d, expected %d", seed, len(got), len(want))
		}
		if len(naive) == len(want) && seed == 1 {
			t.Log("note: on this seed the single-width reading happens to agree")
		}
	}
}

// The harness prints an end marker after the fixture. A capture once came back
// "160 of 161" purely because the expected list did not contain it: the
// algorithm was right and the bookkeeping was one line short. The mock could
// not catch it, because there the child prints the fixture and nothing else --
// so the case is modelled explicitly now.
func TestChildPrintsMoreThanTheFixture(t *testing.T) {
	const writeWidth, frameWidth, lines = 120, 119, 80
	printed := append(groundTruthLines(writeWidth, lines), markerDone)

	writer := newMockConPTY(writeWidth, 2000, 4)
	viewer := newMockConPTY(frameWidth, 2000, 4)

	got := analyseStreams(writer.LiveStream(printed),
		viewer.FrameAtWidth(printed, writeWidth), writeWidth, lines, writeWidth)

	if !strings.Contains(strings.Join(got, "\n"), "PASS") {
		t.Fatalf("the end marker must be part of what is expected:\n%s", strings.Join(got, "\n"))
	}
}
