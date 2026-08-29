package main

import (
	"fmt"
	"testing"
)

// TestSeedSweep runs the mock pipeline for many random seeds, using the exact
// harness of runCase (endtoend_test.go) with the probe's random generator.
// The reconciler and the mock are portable, so algorithmic failures surface
// here without Windows; a field run is then only needed to confirm the mock
// still matches real conhost.
//
// The first version of this sweep hand-rolled the harness with a frame taken
// at width-1 and reconciled at width-1, which is not what either the probe or
// the e2e tests do, and reported 300/300 false failures. The harness below is
// runCase's, line for line.
func TestSeedSweep(t *testing.T) {
	if testing.Short() {
		t.Skip("sweep is slow")
	}
	const width, height, lines, n = 120, 2000, 150, 300
	base := int64(1788000000000000000)
	failed := 0
	var bad []int64
	for k := 0; k < n; k++ {
		seed := base + int64(k)*7919
		all := append(randomGroundTruth(seed, width, lines), markerDone)

		m := newMockConPTY(width, height, seed)
		live := m.LiveStream(all)
		frame := m.Frame(all)

		liveChunks := m.Chunks(live, 97)
		var frameBuf []byte
		for _, c := range m.Chunks(frame, 313) {
			frameBuf = append(frameBuf, c...)
		}

		ll := liveLinesFromChunks(liveChunks, width)
		frameLines := trimTrailingBlanks(splitFrameLines(frameBuf))
		recovered := trimTrailingBlanks(reconcileOrdered(frameLines, ll, width))
		truth := m.fit(all)

		if len(recovered) != len(truth) || diffCount(recovered, truth) != 0 {
			failed++
			if len(bad) < 12 {
				bad = append(bad, seed)
			}
		}
	}
	fmt.Printf("sweep: %d/%d failed; first failing seeds: %v\n", failed, n, bad)
	if failed > 0 {
		t.Errorf("%d of %d seeds fail against the mock", failed, n)
	}
}
