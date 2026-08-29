package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"strings"
	"unicode/utf16"
)

// A mock of the ConPTY emission grammar, so the reconciliation can be
// exercised without Windows.
//
// The stream wrapper is a documented reconstruction, not a second conhost
// implementation. Its buffer state comes from the ported MS TextBuffer and
// AdaptDispatch below; only the byte grammar around that state is reconstructed
// from §13/§17 and the pre-#17510 XtermEngine source:
//
//   - a frame opens with ESC[?25l, the XTWINOPS size report, and ESC[H
//   - a logical line is emitted whole; the receiver's autowrap places it
//   - a logical line is terminated by ESC[K CR LF
//   - EXCEPT when the port-backed row model says it exactly fills its rows, in
//     which case no terminator is emitted and it merges with the line that
//     follows -- the P13 failure, measured in §17
//   - the frame closes with ESC[<row>;1H and ESC[?25h
//
// The live stream is modelled separately, because §17 measured the two to
// differ in exactly the way that makes the correction possible: the live
// stream terminates an exact-width line with a plain CRLF.
//
// Jitter is built in because the real thing is not tidy: bytes arrive in
// arbitrarily sized chunks at arbitrary moments, a frame can be split across
// reads, and any parser that only works on whole frames would pass here and
// fail in the field.

type mockConPTY struct {
	Width  int
	Height int
	rnd    *rand.Rand
}

func newMockConPTY(width, height int, seed int64) *mockConPTY {
	return &mockConPTY{Width: width, Height: height, rnd: rand.New(rand.NewSource(seed))}
}

// rowsFor is how many buffer rows a logical line occupies.
func rowsFor(line string, width int) int { return cellRows(line, width) }

// FrameAtWidth renders the repaint produced by a resize to a *different*
// width than the lines were written at. This is the ordinary case in the
// field -- a frame is obtained by changing the width -- and it is where the
// merge boundary stops being a multiple of the frame width and stays a
// multiple of the write width. A mock that could not express this let a real
// bug through: the correction searched for multiples of the frame width,
// found none, and silently did nothing.
func (m *mockConPTY) FrameAtWidth(lines []string, writeWidth int) []byte {
	sources := m.frameSourcesAtWidth(lines, writeWidth)

	var sb strings.Builder
	sb.WriteString("\x1b[?25l\x1b[8;")
	sb.WriteString(itoa(m.Height))
	sb.WriteString(";")
	sb.WriteString(itoa(m.Width))
	sb.WriteString("t\x1b[H")

	appendFrameRuns(&sb, sources, m.Width)
	used := 0
	for _, l := range sources {
		used += rowsFor(l.Text, m.Width)
	}
	for ; used < m.Height-1; used++ {
		sb.WriteString("\x1b[K\r\n")
	}
	sb.WriteString("\x1b[K")
	sb.WriteString("\x1b[")
	sb.WriteString(itoa(min(m.Height, contentRows(liveTexts(sources), m.Width)+1)))
	sb.WriteString(";1H\x1b[?25h")
	return []byte(sb.String())
}

// FrameAtWidths renders a frame over lines whose first `split` entries were
// written at widthA and the rest at widthB -- the shape produced when the
// window is resized while output is still arriving.
func (m *mockConPTY) FrameAtWidths(lines []string, split, widthA, widthB int) []byte {
	input := make([]liveLine, len(lines))
	for i, line := range lines {
		width := widthB
		if i < split {
			width = widthA
		}
		input[i] = liveLine{Text: line, Width: width}
	}
	sources := m.frameSourcesAtWidths(input)

	var sb strings.Builder
	sb.WriteString("\x1b[?25l\x1b[8;")
	sb.WriteString(itoa(m.Height))
	sb.WriteString(";")
	sb.WriteString(itoa(m.Width))
	sb.WriteString("t\x1b[H")

	appendFrameRuns(&sb, sources, m.Width)
	used := 0
	for _, l := range sources {
		used += rowsFor(l.Text, m.Width)
	}
	for ; used < m.Height-1; used++ {
		sb.WriteString("\x1b[K\r\n")
	}
	sb.WriteString("\x1b[K\x1b[")
	sb.WriteString(itoa(min(m.Height, contentRows(liveTexts(sources), m.Width)+1)))
	sb.WriteString(";1H\x1b[?25h")
	return []byte(sb.String())
}

// Frame renders the repaint a resize produces, over the given logical lines.
// Lines beyond the buffer height are dropped from the top, which is the ring
// behaviour §17 measured (history depth equals buffer height).
func (m *mockConPTY) Frame(lines []string) []byte {
	sources := m.frameSourcesAtWidth(lines, m.Width)

	var sb strings.Builder
	sb.WriteString("\x1b[?25l")
	sb.WriteString("\x1b[8;")
	sb.WriteString(itoa(m.Height))
	sb.WriteString(";")
	sb.WriteString(itoa(m.Width))
	sb.WriteString("t")
	sb.WriteString("\x1b[H")

	appendFrameRuns(&sb, sources, m.Width)
	used := 0
	for _, l := range sources {
		used += rowsFor(l.Text, m.Width)
	}
	// The rest of the buffer is blank rows, each costing exactly ESC[K CR LF.
	for ; used < m.Height-1; used++ {
		sb.WriteString("\x1b[K\r\n")
	}
	sb.WriteString("\x1b[K")

	sb.WriteString("\x1b[")
	sb.WriteString(itoa(min(m.Height, contentRows(liveTexts(sources), m.Width)+1)))
	sb.WriteString(";1H")
	sb.WriteString("\x1b[?25h")
	return []byte(sb.String())
}

// appendFrameRuns writes the measured frame envelope around rows generated by
// the ported TextBuffer::Reflow. A frame run is one sequence of source lines
// whose last row was forced at write time; conhost loses those source-line
// boundaries and the old frame carries it as one ESC[K CRLF-delimited run.
func appendFrameRuns(sb *strings.Builder, lines []liveLine, frameWidth int) {
	for i := 0; i < len(lines); {
		j := i + 1
		for j < len(lines) && mergesAtWidth(lines[j-1].Text, lines[j-1].Width) {
			j++
		}
		sb.WriteString(msFrameRunText(lines[i:j], frameWidth))
		if !mergesAtWidth(lines[j-1].Text, lines[j-1].Width) {
			sb.WriteString("\x1b[K\r\n")
		}
		i = j
	}
}

// msFrameRunText renders a complete merged frame run through the ported
// Microsoft write/reflow path. Reflow is intentionally performed once for
// the whole run: a wide glyph's edge padding depends on the column left by
// all preceding wrapped rows, not just on the logical line containing it.
func msFrameRunText(lines []liveLine, frameWidth int) string {
	if len(lines) == 0 || frameWidth < 1 {
		return ""
	}

	height := 2
	for _, line := range lines {
		width := line.Width
		if width < 1 {
			width = frameWidth
		}
		height += cellLen(line.Text)/width + 2
	}
	width := lines[0].Width
	if width < 1 {
		width = frameWidth
	}
	t := newMsTerminal(width, height)
	for _, line := range lines {
		lineWidth := line.Width
		if lineWidth < 1 {
			lineWidth = width
		}
		if lineWidth != width {
			t.resize(lineWidth, height)
			width = lineWidth
		}
		t.Feed([]byte(line.Text))
		if !mergesAtWidth(line.Text, line.Width) {
			t.Feed([]byte("\r\n"))
		}
	}
	t.resize(frameWidth, height)
	t.flushPendingText()

	b := t.disp.page.buffer
	var sb strings.Builder
	for y := 0; y < b.Height(); y++ {
		r := b.GetRowByOffset(y)
		if r.MeasureRight() == 0 && !r.WasWrapForced() {
			break
		}
		sb.WriteString(msWriteInfosText(r))
	}
	// WriteInfos receives the complete CHAR_INFO row, including ordinary
	// trailing spaces. The frame splitter trims spaces only at the end of a
	// logical run; spaces on an intermediate row remain observable before the
	// next row's text.
	return strings.TrimRight(sb.String(), " ")
}

// msWriteInfosText is the text-only 1:1 port of
// VtIo::Writer::WriteInfos. The source's CHAR_INFO attributes are outside
// this oracle, but its leading/trailing wide-cell decisions are preserved.
// GetText returns the complete readable CHAR_INFO row, including ordinary
// trailing blank cells; the caller trims only the end of a complete run.
func msWriteInfosText(r *msROW) string {
	if r._columnCount == 0 {
		return ""
	}
	text := r.GetText()
	result := string(utf16.Decode(text))
	if r.WasDoubleBytePadded() {
		result += " "
	}
	return result
}

// LiveStream renders the same lines the way they arrive as they are printed:
// each logical line whole, terminated by CRLF -- including the exact-width
// ones, which is the boundary the frame will have lost.
func (m *mockConPTY) LiveStream(lines []string) []byte {
	var sb strings.Builder
	// conhost announces the window title before anything else. An OSC skipper
	// that knew only about BEL let this text leak into the first logical line
	// of a real capture; the mock never had it, so the mock never caught it.
	sb.WriteString("\x1b]0;C:\\probe.exe\x07")
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteString("\r\n")
	}
	return []byte(sb.String())
}

// LiveStreamScrolling models the other half of §17: while the buffer scrolls,
// a long line is split across partial redraws, so the live stream is *not*
// reliable for long lines. The reconciliation must not depend on it for those.
func (m *mockConPTY) LiveStreamScrolling(lines []string) []byte {
	var sb strings.Builder
	sb.WriteString("\x1b]0;C:\\probe.exe\x07")
	row := 0
	for _, l := range lines {
		sb.WriteString(l)
		row += rowsFor(l, m.Width)
		if rowsFor(l, m.Width) > 1 {
			// The measured shape: instead of a newline, conhost repositions
			// the cursor absolutely to the row where the next line goes.
			// There is no CRLF at all, which is what defeated the seam rules
			// this mock used to model -- a cursor handles it, a rule does not.
			// The fixture uses this at every multi-row line. That is a
			// deterministic instance of the documented shape, not a guessed
			// probability of when conhost chooses to repaint.
			fmt.Fprintf(&sb, "\x1b[%d;1H", row+1)
			continue
		}
		sb.WriteString("\r\n")
	}
	return []byte(sb.String())
}

// FrameInterleaved models what a resize during output really produces: the
// child keeps printing while the repaint is being written, so fresh output
// lands inside the frame rather than after it.
func (m *mockConPTY) FrameInterleaved(lines []string, writeWidth int, extra []string) ([]byte, []string) {
	frame := m.FrameAtWidth(lines, writeWidth)
	term := []byte("\x1b[K\r\n")
	boundary := bytes.Index(frame, term)
	if boundary < 0 {
		return frame, lines
	}
	boundary += len(term)

	var sb strings.Builder
	sb.Write(frame[:boundary])
	for _, e := range extra {
		sb.WriteString(e)
		sb.WriteString("\r\n")
	}
	sb.Write(frame[boundary:])
	return []byte(sb.String()), append(append([]string{}, lines...), extra...)
}

func (m *mockConPTY) fitRows(lines []string) ([]string, bool) {
	kept, cutMidLine := m.bufferLines(lines)
	return kept, cutMidLine
}

func (m *mockConPTY) fit(lines []string) []string {
	kept, _ := m.bufferLinesAtWidth(lines, m.Width)
	return kept
}

// frameSourcesAtWidth writes at the source width, resizes the ported buffer
// to the frame width, and reads the surviving logical lines back. This is the
// actual order of operations in a resize; trimming the ring before the resize
// would keep rows that conhost has already evicted during reflow.
func (m *mockConPTY) frameSourcesAtWidth(lines []string, writeWidth int) []liveLine {
	if writeWidth < 1 {
		writeWidth = m.Width
	}
	t := newMsTerminal(writeWidth, m.Height)
	for _, line := range lines {
		t.Feed([]byte(line))
		t.Feed([]byte("\r\n"))
	}
	t.resize(m.Width, m.Height)
	kept, _ := currentBufferLiveLines(t)
	return kept
}

// frameSourcesAtWidths is the same exchange with a resize during output.
// Each source line is written through the port at its recorded width, then
// the final frame width is applied once more before the buffer is read.
func (m *mockConPTY) frameSourcesAtWidths(lines []liveLine) []liveLine {
	width := m.Width
	if len(lines) > 0 && lines[0].Width > 0 {
		width = lines[0].Width
	}
	t := newMsTerminal(width, m.Height)
	for _, line := range lines {
		lineWidth := line.Width
		if lineWidth < 1 {
			lineWidth = width
		}
		if lineWidth != width {
			t.resize(lineWidth, m.Height)
			width = lineWidth
		}
		t.Feed([]byte(line.Text))
		t.Feed([]byte("\r\n"))
	}
	t.resize(m.Width, m.Height)
	kept, _ := currentBufferLiveLines(t)
	return kept
}

func liveTexts(lines []liveLine) []string {
	texts := make([]string, len(lines))
	for i, line := range lines {
		texts[i] = line.Text
	}
	return texts
}

// bufferLines reads the current rows of the ported TextBuffer after the
// supplied writes. This is the mock's ring, not a hand-counted approximation:
// AdaptDispatch performs delayed EOL wrapping, wide-glyph padding, and
// IncrementCircularBuffer exactly as the port does. The first surviving row
// may be the middle of a wrapped line, which is why it is returned as a text
// line without inventing its missing prefix.
func (m *mockConPTY) bufferLines(lines []string) ([]string, bool) {
	return m.bufferLinesAtWidth(lines, m.Width)
}

func (m *mockConPTY) bufferLinesAtWidth(lines []string, width int) ([]string, bool) {
	totalRows := RowsFor(lines, width)
	if totalRows <= m.Height {
		// The input is the complete buffer in this case. Keep explicit empty
		// logical lines: unlike trailing padding rows, they are part of the
		// caller's writes and are observable before the frame trim.
		return append([]string(nil), lines...), false
	}
	t := newMsTerminal(width, m.Height)
	for _, line := range lines {
		t.Feed([]byte(line))
		t.Feed([]byte("\r\n"))
	}

	kept := make([]string, 0, m.Height)
	keptLines, cutMidLine := currentBufferLiveLines(t)
	for _, line := range keptLines {
		kept = append(kept, line.Text)
	}
	for len(kept) > 0 && kept[len(kept)-1] == "" {
		kept = kept[:len(kept)-1]
	}
	return kept, cutMidLine
}

func currentBufferLiveLines(t *msTerminal) ([]liveLine, bool) {
	b := t.disp.page.buffer
	var out []liveLine
	var current strings.Builder
	currentWidth := 0
	started := false
	for y := 0; y < b.Height(); y++ {
		r := b.GetRowByOffset(y)
		rowWidth := r.writeWidth
		if rowWidth < 1 && r.MeasureRight() > 0 {
			rowWidth = b.Width()
		}
		if !started {
			currentWidth = rowWidth
			started = true
		}
		current.WriteString(string(utf16.Decode(r.GetTextRange(0, r.MeasureRight()))))
		if !r.WasWrapForced() {
			out = append(out, liveLine{Text: current.String(), Width: currentWidth})
			current.Reset()
			started = false
		}
	}
	if current.Len() > 0 {
		out = append(out, liveLine{Text: current.String(), Width: currentWidth})
	}
	for len(out) > 0 && out[len(out)-1].Text == "" && out[len(out)-1].Width == 0 {
		out = out[:len(out)-1]
	}

	// A leading wrapped row means that the top of the ring cut into a
	// logical line. The row's own wrap flag tells us that it continues.
	cutMidLine := b.Height() > 0 && b.GetRowByOffset(0).WasWrapForced()
	return out, cutMidLine
}

func contentRows(lines []string, width int) int {
	n := 0
	for _, l := range lines {
		n += rowsFor(l, width)
	}
	return n
}

// Chunks splits a byte stream the way a pipe delivers it: arbitrary sizes,
// arbitrary boundaries, including boundaries inside an escape sequence.
func (m *mockConPTY) Chunks(b []byte, maxChunk int) [][]byte {
	if maxChunk < 1 {
		maxChunk = 1
	}
	var out [][]byte
	for i := 0; i < len(b); {
		n := 1 + m.rnd.Intn(maxChunk)
		if i+n > len(b) {
			n = len(b) - i
		}
		out = append(out, b[i:i+n])
		i += n
	}
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
