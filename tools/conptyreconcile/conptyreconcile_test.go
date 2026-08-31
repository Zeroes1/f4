package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPinnedUnicodeWidthAndUTF16Families(t *testing.T) {
	detector := newWidthDetector()
	cases := []struct {
		name string
		text string
		wide bool
	}{
		{name: "ascii", text: "A", wide: false},
		{name: "cjk", text: "漢", wide: true},
		{name: "emoji", text: "😀", wide: true},
		{name: "ambiguous", text: "Ω", wide: false},
		{name: "box drawing pinned narrow", text: "─", wide: false},
		{name: "combining", text: "\u0301", wide: false},
		{name: "variation selector", text: "\ufe0f", wide: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := detector.IsWide(utf16Units(tc.text)); got != tc.wide {
				t.Fatalf("IsWide(%q)=%v, want %v", tc.text, got, tc.wide)
			}
		})
	}

	for _, text := range []string{"😀", "👩‍💻", "🏳️‍🌈", "אבג العربية 123"} {
		units := utf16Units(text)
		if got := string(runesFromUTF16(units)); got != text {
			t.Fatalf("UTF-16 round trip changed %q to %q", text, got)
		}
	}
}

func TestIncrementalParserPreservesPinnedInputBoundaries(t *testing.T) {
	input := []byte("A漢e\u0301☕️ 😀 👩‍💻 אבג 123\r\n" +
		"\x1b[31mCSI\x1b[0m\x1b]2;OSC title\x07" +
		"\x1b[2;3Hcursor\x1b[K\r\n__END__")
	whole, err := parseWithChunks(40, 20, [][]byte{input})
	if err != nil {
		t.Fatal(err)
	}
	chunked, err := parseWithChunks(40, 20, allByteChunks(input))
	if err != nil {
		t.Fatal(err)
	}
	if whole.snapshot() != chunked.snapshot() {
		t.Fatalf("one-byte chunking changed snapshot:\nwhole=%+v\nchunked=%+v", whole.snapshot(), chunked.snapshot())
	}
	if whole.title != "OSC title" {
		t.Fatalf("OSC title=%q", whole.title)
	}
	if !bytes.Contains([]byte(whole.buffer.text()), []byte("__END__")) {
		t.Fatalf("end marker missing from buffer text %q", whole.buffer.text())
	}
}

func TestEqualRowsRemainDistinctInReconciliation(t *testing.T) {
	f := frame{Width: 4, Height: 2, Rows: []frameRow{
		{Units: utf16Units("same"), SourceIndex: 0},
		{Units: utf16Units("same"), SourceIndex: 1},
	}}
	live := []logicalRow{
		{units: utf16Units("same"), rows: []int{0}},
		{units: utf16Units("same"), rows: []int{1}, sourceStart: 1},
	}
	if err := reconcile(f, live); err != nil {
		t.Fatal(err)
	}
}

func TestRecordedSeedsAreExactly300AndUnique(t *testing.T) {
	if err := validateRecordedSeeds(); err != nil {
		t.Fatal(err)
	}
}

func TestNativeProbeWorkloadHasStableHandoffMarkers(t *testing.T) {
	workload := probeWorkload()
	if workload == "" {
		t.Fatal("native probe workload is empty")
	}
	last := -1
	for _, marker := range probeExpectedMarkers() {
		position := strings.Index(workload, marker)
		if position <= last {
			t.Fatalf("marker %q is missing or out of order in workload", marker)
		}
		if strings.Count(workload, marker) != 1 {
			t.Fatalf("marker %q occurs %d times, want one", marker, strings.Count(workload, marker))
		}
		last = position
	}
	if !strings.Contains(workload, strings.Repeat("C", 257)) {
		t.Fatal("long-line fixture is missing")
	}
}

func TestNativeProbeMarkerSurvivesTerminalControls(t *testing.T) {
	output := []byte("__F4_NATIVE_\x1b[2J\r\nPROBE_END__")
	if !probeOutputContainsMarker(output, probeEndMarker) {
		t.Fatalf("marker was not recovered from terminal-control-separated output")
	}
}

func TestMockMatrixAndRecordedSeeds(t *testing.T) {
	for _, width := range edgeScenarioWidths() {
		if err := runMockScenario(edgeScenario(width)); err != nil {
			t.Fatalf("matrix width %d: %v", width, err)
		}
	}
	for _, seed := range recordedSeeds {
		scenario := scenarioForSeed(int64(seed))
		if err := runMockScenario(scenario); err != nil {
			t.Fatalf("recorded seed %d: %v", seed, err)
		}
	}
}

func TestRandomDelayBoundaries(t *testing.T) {
	for _, seed := range recordedSeeds {
		if err := runDelayedMockScenario(scenarioForSeed(int64(seed)), int64(seed^0x9e3779b97f4a7c15)); err != nil {
			t.Fatalf("delay seed %d: %v", seed, err)
		}
	}
}

func FuzzIncrementalParser(f *testing.F) {
	f.Add([]byte("plain\x1b[31mtext\x1b[0m"))
	f.Add([]byte("\x1b]2;title\x07漢\xf0\x9f\x98\x80"))
	f.Add([]byte("\x1b[2;3H\x1b[K\x18garbage\x1b\\"))
	f.Fuzz(func(t *testing.T, input []byte) {
		if len(input) > 1<<16 {
			t.Skip()
		}
		parser := newVTParser(40, 20)
		for _, chunk := range allByteChunks(input) {
			if err := parser.feed(chunk); err != nil {
				t.Fatalf("feed: %v", err)
			}
		}
		if err := parser.finish(); err != nil {
			t.Fatalf("finish: %v", err)
		}
	})
}
