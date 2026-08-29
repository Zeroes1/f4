package main

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// The terminal side of direction F, kept portable so that all of it is tested
// here rather than on a tester's machine.
//
// The shape follows docs/CONPTY_RESEARCH.md §18. conhost owns a grid that is
// as tall as the scrollback is deep; f4 holds a mirror of the logical lines it
// has read out of frames, re-wraps them itself at any width, and shows the
// bottom slice. Because the mirror is logical lines rather than rows, a width
// change costs no round trip and a height change costs no history -- which is
// what makes the geometry switch for a full-screen program cheap.
//
// What is deliberately *not* here: attributes. The mirror stores text. Colour
// travels in the stream as SGR and is applied at render time by f4's existing
// renderer; the reflow only has to get the boundaries right.

// ---------------------------------------------------------------------------
// the mirror
// ---------------------------------------------------------------------------

// Mirror is f4's copy of the child's output as logical lines.
type Mirror struct {
	lines []string

	// scroll is how many rows above the bottom the viewport is parked. Zero
	// is the live bottom, which is where output leaves it.
	scroll int
}

func NewMirror() *Mirror { return &Mirror{} }

// Replace swaps the whole mirror, which is what a frame produces: a frame
// covers the entire buffer, so nothing is merged and nothing has to be
// identified. This is the property §7's design lacked.
func (m *Mirror) Replace(lines []string) {
	m.lines = append(m.lines[:0], lines...)
	m.clampScroll(0)
}

// Append adds lines observed in the live stream between frames.
func (m *Mirror) Append(lines ...string) {
	m.lines = append(m.lines, lines...)
}

func (m *Mirror) Lines() []string { return m.lines }

// Retire drops the oldest lines once the mirror is deeper than the configured
// history. conhost's own ring does the same at the buffer height (§17); f4
// bounds its copy in logical lines, which is the fix recorded in 6.12 -- a row
// bound evicts on every narrowing step, a line bound does not.
func (m *Mirror) Retire(maxLines int) {
	if maxLines > 0 && len(m.lines) > maxLines {
		drop := len(m.lines) - maxLines
		m.lines = append(m.lines[:0], m.lines[drop:]...)
	}
}

// ---------------------------------------------------------------------------
// reflow
// ---------------------------------------------------------------------------

// Row is one visual row: which logical line it came from, and the offset
// within that line where it starts. The offsets are what make the mapping
// between the screen and the mirror exact in both directions.
type Row struct {
	Line   int
	Offset int
	Text   string
}

// Wrap lays the mirror out at the given width. An empty logical line still
// occupies one row, which is what makes the row count match a terminal's.
func Wrap(lines []string, width int) []Row {
	if width < 1 {
		width = 1
	}
	rows := make([]Row, 0, len(lines))
	for i, line := range lines {
		if line == "" {
			rows = append(rows, Row{Line: i, Offset: 0, Text: ""})
			continue
		}
		if !utf8.ValidString(line) {
			// The Microsoft model is UTF-16 based and therefore cannot retain
			// malformed UTF-8 bytes. Wrap still has a byte-preservation contract
			// for arbitrary Go strings, so leave this exceptional input opaque.
			off, rest := 0, line
			for rest != "" {
				head, tail := takeRawRow(rest, width)
				rows = append(rows, Row{Line: i, Offset: off, Text: head})
				off += len(head)
				rest = tail
			}
			continue
		}
		// Where a logical line breaks at a given width is conhost's decision,
		// not ours: the line is written into a ported buffer of that width and
		// the rows it occupies are read back, joined exactly as
		// TextBuffer::Reflow reads them (WasWrapForced / MeasureRight). The
		// hand-written cut loop that used to be here was a reimplementation.
		t := newMsTerminal(width, msRowsForSafely(line, width))
		t.Feed([]byte(line))
		t.flushPendingText()
		b := t.disp.page.buffer
		last := t.disp.page.cursor.GetPosition().y

		off := 0
		for y := 0; y <= last && y < b.Height(); y++ {
			r := b.GetRowByOffset(y)
			text := string(utf16.Decode(r.GetTextRange(0, r.MeasureRight())))
			if y > 0 && text == "" && !r.WasWrapForced() {
				// Past the end of the line: the buffer's own blank padding.
				break
			}
			rows = append(rows, Row{Line: i, Offset: off, Text: text})
			off += len(text)
			if !r.WasWrapForced() {
				break
			}
		}
	}
	return rows
}

// RowsFor is the row count without building the rows, for sizing decisions.
func RowsFor(lines []string, width int) int {
	if width < 1 {
		width = 1
	}
	// The row count has to agree with Wrap exactly, or a viewport sized from
	// it will not match the rows drawn into it. So it is Wrap's count, which
	// is conhost's, rather than a second measurement made independently.
	return len(Wrap(lines, width))
}

// ---------------------------------------------------------------------------
// the visible slice
// ---------------------------------------------------------------------------

// Viewport is what f4 draws: the bottom `height` rows of the wrapped mirror,
// offset upwards by the scroll position.
type Viewport struct {
	Rows  []Row
	Width int

	// Top is the index, within the wrapped rows, of the first visible row.
	Top int

	// Total is how many rows the whole mirror occupies at this width.
	Total int
}

// Slice wraps the mirror and returns the visible window. Short histories are
// bottom-aligned the way a terminal is: the content sits at the top of the
// window and the remainder is blank, so `Rows` may be shorter than height.
func (m *Mirror) Slice(width, height int) Viewport {
	if height < 1 {
		height = 1
	}
	all := Wrap(m.lines, width)
	m.clampScroll(len(all) - height)

	top := len(all) - height - m.scroll
	if top < 0 {
		top = 0
	}
	end := top + height
	if end > len(all) {
		end = len(all)
	}
	return Viewport{Rows: all[top:end], Width: width, Top: top, Total: len(all)}
}

func (m *Mirror) clampScroll(max int) {
	if max < 0 {
		max = 0
	}
	if m.scroll > max {
		m.scroll = max
	}
	if m.scroll < 0 {
		m.scroll = 0
	}
}

// ScrollBy moves the viewport by whole rows, positive being upwards into
// history, and reports where it ended up. It is clamped, never wrapped: a
// terminal that jumps to the top when the user scrolls one row past the end is
// worse than one that stops.
func (m *Mirror) ScrollBy(delta, width, height int) int {
	total := RowsFor(m.lines, width)
	m.scroll += delta
	m.clampScroll(total - height)
	return m.scroll
}

func (m *Mirror) ScrollToBottom() { m.scroll = 0 }

func (m *Mirror) ScrollPosition() int { return m.scroll }

// ---------------------------------------------------------------------------
// coordinates
// ---------------------------------------------------------------------------

// Position is a point in the mirror: a logical line and an offset within it.
type Position struct {
	Line   int
	Offset int
}

// ScreenToMirror maps a point in the visible window to a point in the mirror.
// It is what mouse clicks and selections need, and it is why Row carries an
// offset: at any width other than the one the line was written at, a screen
// column is not a character index.
func (v Viewport) ScreenToMirror(col, row int) (Position, bool) {
	if row < 0 || row >= len(v.Rows) || col < 0 {
		return Position{}, false
	}
	r := v.Rows[row]
	// col is a screen column; the offset it names is a byte offset into the
	// row's text, so the conversion has to walk cells.
	head, _ := cutCells(r.Text, col)
	return Position{Line: r.Line, Offset: r.Offset + len(head)}, true
}

// MirrorToScreen is the inverse, and reports false when the position is not
// currently visible -- scrolled away, or in a line the viewport does not show.
func (v Viewport) MirrorToScreen(p Position) (col, row int, ok bool) {
	for i, r := range v.Rows {
		if r.Line != p.Line {
			continue
		}
		if p.Offset >= r.Offset && p.Offset <= r.Offset+len(r.Text) {
			return cellLen(r.Text[:p.Offset-r.Offset]), i, true
		}
	}
	return 0, 0, false
}

// ---------------------------------------------------------------------------
// geometry: what the child is told
// ---------------------------------------------------------------------------

// Geometry is the size f4 gives the pseudoconsole. In ordinary use the height
// is the scrollback depth and the child is told something untrue about it;
// while a full-screen program runs, the child must be told the truth or it
// draws its frame across thirty thousand rows (§17).
type Geometry struct {
	Width  int
	Height int
}

// GeometryFor returns the size to hand ConPTY. `windowRows` is what f4 really
// shows; `historyRows` is the configured depth.
func GeometryFor(width, windowRows, historyRows int, fullScreen bool) Geometry {
	if width < 1 {
		width = 1
	}
	if windowRows < 1 {
		windowRows = 1
	}
	if fullScreen || historyRows < windowRows {
		return Geometry{Width: width, Height: windowRows}
	}
	return Geometry{Width: width, Height: historyRows}
}

// SafeHistoryRows caps the configured depth against the cell-product ceiling
// measured in §17 -- beyond roughly 32 million cells the host stops producing
// output, and higher still stops being closable. The default cap leaves a
// factor of two, and the number is a setting per §16 rather than a constant
// here; this function only enforces whatever it is given.
func SafeHistoryRows(width, wanted, maxCells int) int {
	if width < 1 || maxCells < 1 {
		return wanted
	}
	limit := maxCells / width
	if limit < 1 {
		limit = 1
	}
	if wanted > limit {
		return limit
	}
	return wanted
}

// ---------------------------------------------------------------------------
// full-screen detection
// ---------------------------------------------------------------------------

// FullScreenState tracks whether the child is drawing a full-screen interface,
// which decides the geometry above. §17 measured both mechanisms this can rest
// on and found each incomplete on its own, so this is explicitly layered and
// says which layer answered.
type FullScreenState struct {
	// altScreen is set by ESC[?1049h and cleared by ESC[?1049l. Available on
	// builds whose emitter passes it through; on 10.0.22000 conhost consumes
	// it and this layer never fires.
	altScreen bool

	// process is set by the caller when a watched program appears among the
	// child's descendants. Deterministic, and the only layer that is not an
	// inference -- but it needs process enumeration, so it lives outside.
	process bool
}

// Feed scans a chunk of the stream for alternate-screen transitions. It is
// tolerant of chunk boundaries: a sequence split across two reads is simply
// not seen, which is safe, because the state only changes on a complete one.
func (f *FullScreenState) Feed(chunk []byte) {
	s := string(chunk)
	// Later occurrences win, so a chunk containing both enter and leave ends
	// in the right state.
	for i := 0; i+7 < len(s); i++ {
		if strings.HasPrefix(s[i:], "\x1b[?1049h") {
			f.altScreen = true
		} else if strings.HasPrefix(s[i:], "\x1b[?1049l") {
			f.altScreen = false
		}
	}
}

// SetProcess is the deterministic layer's input: a watched image name appeared
// or exited among the child's descendants.
func (f *FullScreenState) SetProcess(running bool) { f.process = running }

// Active reports whether the child should be given a real-sized console, and
// which layer decided, so a log can say why the geometry changed.
func (f *FullScreenState) Active() (bool, string) {
	switch {
	case f.process:
		return true, "process"
	case f.altScreen:
		return true, "alternate screen"
	default:
		return false, ""
	}
}
