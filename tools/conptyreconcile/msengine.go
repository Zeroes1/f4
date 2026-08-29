// The stream front end for the ported conhost model, and the ForwardTab /
// SetXPosition ports it needs.
//
// The ported parts (Copyright (c) Microsoft Corporation, MIT license,
// microsoft/terminal commit 079d1cc423336c89c1e220701c94b320cecb603a):
//   - AdaptDispatch::ForwardTab, AdaptDispatch::_InitTabStopsForWidth
//   - Cursor::SetXPosition
//
// The rest of this file is this tool's own routing, NOT a Microsoft port,
// and it is deliberately dumb: it decodes UTF-8 into UTF-16 and hands
// every recognized control to the *ported* handler with the parameters the
// original dispatch (OutputStateMachineEngine::ActionExecute /
// ActionCsiDispatch) would pass. The full StateMachine is a recorded
// non-port; the routed set is exactly:
//
//   C0:  BS -> CursorBackward(1), HT -> ForwardTab(1),
//        LF/FF/VT -> LineFeed(DependsOnMode), CR -> CarriageReturn()
//   CSI: A CUU, B CUD, C CUF, D CUB, G/` CHA, d VPA, H/f CUP,
//        J ED (0/1/2; 3 recorded non-port), K EL (0/1/2),
//        ?7h/?7l DECAWM -> the AutoWrap mode the ported code tests
//   XTWINOPS 8;h;w t -> buffer resize through the ported TextBuffer::Reflow
//   OSC  -> skipped to BEL/ST; other ESC -> one byte skipped
//   SGR (m), cursor show/hide (?25), title, and every other final are
//   consumed without effect and listed here rather than guessed at.
//
// One deliberate, recorded deviation lives in msTerminal.recycle below:
// when the ported IncrementCircularBuffer retires the top row, this tool
// copies the row's text and wrap flag aside before the ported code resets
// it. The copy is read-only; the ported buffer semantics are untouched.
// conhost keeps no scrollback (P16) -- the mirror this tool verifies is
// exactly the component that has to.

package main

import (
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// ported: ForwardTab and its tab-stop table
// ---------------------------------------------------------------------------

// AdaptDispatch::_InitTabStopsForWidth
func (d *msAdaptDispatch) _InitTabStopsForWidth(width int) {
	screenWidth := width
	initialWidth := len(d.tabStopColumns)
	if screenWidth > initialWidth {
		grown := make([]bool, screenWidth)
		copy(grown, d.tabStopColumns)
		d.tabStopColumns = grown
		if !d.initDefaultTabStopsOff {
			for column := 8; column < len(d.tabStopColumns); column += 8 {
				if column >= initialWidth {
					d.tabStopColumns[column] = true
				}
			}
		}
	}
}

// Cursor::SetXPosition
func (c *msCursor) SetXPosition(newX int) {
	c.ResetDelayEOLWrap()
	c._cPosition.x = newX
}

// AdaptDispatch::ForwardTab
func (d *msAdaptDispatch) ForwardTab(numTabs int) {
	page := &d.page
	cursor := page.Cursor()
	column := cursor.GetPosition().x
	row := cursor.GetPosition().y
	width := page.Buffer().GetLineWidth(row)
	tabsPerformed := 0

	topMargin, bottomMargin := d._GetVerticalMargins(page, true)
	_, rightMargin := d._GetHorizontalMargins(width)
	clampToMargin := row >= topMargin && row <= bottomMargin && column <= rightMargin
	maxColumn := width - 1
	if clampToMargin {
		maxColumn = rightMargin
	}

	d._InitTabStopsForWidth(width)
	for column < maxColumn && tabsPerformed < numTabs {
		column++
		if d.tabStopColumns[column] {
			tabsPerformed++
		}
	}

	// (see the original comment about delayed wrap preservation)
	delayedWrapOriginallySet := cursor.GetDelayEOLWrap() != nil
	cursor.SetXPosition(column)
	if delayedWrapOriginallySet {
		cursor.DelayEOLWrap()
	}
}

// ---------------------------------------------------------------------------
// the terminal: ported model + this tool's stream router
// ---------------------------------------------------------------------------

// msRetiredLine is the read-only copy taken when the ported circular buffer
// recycles its top row (see the file header).
type msRetiredLine struct {
	text    string
	wrapped bool
	width   int
}

type msTerminal struct {
	disp        *msAdaptDispatch
	retired     []msRetiredLine
	pending     []byte
	pendingText []uint16
}

// msDefaultUnsizedHeight is this tool's choice for streams that never
// announce a geometry (no ESC[8;;t): tall enough that nothing scrolls off
// before the first size report arrives. A tool setting, not a Microsoft
// value; recorded here and in the docs ledger.
const msDefaultUnsizedHeight = 8192

func newMsTerminal(width, height int) *msTerminal {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = msDefaultUnsizedHeight
	}
	t := &msTerminal{disp: newMsAdaptDispatch(width, height)}
	t.disp.writeWidth = width
	t.disp.page.buffer.onRecycle = t.recycle
	return t
}

// recycle: the recorded read-only tap (file header).
func (t *msTerminal) recycle(r *msROW) {
	t.retired = append(t.retired, msRetiredLine{
		text:    string(utf16.Decode(r.GetTextRange(0, r.MeasureRight()))),
		wrapped: r.WasWrapForced(),
		width:   r.writeWidth,
	})
}

func (t *msTerminal) resize(width, height int) {
	old := t.disp.page.buffer
	if width == old.Width() && height == old.Height() {
		return
	}
	newBuffer := newMsTextBuffer(width, height)
	newCursor := &msCursor{}
	msTextBufferReflow(old, newBuffer, t.disp.page.cursor, newCursor)
	newBuffer.onRecycle = t.recycle
	t.disp.page.buffer = newBuffer
	t.disp.page.cursor = newCursor
	t.disp.page.top = 0
	t.disp.writeWidth = width
}

// Feed routes the byte stream into the ported handlers (file header). The
// pending suffix is the parser's incomplete escape/UTF-8 sequence; retaining
// it is required because ConPTY reads are allowed to end at any byte.
func (t *msTerminal) Feed(input []byte) {
	b := make([]byte, 0, len(t.pending)+len(input))
	b = append(b, t.pending...)
	b = append(b, input...)
	t.pending = nil

	d := t.disp
	text := t.pendingText
	t.pendingText = nil
	flush := func() {
		if len(text) > 0 {
			d.PrintString(text)
			text = text[:0]
		}
	}
	i := 0
	incomplete := false
	for i < len(b) {
		c := b[i]
		switch {
		case c == 0x1b && i+1 >= len(b):
			incomplete = true
		case c == 0x1b && b[i+1] == '[':
			flush()
			j := i + 2
			for j < len(b) && (b[j] < 0x40 || b[j] > 0x7e) {
				j++
			}
			if j >= len(b) {
				incomplete = true
				break
			}
			t.csi(string(b[i+2:j]), b[j])
			i = j + 1
		case c == 0x1b && b[i+1] == ']':
			flush()
			end, ok := completeOSC(b, i)
			if !ok {
				incomplete = true
				break
			}
			i = end
		case c == 0x1b:
			flush()
			i += 2
		case c == '\r':
			flush()
			t.markEmptyRowWidth()
			d.CarriageReturn()
			i++
		case c == '\n' || c == '\v' || c == '\f':
			flush()
			t.markEmptyRowWidth()
			d.LineFeed()
			i++
		case c == '\b':
			flush()
			d.CursorBackward(1)
			i++
		case c == '\t':
			flush()
			d.ForwardTab(1)
			i++
		default:
			r, size := utf8.DecodeRune(b[i:])
			if r == utf8.RuneError && !utf8.FullRune(b[i:]) {
				incomplete = true
				break
			}
			if size < 1 {
				size = 1
			}
			if r >= 0x20 && r != 0x7f {
				if r > 0xFFFF {
					r1, r2 := utf16.EncodeRune(r)
					text = append(text, uint16(r1), uint16(r2))
				} else {
					text = append(text, uint16(r))
				}
			}
			i += size
		}
		if incomplete {
			break
		}
	}
	if incomplete && i < len(b) {
		t.pending = append(t.pending, b[i:]...)
	}
	t.pendingText = text
}

func (t *msTerminal) flushPendingText() {
	if len(t.pendingText) == 0 {
		return
	}
	t.disp.PrintString(t.pendingText)
	t.pendingText = nil
}

// markEmptyRowWidth is tool metadata only. Microsoft records no write width
// in ROW; this tap records the width for an explicitly emitted empty line so a
// later resize cannot mistake a reflow-created blank row for child output.
func (t *msTerminal) markEmptyRowWidth() {
	p := t.disp.page.cursor.GetPosition()
	r := t.disp.page.buffer.GetMutableRowByOffset(p.y)
	if r.MeasureRight() == 0 && !r.WasWrapForced() {
		r.writeWidth = t.disp.writeWidth
	}
}

// completeOSC is the bounded counterpart of skipOSC for the incremental
// parser. skipOSC intentionally treats an unterminated sequence as consuming
// the rest of a complete capture; Feed must instead retain that suffix.
func completeOSC(b []byte, i int) (int, bool) {
	for j := i + 2; j < len(b); j++ {
		if b[j] == 0x07 {
			return j + 1, true
		}
		if b[j] == 0x1b && j+1 < len(b) && b[j+1] == '\\' {
			return j + 2, true
		}
	}
	return len(b), false
}

func (t *msTerminal) csi(params string, final byte) {
	d := t.disp
	private := strings.HasPrefix(params, "?")
	nums := msCsiNums(params)
	arg := func(i, def int) int {
		if i < len(nums) && nums[i] > 0 {
			return nums[i]
		}
		return def
	}
	switch final {
	case 'H', 'f':
		// CSI cursor positions are 1-based. AdaptDispatch's position
		// methods operate on zero-based buffer coordinates, so perform the
		// same -1 conversion as the original dispatch before routing.
		d.CursorPosition(arg(0, 1)-1, arg(1, 1)-1)
	case 'A':
		d.CursorUp(arg(0, 1))
	case 'B':
		d.CursorDown(arg(0, 1))
	case 'C':
		d.CursorForward(arg(0, 1))
	case 'D':
		d.CursorBackward(arg(0, 1))
	case 'G', '`':
		d.CursorHorizontalPositionAbsolute(arg(0, 1) - 1)
	case 'd':
		d.VerticalLinePositionAbsolute(arg(0, 1) - 1)
	case 'J':
		n := 0
		if len(nums) > 0 {
			n = nums[0]
		}
		d.EraseInDisplay(msEraseType(n))
	case 'K':
		n := 0
		if len(nums) > 0 {
			n = nums[0]
		}
		d.EraseInLine(msEraseType(n))
	case 'h', 'l':
		if private && len(nums) > 0 && nums[0] == 7 {
			d.modeAutoWrap = final == 'h'
		}
	case 't':
		// XTWINOPS size report: ESC[8;rows;cols t (P14). conhost resizes
		// its buffer; here that is the ported TextBuffer::Reflow.
		if len(nums) >= 3 && nums[0] == 8 && nums[2] > 0 {
			h := nums[1]
			if h <= 0 {
				h = t.disp.page.buffer.Height()
			}
			t.resize(nums[2], h)
		}
	}
}

func msCsiNums(s string) []int {
	s = strings.TrimLeft(s, "?>=")
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ";")
	out := make([]int, len(parts))
	for i, p := range parts {
		if k := strings.IndexByte(p, ':'); k >= 0 {
			p = p[:k]
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			n = 0
		}
		out[i] = n
	}
	return out
}

// ---------------------------------------------------------------------------
// reading the model out: logical lines, the way TextBuffer::Reflow reads
// rows (join on WasWrapForced, text up to MeasureRight)
// ---------------------------------------------------------------------------

func (t *msTerminal) logicalLines() []liveLine {
	t.flushPendingText()
	b := t.disp.page.buffer
	w := b.Width()
	var out []liveLine
	var cur strings.Builder
	curWidth := 0
	started := false

	emitRow := func(text string, wrapped bool, width int) {
		if !started {
			curWidth = width
			started = true
		}
		cur.WriteString(text)
		if !wrapped {
			out = append(out, liveLine{Text: cur.String(), Width: curWidth})
			cur.Reset()
			started = false
		}
	}

	for _, r := range t.retired {
		emitRow(r.text, r.wrapped, r.width)
	}
	for y := 0; y < b.Height(); y++ {
		row := b.GetRowByOffset(y)
		rowWidth := row.writeWidth
		if rowWidth == 0 {
			rowWidth = w
		}
		emitRow(string(utf16.Decode(row.GetTextRange(0, row.MeasureRight()))), row.WasWrapForced(), rowWidth)
	}
	if cur.Len() > 0 {
		out = append(out, liveLine{Text: cur.String(), Width: curWidth})
	}
	// Trailing blank rows are buffer padding rather than printed lines.
	for len(out) > 0 && out[len(out)-1].Text == "" {
		out = out[:len(out)-1]
	}
	return out
}
