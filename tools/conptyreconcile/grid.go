package main

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// A terminal, in the sense the research document means it: the stream is
// applied to a grid with a cursor, and a row is marked as wrapped when *this*
// code wraps it. Logical lines are then rows joined by that flag.
//
// This replaces a set of hand-written rules about "seams" -- a CRLF followed
// by a cursor move meant a continuation, and so on -- which were my invention
// and were wrong. A real capture showed why: a 119-column line in a 120-column
// console ended with no CRLF at all, followed by an absolute `ESC[30;1H`,
// because conhost repositioned the cursor rather than emitting a newline and a
// blank row. No amount of patching a seam rule reaches that; a cursor does.
//
// The document said so already -- finding P11: read the bytes with a cursor
// model, not a CRLF split. This is that model.
//
// The semantics are Microsoft's, from src/buffer/out:
//
//   - ROW::SetWrapForced is set when a write runs past the last column.
//     TextBuffer::Reflow does the same: "if (newX >= newWidth) { SetWrapForced(true); }"
//   - ROW::SetDoubleBytePadded marks the cell left empty when a double-width
//     glyph will not fit at the end of a row and moves down whole.
//   - a write of exactly the width leaves the cursor pending at the edge
//     (delayed EOL wrap) rather than moving to the next row; the wrap happens
//     when the next glyph arrives.

type gridRow struct {
	width   int    // the console width in force when this row was written
	cells   []rune // one entry per column; a wide glyph occupies two, the second being 0
	wrapped bool   // ROW::WasWrapForced
	padded  bool   // ROW::WasDoubleBytePadded
	used    int    // columns written
}

// Grid is the terminal's screen, with a height and a scrollback, because an
// absolute cursor position only means anything relative to a screen of a known
// size. conhost repositions with ESC[<row>;<col>H while the buffer scrolls,
// and a grid that grew without bound would place those rows in the wrong
// place entirely.
type Grid struct {
	width, height int
	rows          []*gridRow
	retired       []*gridRow // rows that scrolled off the top
	x, y          int
	pendingWrap   bool // delayed EOL wrap: the cursor sits past the last column
}

func NewGrid(width int) *Grid { return NewGridSized(width, 0) }

// NewGridSized takes the console height. Zero means unbounded, which is right
// only when the stream is known not to scroll.
func NewGridSized(width, height int) *Grid {
	if width < 1 {
		width = 1
	}
	g := &Grid{width: width, height: height}
	g.ensure(0)
	return g
}

// scrollIfNeeded moves the top row into the scrollback when the cursor runs
// past the bottom, which is what a terminal does and what makes an absolute
// cursor position land on the row conhost meant.
func (g *Grid) scrollIfNeeded() {
	if g.height <= 0 {
		return
	}
	for g.y >= g.height {
		if len(g.rows) > 0 {
			g.retired = append(g.retired, g.rows[0])
			g.rows = g.rows[1:]
		}
		g.y--
	}
}

// resizeTo retires everything laid out at the old width and starts a fresh
// screen at the new one. Reflowing what is already there would mean porting
// TextBuffer::Reflow as well, and the frame that follows a resize carries the
// whole buffer anyway (§13).
func (g *Grid) resizeTo(width, height int) {
	// The row the cursor is sitting on, if nothing has been written to it, is
	// not a line -- it is where the next line would go. Retiring it would add
	// a blank line between the text written before the resize and the text
	// written after.
	rows := g.rows
	for len(rows) > 0 {
		last := rows[len(rows)-1]
		if last.used == 0 && !last.wrapped {
			rows = rows[:len(rows)-1]
			continue
		}
		break
	}
	g.retired = append(g.retired, rows...)
	g.rows = nil
	g.width = width
	if height > 0 {
		g.height = height
	}
	g.x, g.y = 0, 0
	g.pendingWrap = false
	g.ensure(0)
}

func (g *Grid) ensure(y int) {
	for len(g.rows) <= y {
		g.rows = append(g.rows, &gridRow{width: g.width, cells: make([]rune, g.width)})
	}
}

func (g *Grid) row(y int) *gridRow {
	g.ensure(y)
	return g.rows[y]
}

// put writes one glyph, wrapping as a terminal does.
func (g *Grid) put(r rune) {
	w := cellWidth(r)
	if w == 0 {
		// A combining mark attaches to what is already there and moves
		// nothing. Storing it is not needed for line structure.
		return
	}

	if g.pendingWrap {
		// The previous write filled the row exactly; the wrap happens now,
		// which is what makes this row a continuation.
		g.row(g.y).wrapped = true
		g.y++
		g.x = 0
		g.pendingWrap = false
		g.scrollIfNeeded()
	}

	g.scrollIfNeeded()
	if g.x+w > g.width {
		// A double-width glyph that will not fit moves down whole and leaves
		// a padding cell behind -- ROW::SetDoubleBytePadded. The padding is
		// not text and must not become a space in the logical line.
		row := g.row(g.y)
		row.padded = true
		row.wrapped = true
		g.y++
		g.x = 0
		g.scrollIfNeeded()
	}

	row := g.row(g.y)
	row.cells[g.x] = r
	if g.x+1 < g.width && w == 2 {
		row.cells[g.x+1] = 0
	}
	g.x += w
	if g.x > row.used {
		row.used = g.x
	}
	if g.x >= g.width {
		g.x = g.width
		g.pendingWrap = true
	}
}

func (g *Grid) newline() {
	g.pendingWrap = false
	g.y++
	g.x = 0
	g.scrollIfNeeded()
	g.ensure(g.y)
}

func (g *Grid) carriageReturn() {
	g.pendingWrap = false
	g.x = 0
}

func (g *Grid) moveTo(y, x int) {
	if y < 0 {
		y = 0
	}
	if x < 0 {
		x = 0
	}
	if x > g.width {
		x = g.width
	}
	g.pendingWrap = false
	g.y = y
	g.x = x
	g.ensure(g.y)
}

func (g *Grid) eraseToEnd() {
	row := g.row(g.y)
	for i := g.x; i < g.width; i++ {
		row.cells[i] = 0
	}
	if row.used > g.x {
		row.used = g.x
	}
	// Erasing to the end of a row means the row ends here: it cannot be a
	// continuation of anything.
	row.wrapped = false
}

func (g *Grid) eraseAll() {
	g.rows = nil
	g.ensure(0)
	g.x, g.y = 0, 0
	g.pendingWrap = false
}

// Feed applies a chunk of the stream.
func (g *Grid) Feed(b []byte) {
	i := 0
	for i < len(b) {
		c := b[i]
		switch {
		case c == 0x1b && i+1 < len(b) && b[i+1] == '[':
			j := i + 2
			for j < len(b) && !(b[j] >= 0x40 && b[j] <= 0x7e) {
				j++
			}
			if j < len(b) {
				g.csi(string(b[i+2:j]), b[j])
			}
			i = j + 1
		case c == 0x1b && i+1 < len(b) && b[i+1] == ']':
			i = skipOSC(b, i)
		case c == 0x1b:
			i += 2
		case c == '\r':
			g.carriageReturn()
			i++
		case c == '\n':
			g.newline()
			i++
		case c == '\b':
			if g.x > 0 {
				g.x--
			}
			g.pendingWrap = false
			i++
		case c == '\t':
			next := ((g.x / 8) + 1) * 8
			if next >= g.width {
				g.pendingWrap = true
				g.x = g.width
			} else {
				g.x = next
			}
			i++
		default:
			r, size := decodeRune(b[i:])
			if size < 1 {
				size = 1
			}
			if r >= 0x20 && r != 0x7f {
				g.put(r)
			}
			i += size
		}
	}
}

func decodeRune(b []byte) (rune, int) {
	if len(b) == 0 {
		return 0, 0
	}
	return utf8.DecodeRune(b)
}

func (g *Grid) csi(params string, final byte) {
	nums := csiNums(params)
	arg := func(i, def int) int {
		if i < len(nums) && nums[i] > 0 {
			return nums[i]
		}
		return def
	}
	switch final {
	case 'H', 'f':
		g.moveTo(arg(0, 1)-1, arg(1, 1)-1)
	case 'A':
		g.moveTo(maxInt(0, g.y-arg(0, 1)), g.x)
	case 'B':
		g.moveTo(g.y+arg(0, 1), g.x)
	case 'C':
		g.moveTo(g.y, minInt(g.width, g.x+arg(0, 1)))
	case 'D':
		g.moveTo(g.y, maxInt(0, g.x-arg(0, 1)))
	case 'G':
		g.moveTo(g.y, arg(0, 1)-1)
	case 'd':
		g.moveTo(arg(0, 1)-1, g.x)
	case 'J':
		if arg(0, 0) >= 2 {
			g.eraseAll()
		} else {
			g.eraseToEnd()
		}
	case 'K':
		g.eraseToEnd()
	case 't':
		// The XTWINOPS size report announces a new geometry. Lines already
		// completed keep their identity; what follows is laid out at the new
		// width. Reflowing what is already there would mean porting
		// TextBuffer::Reflow as well, and the frame that follows a resize
		// carries the whole buffer anyway (§13).
		if len(nums) >= 3 && nums[0] == 8 && nums[2] > 0 && nums[2] != g.width {
			g.resizeTo(nums[2], nums[1])
		}
	}
}

func csiNums(s string) []int {
	s = strings.TrimLeft(s, "?>=")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	out := make([]int, len(parts))
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		out[i] = n
	}
	return out
}

// LogicalLinesWithWidth is LogicalLines with the width each line was written
// at, which is what decides whether conhost merged it -- and which differs
// within one capture when the window was resized while output was arriving.
func (g *Grid) LogicalLinesWithWidth() []liveLine {
	var out []liveLine
	var cur strings.Builder
	w := g.width
	started := false

	all := make([]*gridRow, 0, len(g.retired)+len(g.rows))
	all = append(all, g.retired...)
	all = append(all, g.rows...)
	for _, r := range all {
		if !started {
			w = r.width
			started = true
		}
		for i := 0; i < r.used && i < len(r.cells); i++ {
			if r.cells[i] == 0 {
				continue
			}
			cur.WriteRune(r.cells[i])
		}
		if !r.wrapped {
			out = append(out, liveLine{Text: cur.String(), Width: w})
			cur.Reset()
			started = false
		}
	}
	if cur.Len() > 0 {
		out = append(out, liveLine{Text: cur.String(), Width: w})
	}
	for len(out) > 0 && out[len(out)-1].Text == "" {
		out = out[:len(out)-1]
	}
	return out
}

// LogicalLines reads the grid the way TextBuffer::Reflow does: a row whose
// wrapForced is set continues into the next one.
func (g *Grid) LogicalLines() []string {
	var out []string
	var cur strings.Builder
	all := make([]*gridRow, 0, len(g.retired)+len(g.rows))
	all = append(all, g.retired...)
	all = append(all, g.rows...)
	for _, r := range all {
		for i := 0; i < r.used && i < len(r.cells); i++ {
			if r.cells[i] == 0 {
				continue // the trailing half of a wide glyph, or an erased cell
			}
			cur.WriteRune(r.cells[i])
		}
		if !r.wrapped {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	// Trailing blank rows are buffer padding rather than printed lines.
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
