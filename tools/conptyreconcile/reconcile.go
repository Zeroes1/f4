package main

import (
	"bytes"

	"strings"
)

// The reconciliation this file tests exists because of one measured fact
// (docs/CONPTY_RESEARCH.md §17): conhost loses the boundary between a line
// whose length equalled the console width and the line after it. Every frame,
// at every width, delivers those two logical lines merged. The live stream
// does not: there the exact-width line is terminated by a plain CRLF.
//
// So the frame is authoritative for long lines (always whole) and wrong for
// exact-width ones; the live stream is the reverse. The correction is to
// record, from the live stream, which full rows were followed by a hard break,
// and use that to un-merge what a frame joined.
//
// The obvious objection -- that ten identical `-----` rows cannot be told
// apart -- does not apply, because the decision is made per *content*, not per
// position: if a row of that exact text ended a line once, splitting after any
// identical row is the same decision. What has to be measured is the opposite
// error: a genuinely wrapped line whose first W characters happen to equal
// some row that did end a line. This file's job is to count those against
// ground truth rather than to argue about them.

// ---------------------------------------------------------------------------
// frame parsing
// ---------------------------------------------------------------------------

// splitFrameLines returns the logical lines a repaint frame carries. A logical
// line ends at ESC[K CR LF; anything between two terminators is one logical
// line however long, which is the §13 reading.
func splitFrameLines(frame []byte) []string {
	body := frame
	// Drop the frame head: cursor hide, optional size report, home.
	if i := bytes.Index(body, []byte("\x1b[H")); i >= 0 && i < 64 {
		body = body[i+3:]
	}
	term := []byte("\x1b[K\r\n")
	parts := bytes.Split(body, term)
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimRight(stripEscapes(p), " "))
	}
	// The tail after the last terminator is the cursor reposition and the
	// cursor show, not a line.
	if n := len(out); n > 0 && strings.TrimSpace(out[n-1]) == "" {
		out = out[:n-1]
	}
	return out
}

// stripEscapes removes CSI sequences and control bytes, leaving text. It is
// deliberately crude: the reconciliation only compares text, and a sequence
// left in place would break a comparison that is otherwise exact.
func stripEscapes(b []byte) string {
	var sb strings.Builder
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == 0x1b {
			// Skip ESC [ ... final-byte, or ESC ] ... BEL/ST, or ESC X.
			if i+1 < len(b) && b[i+1] == '[' {
				j := i + 2
				for j < len(b) && !(b[j] >= 0x40 && b[j] <= 0x7e) {
					j++
				}
				i = j
				continue
			}
			if i+1 < len(b) && b[i+1] == ']' {
				i = skipOSC(b, i) - 1
				continue
			}
			i++
			continue
		}
		if c == '\r' || c == '\n' {
			continue
		}
		if c >= 0x20 && c != 0x7f {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// live-stream hard breaks
// ---------------------------------------------------------------------------

// liveHardBreaks returns every logical line the live stream terminated with a
// CRLF whose length is a multiple of the width -- precisely the lines a frame
// will have merged into their successor. On a build where a wrapped line arrives whole (the
// 22000 shape, P11/P12), such a row can only be a hard break -- which is
// exactly the boundary a frame will have lost.
//
// On 19045 (P6) the wrap point carries a CRLF too and this signal is not
// available; that is why the correction is a setting rather than a constant.
func liveHardBreaks(stream []byte, width int) map[string]int {
	return liveHardBreaksTracking(stream, width)
}

// liveHardBreaksTracking is the form the tool actually needs. The width can
// change while output is in flight -- a window drag during a `dir` is the
// ordinary case -- and whether a line merges is decided by the width in force
// when *that line* was written. Fixing on one width gets every line printed
// after a resize wrong.
//
// The width in force is read from the stream itself: ConPTY announces it with
// the XTWINOPS size report, ESC[8;rows;cols t, at the head of every frame.
func liveHardBreaksTracking(stream []byte, initialWidth int) map[string]int {
	out := map[string]int{}
	if initialWidth <= 0 {
		return out
	}
	width := initialWidth
	for _, seg := range liveSegments(stream) {
		if seg.width > 0 {
			width = seg.width
		}
		if len(seg.line) > 0 && len(seg.line)%width == 0 {
			out[seg.line]++
		}
	}
	return out
}

// liveLine is one logical line as the live stream delivered it, together with
// the console width in force when it was written.
type liveLine struct {
	Text  string
	Width int
}

// liveLines splits the stream into logical lines and tags each with the width
// that applied to it, tracking the size reports ConPTY sends.
func liveLines(stream []byte, initialWidth int) []liveLine {
	out := make([]liveLine, 0, 64)
	width := initialWidth
	for _, seg := range liveSegments(stream) {
		if seg.width > 0 {
			width = seg.width
		}
		out = append(out, liveLine{Text: seg.line, Width: width})
	}
	return out
}

type liveSegment struct {
	line  string
	width int // non-zero when a size report preceded this line
}

func liveHardBreaksFixed(stream []byte, width int) map[string]int {
	out := map[string]int{}
	if width <= 0 {
		return out
	}
	for _, line := range liveLogicalLines(stream) {
		// A logical line merges into its successor exactly when it fills its
		// rows completely -- when its length is a non-zero multiple of the
		// width. Two unit tests shaped this: the first version recorded only
		// one-row lines and missed a line of 2W; the second recorded just the
		// final row and then split a 2W line of identical characters in half,
		// because its first row and its last row are the same text. The whole
		// line is recorded, and matching is by whole line.
		if len(line) > 0 && len(line)%width == 0 {
			out[line]++
		}
	}
	return out
}

// liveLogicalLines splits the live stream into logical lines, rejoining the
// pieces of one that conhost split while the buffer was scrolling.
//
// The distinguishing signal is measured, not guessed. In an overflowing
// session the stream reads:
//
//	~L1~xxx...xxx CR LF  ESC[499;120H  xxx...xxx CR LF  ESC[499;120H  xxx~E1~ CR LF
//
// A CRLF that is followed by an absolute cursor position is a scroll seam, not
// a line ending: conhost is repositioning to continue the same line. A CRLF
// with ordinary text after it is a real boundary. Reading it any other way
// produces exactly the failure the end-to-end test caught -- a fragment of a
// wrapped line recorded as a whole line, which then cuts genuine wraps apart.
func liveLogicalLines(stream []byte) []string {
	var out []string
	var cur []byte

	i := 0
	for i < len(stream) {
		// A CSI sequence: consume it, and remember whether it repositioned.
		if stream[i] == 0x1b && i+1 < len(stream) && stream[i+1] == '[' {
			j := i + 2
			for j < len(stream) && !(stream[j] >= 0x40 && stream[j] <= 0x7e) {
				j++
			}
			i = j + 1
			continue
		}
		if stream[i] == 0x1b && i+1 < len(stream) && stream[i+1] == ']' {
			i = skipOSC(stream, i)
			continue
		}
		if stream[i] == 0x1b {
			// A bare escape is two bytes (ESC 7, ESC 8, ESC \\ and friends).
			// Skipping only one left the following byte to be read as text,
			// which the fuzzer found within seconds on the input "\x1b0".
			i += 2
			continue
		}
		if stream[i] == '\r' {
			i++
			continue
		}
		if stream[i] == '\n' {
			if seamFollows(stream, i+1) {
				// Same logical line, continued after a reposition.
				i++
				continue
			}
			out = append(out, string(cur))
			cur = cur[:0]
			i++
			continue
		}
		if stream[i] >= 0x20 && stream[i] != 0x7f {
			cur = append(cur, stream[i])
		}
		i++
	}
	if len(cur) > 0 {
		out = append(out, string(cur))
	}
	return out
}

// liveSegments is liveLogicalLines with the width announcements kept. A
// segment carries the logical line and, when a size report was seen just
// before it, the width that report declared.
func liveSegments(stream []byte) []liveSegment {
	var out []liveSegment
	var cur []byte
	pending := 0

	flush := func() {
		out = append(out, liveSegment{line: string(cur), width: pending})
		cur = cur[:0]
		pending = 0
	}

	i := 0
	for i < len(stream) {
		if stream[i] == 0x1b && i+1 < len(stream) && stream[i+1] == '[' {
			j := i + 2
			for j < len(stream) && !(stream[j] >= 0x40 && stream[j] <= 0x7e) {
				j++
			}
			if j < len(stream) && stream[j] == 't' {
				if w := sizeReportWidth(stream[i : j+1]); w > 0 {
					pending = w
				}
			}
			i = j + 1
			continue
		}
		if stream[i] == 0x1b && i+1 < len(stream) && stream[i+1] == ']' {
			i = skipOSC(stream, i)
			continue
		}
		if stream[i] == 0x1b {
			i += 2
			continue
		}
		if stream[i] == '\r' {
			i++
			continue
		}
		if stream[i] == '\n' {
			if seamFollows(stream, i+1) {
				i++
				continue
			}
			flush()
			i++
			continue
		}
		if stream[i] >= 0x20 && stream[i] != 0x7f {
			cur = append(cur, stream[i])
		}
		i++
	}
	if len(cur) > 0 || pending > 0 {
		flush()
	}
	return out
}

// skipOSC steps over an OSC sequence, which ends at BEL or at ST (ESC \\).
// A version that recognised only BEL let the window title conhost sends --
// ESC ] 0 ; <path> BEL -- leak into the first logical line of a real capture,
// which made the whole alignment fail. Found by replaying a field dump, not by
// reading the code.
func skipOSC(b []byte, i int) int {
	j := i + 2
	for j < len(b) {
		if b[j] == 0x07 {
			return j + 1
		}
		if b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\' {
			return j + 2
		}
		j++
	}
	return j
}

// sizeReportWidth reads the columns out of ESC[8;rows;cols t.
func sizeReportWidth(seq []byte) int {
	body := string(seq)
	if len(body) < 4 || body[len(body)-1] != 't' {
		return 0
	}
	body = body[2 : len(body)-1]
	parts := strings.Split(body, ";")
	if len(parts) != 3 || parts[0] != "8" {
		return 0
	}
	w := 0
	for _, c := range parts[2] {
		if c < '0' || c > '9' {
			return 0
		}
		w = w*10 + int(c-'0')
	}
	return w
}

// seamFollows reports whether the bytes at off begin an absolute cursor
// position, which marks a continuation rather than a new line.
func seamFollows(b []byte, off int) bool {
	if off >= len(b) || b[off] != 0x1b {
		return false
	}
	if off+1 >= len(b) || b[off+1] != '[' {
		return false
	}
	j := off + 2
	for j < len(b) && !(b[j] >= 0x40 && b[j] <= 0x7e) {
		j++
	}
	return j < len(b) && (b[j] == 'H' || b[j] == 'f')
}

// stripEscapesKeepBreaks removes CSI/OSC sequences but keeps CR and LF, so
// that line structure survives.
func stripEscapesKeepBreaks(b []byte) string {
	var sb strings.Builder
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c == 0x1b {
			if i+1 < len(b) && b[i+1] == '[' {
				j := i + 2
				for j < len(b) && !(b[j] >= 0x40 && b[j] <= 0x7e) {
					j++
				}
				i = j
				continue
			}
			if i+1 < len(b) && b[i+1] == ']' {
				i = skipOSC(b, i) - 1
				continue
			}
			i++
			continue
		}
		if c == '\r' || c == '\n' || (c >= 0x20 && c != 0x7f) {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// ---------------------------------------------------------------------------
// the correction
// ---------------------------------------------------------------------------

// reconcile splits frame lines that conhost merged. A merge can only have
// happened at an exact multiple of the width, so those are the only candidate
// points, and a candidate is taken only when the live stream recorded a hard
// break after a row of exactly that text.
// reconcileOrdered rebuilds the logical lines by walking the live sequence in
// order, instead of matching frame runs against recorded text.
//
// Content matching is not enough, and the randomised test says why: a line of
// 120 '+' followed by one of 360 '+' is byte-identical to the reverse, so any
// content-only rule picks one arbitrarily and is wrong half the time. Order
// removes the ambiguity, and the emitter's rule makes the walk exact -- a
// frame run is a sequence of live lines where every one but the last filled
// its rows (and so received no terminator) and the last did not.
//
// The width each line was written at comes from the stream itself, so a resize
// while output is arriving is handled by construction.
func reconcileOrdered(frameRuns []string, live []liveLine) []string {
	if len(live) == 0 {
		return frameRuns
	}
	// The buffer's blank rows are not in the live stream, but the last
	// printed line can merge into the first of them, so the run needs one
	// blank to terminate against. Without it the alignment fails on its very
	// last run whenever output ends on a line that fills its rows.
	live = append(append([]liveLine(nil), live...),
		liveLine{Text: "", Width: live[len(live)-1].Width})

	// The buffer is a ring: the frame may begin partway through what was
	// printed. Find the first live line the frame starts at.
	for start := 0; start < len(live); start++ {
		if out, ok := alignFrom(frameRuns, live[start:]); ok {
			return out
		}
	}
	return frameRuns
}

func alignFrom(frameRuns []string, live []liveLine) ([]string, bool) {
	out := make([]string, 0, len(live))
	i := 0
	for _, run := range frameRuns {
		acc := ""
		consumed := 0
		for {
			if i >= len(live) {
				return nil, false
			}
			l := live[i]
			if len(acc)+len(l.Text) > len(run) || run[len(acc):len(acc)+len(l.Text)] != l.Text {
				return nil, false
			}
			acc += l.Text
			out = append(out, l.Text)
			i++
			consumed++
			// A line that exactly fills its rows gets no terminator and the
			// run continues into the next line; anything else ends the run.
			if !mergesAtWidth(l.Text, l.Width) {
				break
			}
		}
		if acc != run {
			return nil, false
		}
		_ = consumed
	}
	return out, true
}

func mergesAtWidth(line string, width int) bool {
	return width > 0 && len(line) > 0 && len(line)%width == 0
}
