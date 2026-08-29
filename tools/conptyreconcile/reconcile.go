package main

import (
	"bytes"

	"strings"
)

// The reconciliation this file tests exists because of one measured fact
// (docs/CONPTY_RESEARCH.md §17): conhost loses the boundary between a line
// whose last row filled the console width and the line after it. Every frame,
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

// frameWidthFromFrame reads the XTWINOPS size report emitted at the head of
// the old resize frame. The width is needed only by the reconciler to apply
// the ported WriteInfos view; it is not guessed from byte lengths.
func frameWidthFromFrame(frame []byte) int {
	i := bytes.Index(frame, []byte("\x1b[8;"))
	if i < 0 {
		return 0
	}
	j := i + 2
	for j < len(frame) && (frame[j] < 0x40 || frame[j] > 0x7e) {
		j++
	}
	if j >= len(frame) || frame[j] != 't' {
		return 0
	}
	nums := msCsiNums(string(frame[i+2 : j]))
	if len(nums) < 3 {
		return 0
	}
	return nums[2]
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
				for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
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

// liveLine is one logical line as the live stream delivered it, together with
// the console width in force when it was written.
type liveLine struct {
	Text  string
	Width int // width at which the row first received output
}

// liveLines splits the stream into logical lines and tags each with the width
// that applied to it, tracking the size reports ConPTY sends.
func liveLines(stream []byte, initialWidth int) []liveLine {
	// A cursor model, not a CRLF split. The rules this replaced -- "a CRLF
	// followed by a cursor move is a continuation", and so on -- were written
	// from what the stream usually looks like, and a real capture broke them
	// immediately: a 119-column line in a 120-column console ended with no
	// newline at all, followed by an absolute ESC[30;1H, because conhost
	// repositioned rather than emitting a newline and a blank row. The grid
	//
	// On the same capture the seam rules produced 130 logical lines out of
	// 151 printed; the grid produces 151 of 151.
	width := initialWidth
	if width < 1 {
		width = 1
	}
	g := NewGrid(width)
	g.Feed(stream)

	return g.LogicalLinesWithWidth()
}

func liveLinesFromChunks(chunks [][]byte, initialWidth int) []liveLine {
	g := NewGrid(initialWidth)
	for _, chunk := range chunks {
		g.Feed(chunk)
	}
	return g.LogicalLinesWithWidth()
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
			for j < len(stream) && (stream[j] < 0x40 || stream[j] > 0x7e) {
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
	for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
		j++
	}
	return j < len(b) && (b[j] == 'H' || b[j] == 'f')
}

// ---------------------------------------------------------------------------
// the correction
// ---------------------------------------------------------------------------

// reconcileOrdered rebuilds the logical lines by walking the live sequence in
// order. The merge decision uses the write-time width and the ported row
// state; the frame width is used only to reproduce the final repaint bytes.
//
// Content matching is not enough, and the randomised test says why: a line of
// 120 '+' followed by one of 360 '+' is byte-identical to the reverse, so any
// content-only rule picks one arbitrarily and is wrong half the time. Order
// removes the ambiguity, and the emitter's rule makes the walk exact -- a
// frame run is a sequence of live lines where every one but the last filled
// its rows (and so received no terminator) and the last did not.
//
// The width each line was written at comes from the stream itself, so a resize
// while output is arriving is handled by construction. The optional frame
// width is kept optional for callers that only have the logical frame text.
func reconcileOrdered(frameRuns []string, live []liveLine, frameWidth ...int) []string {
	if len(live) == 0 {
		return frameRuns
	}
	originalLive := append([]liveLine(nil), live...)
	width := 0
	if len(frameWidth) > 0 {
		width = frameWidth[0]
	}
	if frameContainsAllLiveText(frameRuns, originalLive) &&
		!frameFollowsLiveOrder(frameRuns, originalLive) {
		out := make([]string, 0, len(originalLive))
		for _, line := range originalLive {
			out = append(out, line.Text)
		}
		return out
	}
	// The buffer is a ring: the frame may begin partway through what was
	// printed. Find plausible starts from the first frame run before doing the
	// full walk; otherwise every failed suffix would re-render the whole frame.
	starts := frameStartCandidates(frameRuns, live, width)
	for _, start := range starts {
		if out, ok := alignFromStrict(frameRuns, live[start:], width); ok {
			return out
		}
	}
	// The extended walk is only needed for the source-backed wide-cell frame
	// cases. Keeping it out of the normal path matters for tall captures: a
	// failed candidate otherwise re-renders every suffix of every run.
	if liveHasWideText(originalLive) {
		for _, start := range starts {
			if out, ok := alignFromExtended(frameRuns, live[start:], width); ok {
				return out
			}
		}
	}
	// The aligners walk run boundaries, and run boundaries are the weakest
	// part of the frame: they depend on where the renderer put its erases
	// and breaks. If the frame turns out to carry exactly the live content
	// and nothing more, the boundaries do not matter at all.
	// One guard on this fallback, and it is the guard of a failure that
	// shipped (TestFrameTakenAtADifferentWidthThanTheLinesWereWritten): a
	// live sequence keyed on the FRAME width must never be rescued. Its
	// texts can even come out right -- the ported terminal joins the
	// spurious wraps back -- but its Width annotations are fiction, and
	// everything downstream that consumes them would key on fiction. Right
	// keying is visible in the annotations themselves: after a resize the
	// live lines carry the write width, not the frame width. When the two
	// widths are equal there was no resize, frames split cleanly, and the
	// aligners above succeed without any fallback.
	liveKeyedOnFrameWidth := width > 0
	for _, l := range originalLive {
		if l.Width != width {
			liveKeyedOnFrameWidth = false
			break
		}
	}
	if !liveKeyedOnFrameWidth && frameAddsNothingBeyondLive(frameRuns, originalLive) {
		out := make([]string, 0, len(originalLive))
		for _, line := range originalLive {
			out = append(out, line.Text)
		}
		return out
	}

	// A resize can interleave freshly written output into the middle of the
	// repaint. In that case the frame and the live stream contain the same
	// child text in different orders, so no single ordered walk can succeed.
	// Returning the live sequence is safe only when every non-empty live line
	// is also present in the frame with the required multiplicity; otherwise a
	// ring/partial-frame case must retain the uncorrected frame.
	if frameContainsAllLiveText(frameRuns, originalLive) &&
		!frameFollowsLiveOrder(frameRuns, originalLive) {
		out := make([]string, 0, len(originalLive))
		for _, line := range originalLive {
			out = append(out, line.Text)
		}
		return out
	}
	return frameRuns
}

// mergeChainEnd walks live lines from start and returns the end of the group
// the BUFFER holds joined: conhost's legacy write path left wrapForced on
// the line's last row (fillsRowsExactly, ported WriteCharsLegacy). This is
// deliberately only the buffer-level effect. The field dump of seed
// 1788002866976838800 shows a second, renderer-level effect -- a repaint can
// glue runs across buffer boundaries and break them inside joins -- and that
// one is NOT modelled here, because its boundaries proved erratic; when it
// strikes, the aligners fail and frameAddsNothingBeyondLive decides on
// content instead of boundaries.
func mergeChainEnd(live []liveLine, start, frameWidth int) int {
	end := start
	for end < len(live) {
		end++
		if !mergesAtWidth(live[end-1].Text, live[end-1].Width) {
			break
		}
	}
	return end
}

// frameAddsNothingBeyondLive: every frame run is a concatenation of whole,
// consecutive live lines, and together the runs consume the live sequence
// exactly. Texts are compared with spaces removed, because the frame's
// rendering inserts spaces the logical text does not have (the padding
// column of a wide glyph at a row edge renders as a space) and trailing
// blanks are trimmed unevenly. Two things are established at once:
//   - content: the frame carries the live text, in order, nothing more;
//   - structure: every run boundary falls on a live line boundary, so the
//     live lines are whole units of the frame, not a different reading of
//     the same bytes. A live sequence parsed at the wrong width fails here
//     precisely because its boundaries land inside runs
//     (TestFrameTakenAtADifferentWidthThanTheLinesWereWritten).
//
// When both hold, the frame cannot correct anything: its extra information
// is only run boundaries, and those depend on renderer details (which rows
// got an ESC[K, where a repaint broke) that the field dump of seed
// 1788002866976838800 shows are richer than any split rule this tool has.
// The frame earns its place only when it holds content the live stream
// never saw, and then this check fails and the aligners above decide.
func frameAddsNothingBeyondLive(frameRuns []string, live []liveLine) bool {
	strip := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if r != ' ' {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	i := 0
	skipEmpty := func() {
		for i < len(live) && strip(live[i].Text) == "" {
			i++
		}
	}
	any := false
	for runIdx, run := range frameRuns {
		rest := strip(run)
		for rest != "" {
			skipEmpty()
			if i >= len(live) {
				return false
			}
			text := strip(live[i].Text)
			if !strings.HasPrefix(rest, text) {
				return false
			}
			rest = rest[len(text):]
			i++
			any = true
		}
		_ = runIdx
	}
	skipEmpty()
	return any && i == len(live)
}

func frameStartCandidates(frameRuns []string, live []liveLine, width int) []int {
	if len(frameRuns) == 0 {
		return nil
	}
	starts := make([]int, 0, len(live))
	for start := 0; start < len(live); start++ {
		end := mergeChainEnd(live, start, width)
		if end == start {
			continue
		}
		frameText := plainLiveText(live[start:end])
		if width > 0 {
			frameText = msFrameRunText(live[start:end], width)
		}
		if frameText == frameRuns[0] {
			starts = append(starts, start)
		}
	}
	return starts
}

func frameContainsAllLiveText(frameRuns []string, live []liveLine) bool {
	var frame strings.Builder
	for _, run := range frameRuns {
		frame.WriteString(run)
	}
	remaining := frame.String()
	allExact := true
	for _, line := range live {
		if line.Text == "" {
			continue
		}
		i := strings.Index(remaining, line.Text)
		if i < 0 {
			allExact = false
			break
		}
		remaining = remaining[:i] + remaining[i+len(line.Text):]
	}
	if allExact {
		return true
	}

	// A frame can contain the same wide glyphs with the documented display
	// padding inserted between their UTF-8 sequences. For this evidence path,
	// remove only literal spaces from both sides before checking multiplicity;
	// the returned value is still the untouched live text, never this normalized
	// representation.
	remaining = strings.ReplaceAll(frame.String(), " ", "")
	for _, line := range live {
		needle := strings.ReplaceAll(line.Text, " ", "")
		if needle == "" {
			continue
		}
		i := strings.Index(remaining, needle)
		if i < 0 {
			return false
		}
		remaining = remaining[:i] + remaining[i+len(needle):]
	}
	return true
}

func frameFollowsLiveOrder(frameRuns []string, live []liveLine) bool {
	var frame strings.Builder
	for _, run := range frameRuns {
		frame.WriteString(run)
	}
	frameText := strings.ReplaceAll(frame.String(), " ", "")
	position := 0
	for _, line := range live {
		needle := strings.ReplaceAll(line.Text, " ", "")
		if needle == "" {
			continue
		}
		i := strings.Index(frameText[position:], needle)
		if i < 0 {
			return false
		}
		position += i + len(needle)
	}
	return true
}

func alignFromStrict(frameRuns []string, live []liveLine, frameWidth ...int) ([]string, bool) {
	out := make([]string, 0, len(live))
	width := 0
	if len(frameWidth) > 0 {
		width = frameWidth[0]
	}
	i := 0
	for _, run := range frameRuns {
		start := i
		i = mergeChainEnd(live, start, width)
		if start == i {
			return nil, false
		}
		frameText := plainLiveText(live[start:i])
		if width > 0 {
			frameText = msFrameRunText(live[start:i], width)
		}
		if frameText != run {
			return nil, false
		}
		for _, l := range live[start:i] {
			out = append(out, l.Text)
		}
	}
	return out, true
}

func alignFromExtended(frameRuns []string, live []liveLine, frameWidth ...int) ([]string, bool) {
	out := make([]string, 0, len(live))
	width := 0
	if len(frameWidth) > 0 {
		width = frameWidth[0]
	}
	i := 0
	for _, run := range frameRuns {
		start := i
		baseEnd := mergeChainEnd(live, start, width)
		if start == baseEnd {
			return nil, false
		}

		// Most frames end a run at baseEnd. The old 10.0.22000 emitter can
		// omit an intermediate terminator after a resize, though, so extend
		// the candidate only as far as the frame text proves. This keeps the
		// source order and avoids treating a frame grammar guess as a rule.
		end := baseEnd
		for {
			var frameText string
			if width > 0 {
				frameText = msFrameRunText(live[start:end], width)
			} else {
				frameText = plainLiveText(live[start:end])
			}
			if frameText == run {
				break
			}
			if width == 0 || end >= len(live) {
				return nil, false
			}
			end++
		}
		for _, l := range live[start:end] {
			out = append(out, l.Text)
		}
		i = end
	}
	return out, true
}

func plainLiveText(lines []liveLine) string {
	var plain strings.Builder
	for _, l := range lines {
		plain.WriteString(l.Text)
	}
	return plain.String()
}

func liveHasWideText(lines []liveLine) bool {
	for _, line := range lines {
		for _, r := range line.Text {
			if cellWidth(r) == 2 {
				return true
			}
		}
	}
	return false
}

func mergesAtWidth(line string, width int) bool {
	// Cells, not bytes: a Cyrillic line is twice as many bytes as columns, so
	// a byte-based test fires at the wrong places and misses the real ones.
	return fillsRowsExactly(line, width)
}
