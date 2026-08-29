package main

import (
	"math/rand"
	"strings"
)

// A mock of the ConPTY emission grammar, so the reconciliation can be
// exercised without Windows.
//
// It is not a port of conhost. It reproduces the grammar this project
// measured on 10.0.22000 (docs/CONPTY_RESEARCH.md §13, §17) and that the
// pre-#17510 VtEngine in microsoft/terminal explains:
//
//   - a frame opens with ESC[?25l, the XTWINOPS size report, and ESC[H
//   - a logical line is emitted whole; the receiver's autowrap places it
//   - a logical line is terminated by ESC[K CR LF
//   - EXCEPT when its length is a non-zero multiple of the width, in which
//     case it exactly fills its rows, no terminator is emitted, and it merges
//     with the line that follows -- the P13 failure, measured in §17
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
func rowsFor(line string, width int) int {
	if len(line) == 0 {
		return 1
	}
	n := (len(line) + width - 1) / width
	if n < 1 {
		n = 1
	}
	return n
}

// mergesWithNext reports the measured P13 condition: a line that exactly
// fills its rows loses its boundary.
func mergesWithNext(line string, width int) bool {
	return len(line) > 0 && len(line)%width == 0
}

// FrameAtWidth renders the repaint produced by a resize to a *different*
// width than the lines were written at. This is the ordinary case in the
// field -- a frame is obtained by changing the width -- and it is where the
// merge boundary stops being a multiple of the frame width and stays a
// multiple of the write width. A mock that could not express this let a real
// bug through: the correction searched for multiples of the frame width,
// found none, and silently did nothing.
func (m *mockConPTY) FrameAtWidth(lines []string, writeWidth int) []byte {
	framed := &mockConPTY{Width: m.Width, Height: m.Height, rnd: m.rnd}
	kept := framed.fit(lines)

	var sb strings.Builder
	sb.WriteString("\x1b[?25l\x1b[8;")
	sb.WriteString(itoa(m.Height))
	sb.WriteString(";")
	sb.WriteString(itoa(m.Width))
	sb.WriteString("t\x1b[H")

	used := 0
	for _, l := range kept {
		sb.WriteString(l)
		// The merge is decided by the width in force when the line was
		// written, not by the width of this frame.
		if !mergesWithNext(l, writeWidth) {
			sb.WriteString("\x1b[K\r\n")
		}
		used += rowsFor(l, m.Width)
	}
	for ; used < m.Height-1; used++ {
		sb.WriteString("\x1b[K\r\n")
	}
	sb.WriteString("\x1b[K")
	sb.WriteString("\x1b[")
	sb.WriteString(itoa(min(m.Height, contentRows(kept, m.Width)+1)))
	sb.WriteString(";1H\x1b[?25h")
	return []byte(sb.String())
}

// FrameAtWidths renders a frame over lines whose first `split` entries were
// written at widthA and the rest at widthB -- the shape produced when the
// window is resized while output is still arriving.
func (m *mockConPTY) FrameAtWidths(lines []string, split, widthA, widthB int) []byte {
	kept := m.fit(lines)
	offset := len(lines) - len(kept)

	var sb strings.Builder
	sb.WriteString("\x1b[?25l\x1b[8;")
	sb.WriteString(itoa(m.Height))
	sb.WriteString(";")
	sb.WriteString(itoa(m.Width))
	sb.WriteString("t\x1b[H")

	used := 0
	for i, l := range kept {
		w := widthB
		if i+offset < split {
			w = widthA
		}
		sb.WriteString(l)
		if !mergesWithNext(l, w) {
			sb.WriteString("\x1b[K\r\n")
		}
		used += rowsFor(l, m.Width)
	}
	for ; used < m.Height-1; used++ {
		sb.WriteString("\x1b[K\r\n")
	}
	sb.WriteString("\x1b[K\x1b[")
	sb.WriteString(itoa(min(m.Height, contentRows(kept, m.Width)+1)))
	sb.WriteString(";1H\x1b[?25h")
	return []byte(sb.String())
}

// Frame renders the repaint a resize produces, over the given logical lines.
// Lines beyond the buffer height are dropped from the top, which is the ring
// behaviour §17 measured (history depth equals buffer height).
func (m *mockConPTY) Frame(lines []string) []byte {
	kept := m.fit(lines)

	var sb strings.Builder
	sb.WriteString("\x1b[?25l")
	sb.WriteString("\x1b[8;")
	sb.WriteString(itoa(m.Height))
	sb.WriteString(";")
	sb.WriteString(itoa(m.Width))
	sb.WriteString("t")
	sb.WriteString("\x1b[H")

	used := 0
	for _, l := range kept {
		sb.WriteString(l)
		if !mergesWithNext(l, m.Width) {
			sb.WriteString("\x1b[K\r\n")
		}
		used += rowsFor(l, m.Width)
	}
	// The rest of the buffer is blank rows, each costing exactly ESC[K CR LF.
	for ; used < m.Height-1; used++ {
		sb.WriteString("\x1b[K\r\n")
	}
	sb.WriteString("\x1b[K")

	sb.WriteString("\x1b[")
	sb.WriteString(itoa(min(m.Height, contentRows(kept, m.Width)+1)))
	sb.WriteString(";1H")
	sb.WriteString("\x1b[?25h")
	return []byte(sb.String())
}

// LiveStream renders the same lines the way they arrive as they are printed:
// each logical line whole, terminated by CRLF -- including the exact-width
// ones, which is the boundary the frame will have lost.
func (m *mockConPTY) LiveStream(lines []string) []byte {
	var sb strings.Builder
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
	for _, l := range lines {
		if len(l) > m.Width && m.rnd.Intn(2) == 0 {
			// The measured shape: the piece, a CRLF, then an absolute cursor
			// position, then the continuation. The reposition is what marks
			// this as a seam rather than a line ending.
			cut := m.Width
			sb.WriteString(l[:cut])
			sb.WriteString("\r\n")
			sb.WriteString("\x1b[")
			sb.WriteString(itoa(m.Height - 1))
			sb.WriteString(";")
			sb.WriteString(itoa(m.Width))
			sb.WriteString("H")
			sb.WriteString(l[cut:])
			sb.WriteString("\r\n")
			continue
		}
		sb.WriteString(l)
		sb.WriteString("\r\n")
	}
	return []byte(sb.String())
}

func (m *mockConPTY) fit(lines []string) []string {
	total := 0
	for _, l := range lines {
		total += rowsFor(l, m.Width)
	}
	if total <= m.Height {
		return lines
	}
	drop := 0
	for i := 0; i < len(lines) && total > m.Height; i++ {
		total -= rowsFor(lines[i], m.Width)
		drop = i + 1
	}
	return lines[drop:]
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
