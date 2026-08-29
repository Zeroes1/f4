package main

// Replaying a field dump byte for byte, without Windows. The dump format is
// the probe's own (main_windows.go escape()): chunks under "@@ ... CHUNK"
// headers, ESC as \e, CR as \r, LF as \n followed by a real newline for
// readability, control bytes as \xNN, backslash doubled. Real newlines in the
// file are formatting, not data.

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

func unescapeDump(t *testing.T, path string) [][]byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("no dump: %v", err)
	}
	var chunks [][]byte
	var cur []byte
	inChunk := false
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "@@") {
			if strings.Contains(line, "CHUNK") {
				if inChunk {
					chunks = append(chunks, cur)
				}
				cur = nil
				inChunk = true
			} else if inChunk {
				chunks = append(chunks, cur)
				cur = nil
				inChunk = false
			}
			continue
		}
		if !inChunk {
			continue
		}
		i := 0
		for i < len(line) {
			c := line[i]
			if c != '\\' {
				cur = append(cur, c)
				i++
				continue
			}
			if i+1 >= len(line) {
				break
			}
			switch line[i+1] {
			case 'e':
				cur = append(cur, 0x1b)
				i += 2
			case 'r':
				cur = append(cur, '\r')
				i += 2
			case 'n':
				cur = append(cur, '\n')
				i += 2
			case '\\':
				cur = append(cur, '\\')
				i += 2
			case 'x':
				n, _ := strconv.ParseUint(line[i+2:i+4], 16, 8)
				cur = append(cur, byte(n))
				i += 4
			default:
				cur = append(cur, c)
				i++
			}
		}
	}
	if inChunk {
		chunks = append(chunks, cur)
	}
	return chunks
}

// TestFieldDump1788002866976838800 replays the real conhost bytes of the
// field run that failed stage 1 (64 of 151). Live is every chunk before the
// resize, the frame is the repaint after it (the probe's step delay makes the
// boundary unambiguous: the frame chunk arrives ~2s after the live ones).
func TestFieldDump1788002866976838800(t *testing.T) {
	replayFieldDump(t, 1788002866976838800)
}

// TestFieldDumpsAll replays every field dump captured in this project. They
// are real conhost bytes; each is worth more than any number of mock rounds,
// because the mock shares this tool's model and can only disagree with it by
// accident, while conhost disagrees on purpose.
func TestFieldDumpsAll(t *testing.T) {
	// Only dumps whose capture is intact are listed. Four of the five field
	// dumps taken before the double-handle fix in main_windows.go begin with
	// another handle's "END total 0 bytes" footer written over their first
	// bytes, so their opening chunk is lost; replaying them measures the
	// corruption, not the pipeline. They are kept in testdata as evidence of
	// the bug, and this list grows again with the next capture.
	for _, seed := range []int64{
		1788002866976838800, // failed stage 1 on exact-width chains
	} {
		seed := seed
		t.Run(fmt.Sprint(seed), func(t *testing.T) { replayFieldDump(t, seed) })
	}
}

func replayFieldDump(t *testing.T, seed int64) {
	t.Helper()
	const width, lines = 120, 150
	chunks := unescapeDump(t, fmt.Sprintf("testdata/conptydump-%d.txt", seed))
	if len(chunks) == 0 {
		t.Skip("empty dump")
	}
	var all []byte
	for _, c := range chunks {
		all = append(all, c...)
	}
	// The live/frame boundary is not a chunk boundary: one ReadFile can carry
	// the tail of the live output and the head of the repaint together, and
	// the first version of this harness split on chunks and mis-replayed four
	// of five dumps that had passed on Windows. The frame announces itself --
	// conhost emits the XTWINOPS size report at the head of the repaint after
	// the resize, which is the same marker frameWidthFromFrame reads.
	i := bytes.Index(all, []byte("\x1b[8;"))
	if i < 0 {
		t.Skip("no size report in dump: cannot locate the frame")
	}
	live, frame := all[:i], all[i:]

	printed := trimTrailingBlanks(append(randomGroundTruth(seed, width, lines), markerDone))
	recovered := trimTrailingBlanks(reconcileOrdered(
		trimTrailingBlanks(splitFrameLines(frame)), liveLines(live, width), width-1))
	if len(recovered) != len(printed) {
		t.Fatalf("recovered %d lines, expected %d", len(recovered), len(printed))
	}
	if n := diffCount(recovered, printed); n != 0 {
		t.Fatalf("%d lines differ", n)
	}
}

// History of this test: it was added RED (64 of 151) and went green in the
// same session, without a single Windows run, in three steps that are worth
// keeping on the record. (1) The live side was verified perfect: 151 of 151
// lines through the ported VT terminal, CUP jumps and all. (2) The buffer
// merge rule was ported: fillsRowsExactly now writes through WriteCharsLegacy
// (msstream.go), whose LF clears wrapForced on the row the cursor already
// wrapped onto -- the mechanism behind P13/§17 exact-width merges, visible in
// the source. (3) The remaining failures were the frame's run boundaries,
// which depend on renderer details richer than any split rule here (this
// dump has 1849 ESC[K over ~1850 rows yet only 64 splittable runs); the fix
// was not another boundary rule but frameAddsNothingBeyondLive in
// reconcile.go: when the frame carries exactly the live content, the live
// reading wins and boundaries stop mattering.
