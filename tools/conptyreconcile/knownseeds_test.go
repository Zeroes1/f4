package main

import (
	"fmt"
	"testing"
)

// TestKnownSeedsAgainstTheMock runs every seed of the probe's built-in suite
// (knownSeeds in main_windows.go, the same list `conptyreconcile -suite`
// executes on Windows) through the mock pipeline. The two are complementary
// and neither replaces the other: this one runs everywhere and guards the
// reconciler against regressions, the Windows one is the only thing that can
// find a new disagreement with conhost.
//
// A seed lands in that list because a real run once failed on it. Keeping it
// exercised here means the fix cannot silently rot between Windows runs.
func TestKnownSeedsAgainstTheMock(t *testing.T) {
	const width, height, lines = 120, 2000, 150
	for _, ks := range knownSeeds {
		ks := ks
		t.Run(fmt.Sprint(ks.seed), func(t *testing.T) {
			all := append(randomGroundTruth(ks.seed, width, lines), markerDone)
			m := newMockConPTY(width, height, ks.seed)
			live := m.LiveStream(all)
			frame := m.Frame(all)
			liveChunks := m.Chunks(live, 97)
			var frameBuf []byte
			for _, c := range m.Chunks(frame, 313) {
				frameBuf = append(frameBuf, c...)
			}
			recovered := trimTrailingBlanks(reconcileOrdered(
				trimTrailingBlanks(splitFrameLines(frameBuf)),
				liveLinesFromChunks(liveChunks, width), width))
			truth := m.fit(all)
			if len(recovered) != len(truth) || diffCount(recovered, truth) != 0 {
				t.Fatalf("seed %d (%s): recovered %d lines, expected %d",
					ks.seed, ks.why, len(recovered), len(truth))
			}
		})
	}
}
