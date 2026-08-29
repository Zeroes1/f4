package main

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

// WHAT THIS SWEEP CAN AND CANNOT FIND -- read before trusting a green run.
//
// 3000 seeds pass here while seed 1788002866976838800 failed on the first
// Windows run that touched it. That is not a paradox and not luck: the sweep
// drives the MOCK, and the mock is built from the same model as the code it
// tests. Its frames come from the same ported buffer, its merge decisions
// from the same fillsRowsExactly. It can only disagree with the reconciler
// by accident, never on purpose. conhost disagrees on purpose -- it has the
// legacy write path, the repaint heuristics and twenty years of behaviour
// this project is still discovering.
//
// So the sweep is a REGRESSION net, not a discovery instrument: it proves the
// pipeline is self-consistent across shapes of input, and it will catch a
// change that breaks that consistency. Every genuinely new fact in this
// project came from a field run or a field dump, and each new dump is worth
// more than another thousand mock rounds (TestFieldDumpsAll).
//
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
	// Parallel across CPUs: the sweep is pure computation and 300 rounds took
	// 68s serially, which is enough friction to discourage running it.
	const width, height, lines, n = 120, 2000, 150, 3000
	base := int64(1788000000000000000)
	failed := 0
	var bad []int64
	var mu sync.Mutex
	var wg sync.WaitGroup
	workers := runtime.NumCPU()
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for k := w; k < n; k += workers {
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
					mu.Lock()
					failed++
					if len(bad) < 12 {
						bad = append(bad, seed)
					}
					mu.Unlock()
				}
			}
		}(w)
	}
	wg.Wait()
	fmt.Printf("sweep: %d/%d failed; first failing seeds: %v\n", failed, n, bad)
	if failed > 0 {
		t.Errorf("%d of %d seeds fail against the mock", failed, n)
	}
}
