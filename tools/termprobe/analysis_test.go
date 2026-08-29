package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestLineRowsReadsBothConPTYShapesAlike(t *testing.T) {
	// P6/19045 shape: the wrap point carries a hard CRLF and full rows are
	// padded. P11/22000 shape: the line is written whole and the terminal's
	// own autowrap places it. Both describe one logical line over two rows.
	width := 20
	body := strings.Repeat("x", 30)
	crlfShape := []byte(markerLongStart + body[:16] + "\r\n" + body[16:] + markerLongEnd)
	wholeShape := []byte(markerLongStart + body + markerLongEnd)

	if got := lineRows(crlfShape, width, markerLongStart); got != 1 {
		t.Fatalf("start marker should sit on one row in the CRLF shape, got %d", got)
	}
	if got := lineRows(wholeShape, width, markerLongStart); got != 1 {
		t.Fatalf("start marker should sit on one row in the whole-line shape, got %d", got)
	}
	// The end marker is on a later row in both, which is what "wrapped" means.
	if got := lineRows(crlfShape, width, markerLongEnd); got != 1 {
		t.Fatalf("end marker rows in CRLF shape: %d", got)
	}
}

func TestLineRowsDetectsRejoinAtWideWidth(t *testing.T) {
	body := strings.Repeat("x", 300)
	raw := []byte(markerLongStart + body + markerLongEnd)
	if got := lineRows(raw, 80, markerLongStart); got != 1 {
		t.Fatalf("marker should occupy one row, got %d", got)
	}
	// At 4000 columns the whole line, markers included, is one row: the
	// rejoin test the F4 trick depends on.
	g := newWideGrid(4000)
	g.feed(raw)
	if g.maxRow != 0 {
		t.Fatalf("a 300-char line at width 4000 should not wrap, occupied %d rows", g.maxRow+1)
	}
	if g.rowsContaining(markerLongEnd) != 1 {
		t.Fatal("the end marker should be on the same single row")
	}
}

func TestCountFillMarkersReportsSpreadNotJustCount(t *testing.T) {
	var b strings.Builder
	for _, n := range []int{1, 2, 3, 900, 901} {
		fmt.Fprintf(&b, "%s line\r\n", fillMarker(n))
	}
	count, lo, hi := countFillMarkers([]byte(b.String()))
	if count != 5 || lo != 1 || hi != 901 {
		t.Fatalf("count=%d lo=%d hi=%d, want 5/1/901", count, lo, hi)
	}
}

func TestCountFillMarkersIgnoresMalformed(t *testing.T) {
	count, _, _ := countFillMarkers([]byte("~Fabc~ ~F12~ ~F000007~"))
	if count != 1 {
		t.Fatalf("only the well formed marker should count, got %d", count)
	}
}

func TestCountFillMarkersDeduplicates(t *testing.T) {
	// A repaint can carry the same row twice; the history question is about
	// distinct lines, not occurrences.
	count, _, _ := countFillMarkers([]byte(fillMarker(4) + " " + fillMarker(4)))
	if count != 1 {
		t.Fatalf("duplicates should collapse, got %d", count)
	}
}

func TestAnalyseFrameRecognisesTheRepaintShape(t *testing.T) {
	frame := []byte("\x1b[?25l\x1b[8;30;120t\x1b[H" + fillMarker(1) + "\r\n" + fillMarker(2) + "\r\n\x1b[?25h")
	shape := analyseFrame(frame, 120)
	if !shape.hidesCursor {
		t.Fatal("the cursor hide should be seen")
	}
	if !shape.startsAtHome {
		t.Fatal("a repaint goes to home before drawing")
	}
	if shape.sizeReport != [2]int{30, 120} {
		t.Fatalf("size report %v", shape.sizeReport)
	}
	if shape.fillMarkers != 2 {
		t.Fatalf("markers %d", shape.fillMarkers)
	}
}

func TestAnalyseFrameSeesACommandBatchAsNotStartingAtHome(t *testing.T) {
	// The distinction that took f4 several field runs: a batch of command
	// output positions below home, a repaint goes to home.
	batch := []byte("\x1b[?25l\x1b[12;1H" + fillMarker(9) + "\x1b[?25h")
	shape := analyseFrame(batch, 120)
	if shape.startsAtHome {
		t.Fatal("a batch positioned at row 12 must not read as starting at home")
	}
}

func TestAnalyseFrameSeesTheAlternateScreenSwitch(t *testing.T) {
	shape := analyseFrame([]byte("\x1b[?1049h drawing \x1b[?1049l"), 80)
	if !shape.sawAltEnter || !shape.sawAltLeave {
		t.Fatalf("alt screen enter/leave should both be seen: %+v", shape)
	}
}

func TestLadderVerdictNamesTheThreeCeilings(t *testing.T) {
	rungs := []rungResult{
		{Height: 1000, CreateOK: true, LinesSeen: 100, ReflowMarkers: 100, ReflowMs: 40},
		{Height: 8000, CreateOK: true, LinesSeen: 100, ReflowMarkers: 100, ReflowMs: 900},
		{Height: 16000, CreateOK: true, LinesSeen: 100, ReflowMarkers: 60, ReflowMs: 1800},
		{Height: 32000, CreateOK: false, CreateNo: "E_INVALIDARG"},
	}
	got := ladderVerdict(rungs, 250)
	for _, want := range []string{"created: 16000", "whole history: 8000", "within 250ms: 1000"} {
		if !strings.Contains(got, want) {
			t.Fatalf("verdict %q missing %q", got, want)
		}
	}
}

func TestLadderVerdictReportsTotalFailure(t *testing.T) {
	got := ladderVerdict([]rungResult{{Height: 125, CreateOK: false}}, 250)
	if !strings.Contains(got, "closed on this build") {
		t.Fatalf("verdict %q", got)
	}
}

func TestDefaultLadderIsTheAgreedRungs(t *testing.T) {
	want := []int{125, 250, 500, 1000, 2000, 4000, 8000, 16000, 32000}
	got := defaultLadder()
	if len(got) != len(want) {
		t.Fatalf("ladder %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("ladder %v", got)
		}
	}
}
