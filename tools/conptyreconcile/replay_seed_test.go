package main

import "testing"

// TestReplaySeed1788002866976838800 replays the seed that failed in the field
// (64 recovered lines of 151) against the mock, using the canonical harness
// of runCase. If this passes while the field run failed, the mock does not
// model what conhost did on that stream -- which is a finding about the mock,
// not about the reconciler.
func TestReplaySeed1788002866976838800(t *testing.T) {
	const seed, width, height, lines = int64(1788002866976838800), 120, 2000, 150
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
	if len(recovered) != len(truth) {
		t.Fatalf("recovered %d lines, expected %d", len(recovered), len(truth))
	}
	if n := diffCount(recovered, truth); n != 0 {
		t.Fatalf("%d lines differ", n)
	}
}
