package main

import (
	"strings"
	"testing"
)

// Fuzzing, because every failure this tool has had in the field was an input
// nobody thought to write a case for: a frame that never arrived, a stream cut
// mid-escape, a width that made a line an exact multiple by accident. The
// properties below are the ones that must hold whatever arrives on the pipe.

// FuzzAnalyseStreamsNeverPanics is the blunt one. Whatever bytes come back
// from a pseudoconsole -- truncated, interleaved, or not VT at all -- the tool
// must produce a report and return. Crashing loses the capture and costs a
// trip to the machine under test.
func FuzzAnalyseStreamsNeverPanics(f *testing.F) {
	m := newMockConPTY(120, 200, 1)
	truth := groundTruthLines(120, 20)
	f.Add(m.LiveStream(truth), m.Frame(truth), 120, 20)
	f.Add([]byte{}, []byte{}, 1, 0)
	f.Add([]byte("\x1b["), []byte("\x1b[8;"), 120, 5)
	f.Add([]byte("\x1b[?25l\x1b[H"), []byte("plain"), 2, 1)
	f.Add([]byte(strings.Repeat("\x1b[K\r\n", 50)), []byte(strings.Repeat("x", 500)), 7, 3)

	f.Fuzz(func(t *testing.T, live, frame []byte, width, lines int) {
		// The tool is only ever called with a sane width and line count; the
		// fuzzer is free to supply anything, so clamp the way the flags do.
		if width < 1 || width > 4096 {
			width = 120
		}
		if lines < 0 || lines > 500 {
			lines = 20
		}
		out := analyseStreams(live, frame, width, lines, width)
		if len(out) == 0 {
			t.Fatal("the report must never be empty")
		}
	})
}

// FuzzReconcileIsSafeAndConservative pins the properties the correction must
// have on any input: it never loses or invents characters, and it only ever
// splits a run -- never joins.
func FuzzReconcileIsSafeAndConservative(f *testing.F) {
	f.Add("abcdefghij", "abcde", 5)
	f.Add(strings.Repeat("=", 240)+"tail", strings.Repeat("=", 120), 120)
	f.Add("", "", 1)

	f.Fuzz(func(t *testing.T, run, recorded string, width int) {
		if width < 1 || width > 1024 {
			width = 10
		}
		live := []liveLine{}
		if recorded != "" {
			live = append(live, liveLine{Text: recorded, Width: width})
		}
		live = append(live, liveLine{Text: run, Width: width})

		got := reconcileOrdered([]string{run}, live)

		if strings.Join(got, "") != run {
			t.Fatalf("content changed:\n in  %q\n out %q", run, got)
		}
		if len(got) < 1 {
			t.Fatalf("produced no lines from %q", run)
		}
	})
}

// FuzzLiveLogicalLinesPreservesText checks the stream splitter: whatever the
// escape soup, the concatenation of the logical lines it returns must equal
// the printable text of the input, so a scroll seam can never eat a character.
func FuzzLiveLogicalLinesPreservesText(f *testing.F) {
	f.Add([]byte("one\r\ntwo\r\n"))
	f.Add([]byte("long\r\n\x1b[499;120Hcontinued\r\n"))
	f.Add([]byte("\x1b[?25l\x1b[8;20;20t\x1b[Hx\x1b[K\r\n"))

	f.Fuzz(func(t *testing.T, stream []byte) {
		lines := liveLogicalLines(stream)

		var want strings.Builder
		i := 0
		for i < len(stream) {
			c := stream[i]
			if c == 0x1b && i+1 < len(stream) && (stream[i+1] == '[' || stream[i+1] == ']') {
				if stream[i+1] == '[' {
					j := i + 2
					for j < len(stream) && (stream[j] < 0x40 || stream[j] > 0x7e) {
						j++
					}
					i = j + 1
					continue
				}
				// OSC ends at BEL or at ST (ESC backslash). An oracle that
				// knew only BEL disagreed with the code on "\x1b]\x1b\\0" --
				// found by the fuzzer, and the oracle was the one at fault.
				j := i + 2
				for j < len(stream) {
					if stream[j] == 0x07 {
						j++
						break
					}
					if stream[j] == 0x1b && j+1 < len(stream) && stream[j+1] == '\\' {
						j += 2
						break
					}
					j++
				}
				i = j
				continue
			}
			if c == 0x1b {
				i += 2
				continue
			}
			if c >= 0x20 && c != 0x7f {
				want.WriteByte(c)
			}
			i++
		}

		if got := strings.Join(lines, ""); got != want.String() {
			t.Fatalf("text changed by line splitting:\n got  %q\n want %q",
				trunc(got), trunc(want.String()))
		}
	})
}
