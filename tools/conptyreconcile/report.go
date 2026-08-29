package main

import "fmt"

// analyseStreams is the whole of this tool's reasoning, kept portable so that
// every branch of it -- including the ones that only fire when something is
// wrong -- is exercised by unit tests rather than discovered on a tester's
// machine. The Windows file above only captures bytes and prints what this
// returns.

// analyseStreams takes the width the child was writing at, which is not the
// width the frame was taken at. That distinction is the whole of a bug this
// tool shipped with once: conhost merges a line into its successor when the
// line exactly fills the rows *at the moment it is written*, so the boundary
// inside a merged run sits at a multiple of the write-time width. Looking for
// multiples of the frame width found nothing at all -- the correction ran and
// changed not one line -- while the frame itself had merged exactly as
// predicted. The mock could not catch it, because there the frame and the
// writing always happened at one width; it can now.
func analyseStreams(live, frame []byte, writeWidth, lines, long int) []string {
	// The child prints an end marker after the fixture, and the expected list
	// must contain it or the very last line reads as a mismatch. A real
	// capture reported "160 of 161" for exactly this reason: the algorithm was
	// right and the bookkeeping was one line short. The mock never caught it
	// because there the child prints the fixture and nothing else.
	truth := append(groundTruthLines(long, lines), markerDone)
	frameLines := trimTrailingBlanks(splitFrameLines(frame))
	ll := liveLines(live, writeWidth)
	fixed := trimTrailingBlanks(reconcileOrdered(frameLines, ll, frameWidthFromFrame(frame)))

	out := []string{
		"",
		fmt.Sprintf("live stream:  %d bytes, %d logical lines", len(live), len(ll)),
		fmt.Sprintf("first frame:  %d bytes, %d logical lines before the correction",
			len(frame), len(frameLines)),
		fmt.Sprintf("corrected:    %d logical lines", len(fixed)),
		"",
	}

	if len(frameLines) == 0 {
		return append(out,
			"NO FRAME: the resize produced no repaint at all.",
			"On a build carrying the post-#17510 emitter this is the expected result",
			"and is itself the answer -- see the generation table in section 17 of",
			"docs/CONPTY_RESEARCH.md.")
	}

	want := tailOf(truth, len(fixed))
	missAfter := diffCount(fixed, want)
	missBefore := diffCount(frameLines, tailOf(truth, len(frameLines)))

	out = append(out,
		fmt.Sprintf("against the printed lines: %d of %d match after the correction",
			len(want)-missAfter, len(want)),
		fmt.Sprintf("without the correction the same comparison misses %d", missBefore),
		"")

	if missAfter == 0 {
		return append(out, "PASS -- every printed line was recovered")
	}

	out = append(out, fmt.Sprintf("FAIL -- %d lines still wrong; the dump beside this log has the bytes", missAfter))
	for i := 0; i < len(fixed) && i < len(want); i++ {
		if fixed[i] != want[i] {
			out = append(out,
				fmt.Sprintf("  first mismatch at line %d", i),
				fmt.Sprintf("    got  %s", trunc(fixed[i])),
				fmt.Sprintf("    want %s", trunc(want[i])))
			break
		}
	}
	return out
}
