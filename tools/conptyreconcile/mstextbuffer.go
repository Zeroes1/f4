// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of the used subset of microsoft/terminal
// src/buffer/out/cursor.cpp and src/buffer/out/textBuffer.cpp
// (commit 079d1cc423336c89c1e220701c94b320cecb603a):
// Cursor position + delayed EOL wrap; TextBuffer circular row storage
// (_firstRow, _getRow modulo mapping, scratchpad row), Replace, Insert,
// GetLineWidth, ClampPositionWithinLine, IncrementCircularBuffer,
// ScrollRows, CopyRow, FillRect, GetLastNonSpaceCharacter.
//
// Mechanical transformations required by Go, and nothing else:
//   - the raw VirtualAlloc arena with a commit watermark -> a Go slice of
//     rows allocated up front; _getRow keeps the exact modulo arithmetic.
//     _estimateOffsetOfLastCommittedRow therefore reports the full height,
//     which is the value the original converges to once rows are touched.
//   - til::rect -> msRect with the same exclusive right/bottom convention.
//
// Recorded non-ports: _PruneHyperlinks (hyperlinks absent), ImageSlice,
// TriggerRedraw/TriggerScroll/TriggerNewTextNotification (renderer
// notifications; no renderer here), the cell-by-cell narrow-rect path of
// ScrollRows' callers (see msdispatch.go).

package main

// ---------------------------------------------------------------------------
// cursor.cpp
// ---------------------------------------------------------------------------

type msCursor struct {
	_cPosition      msPoint
	_coordDelayedAt *msPoint
}

func (c *msCursor) GetPosition() msPoint { return c._cPosition }

// Cursor::SetPosition: "so we call ResetDelayEOLWrap() independent of
// _cPosition != cPosition" (cursor.cpp).
func (c *msCursor) SetPosition(p msPoint) {
	c._cPosition = p
	c.ResetDelayEOLWrap()
}

func (c *msCursor) SetYPosition(y int) {
	c._cPosition.y = y
	c.ResetDelayEOLWrap()
}

// Cursor::DelayEOLWrap
func (c *msCursor) DelayEOLWrap() {
	p := c._cPosition
	c._coordDelayedAt = &p
}

// Cursor::ResetDelayEOLWrap
func (c *msCursor) ResetDelayEOLWrap() { c._coordDelayedAt = nil }

// Cursor::GetDelayEOLWrap
func (c *msCursor) GetDelayEOLWrap() *msPoint { return c._coordDelayedAt }

// ---------------------------------------------------------------------------
// textBuffer.cpp
// ---------------------------------------------------------------------------

// msRect is til::rect: right and bottom are exclusive.
type msRect struct{ left, top, right, bottom int }

func (r msRect) width() int  { return r.right - r.left }
func (r msRect) height() int { return r.bottom - r.top }
func (r msRect) empty() bool { return r.left >= r.right || r.top >= r.bottom }

type msTextBuffer struct {
	_width             int
	_height            int
	_firstRow          int
	_storage           []*msROW
	_scratch           *msROW
	_currentAttributes TextAttribute

	// onRecycle is NOT part of the original TextBuffer. It is the recorded
	// read-only observation tap described in msengine.go: called with the
	// top row immediately before IncrementCircularBuffer resets it. It must
	// not mutate the row or the buffer.
	onRecycle func(*msROW)
}

func newMsTextBuffer(width, height int) *msTextBuffer {
	b := &msTextBuffer{_width: width, _height: height}
	b._storage = make([]*msROW, height)
	for i := range b._storage {
		b._storage[i] = newMsROW(width)
	}
	b._scratch = newMsROW(width)
	return b
}

func (b *msTextBuffer) Width() int  { return b._width }
func (b *msTextBuffer) Height() int { return b._height }

// TextBuffer::_getRow: "Rows are stored circularly, so the index you ask
// for is offset by the start position and mod the total of rows."
func (b *msTextBuffer) _getRow(y int) *msROW {
	offset := (b._firstRow + y) % b._height

	// Support negative wrap around. This way an index of -1 will
	// wrap to _rowCount-1 and make implementing scrolling easier.
	if offset < 0 {
		offset += b._height
	}

	return b._storage[offset]
}

// TextBuffer::GetRowByOffset / GetMutableRowByOffset
func (b *msTextBuffer) GetRowByOffset(index int) *msROW        { return b._getRow(index) }
func (b *msTextBuffer) GetMutableRowByOffset(index int) *msROW { return b._getRow(index) }

// TextBuffer::GetScratchpadRow: "Returns a row filled with whitespace and
// the given attributes, for you to freely use."
func (b *msTextBuffer) GetScratchpadRow(attributes TextAttribute) *msROW {
	b._scratch.Reset(attributes)
	return b._scratch
}

// TextBuffer::GetLineWidth
func (b *msTextBuffer) GetLineWidth(row int) int {
	// Use shift right to quickly divide the width by 2 for double width lines.
	scale := 0
	if b.IsDoubleWidthLine(row) {
		scale = 1
	}
	return b._width >> scale
}

func (b *msTextBuffer) IsDoubleWidthLine(row int) bool {
	return b.GetRowByOffset(row).GetLineRendition() != LineRenditionSingleWidth
}

// TextBuffer::ClampPositionWithinLine
func (b *msTextBuffer) ClampPositionWithinLine(position msPoint) msPoint {
	rightmostColumn := b.GetLineWidth(position.y) - 1
	if position.x < rightmostColumn {
		return msPoint{position.x, position.y}
	}
	return msPoint{rightmostColumn, position.y}
}

// TextBuffer::Replace
func (b *msTextBuffer) Replace(row int, attributes TextAttribute, state *msRowWriteState) {
	r := b.GetMutableRowByOffset(row)
	r.ReplaceText(state)
	r.ReplaceAttributes(state.columnBegin, state.columnEnd, attributes)
}

// TextBuffer::Insert
func (b *msTextBuffer) Insert(row int, attributes TextAttribute, state *msRowWriteState) {
	r := b.GetMutableRowByOffset(row)
	scratch := b.GetScratchpadRow(b._currentAttributes)

	scratch.CopyFrom(r)

	r.ReplaceText(state)
	r.ReplaceAttributes(state.columnBegin, state.columnEnd, attributes)

	// Restore trailing text from our backup in scratch.
	restoreState := msRowWriteState{
		text:        scratch.GetTextRange(state.columnBegin, state.columnLimit),
		columnBegin: state.columnEnd,
		columnLimit: state.columnLimit,
	}
	r.ReplaceText(&restoreState)
}

// TextBuffer::IncrementCircularBuffer
func (b *msTextBuffer) IncrementCircularBuffer(fillAttributes TextAttribute) {
	// Prune hyperlinks to delete obsolete references: recorded non-port.

	// Recorded read-only tap (not in the original; see the struct comment).
	if b.onRecycle != nil {
		b.onRecycle(b.GetRowByOffset(0))
	}

	// Second, clean out the old "first row" as it will become the "last row" of the buffer after the circle is performed.
	b.GetMutableRowByOffset(0).Reset(fillAttributes)
	{
		// Now proceed to increment.
		// Incrementing it will cause the next line down to become the new "top" of the window (the new "0" in logical coordinates)
		b._firstRow++

		// If we pass up the height of the buffer, loop back to 0.
		if b._firstRow >= b._height {
			b._firstRow = 0
		}
	}
}

// TextBuffer::CopyRow (used by ScrollRows)
func (b *msTextBuffer) CopyRow(srcRow, dstRow int, dst *msTextBuffer) {
	dst.GetMutableRowByOffset(dstRow).CopyFrom(b.GetRowByOffset(srcRow))
}

// TextBuffer::ScrollRows
func (b *msTextBuffer) ScrollRows(firstRow, size, delta int) {
	if delta == 0 {
		return
	}

	// Since the for() loop uses !=, we must ensure that size is positive.
	// A negative size doesn't make any sense anyways.
	if size < 0 {
		size = 0
	}

	var y, end, step int

	if delta < 0 {
		y = firstRow
		end = firstRow + size
		step = 1
	} else {
		y = firstRow + size - 1
		end = firstRow - 1
		step = -1
	}

	for ; y != end; y += step {
		b.CopyRow(y, y+delta, b)
	}
}

// TextBuffer::FillRect
func (b *msTextBuffer) FillRect(rect msRect, fill []uint16, attributes TextAttribute) {
	if rect.empty() || len(fill) == 0 {
		return
	}

	scratchpad := b.GetScratchpadRow(attributes)

	// The scratchpad row gets reset to whitespace by default, so there's no need to
	// initialize it again. Filling with whitespace is the most common operation by far.
	if len(fill) != 1 || fill[0] != ' ' {
		state := msRowWriteState{
			columnLimit: rect.right,
			columnEnd:   rect.left,
		}

		// Fill the scratchpad row with consecutive copies of "fill" up to the amount we need.
		for state.columnEnd < rect.right {
			state.columnBegin = state.columnEnd
			state.text = fill
			scratchpad.ReplaceText(&state)
		}
	}

	// Fill the given rows with copies of the scratchpad row.
	{
		state := msRowCopyTextFromState{
			source:            scratchpad,
			columnBegin:       rect.left,
			columnLimit:       rect.right,
			sourceColumnBegin: rect.left,
			sourceColumnLimit: 0x7fffffff,
		}

		for y := rect.top; y < rect.bottom; y++ {
			r := b.GetMutableRowByOffset(y)
			r.CopyTextFrom(&state)
			r.ReplaceAttributes(rect.left, rect.right, attributes)
		}
	}
}

// TextBuffer::_estimateOffsetOfLastCommittedRow: see the file header for
// the recorded arena difference; with a fully allocated slice the estimate
// is the last row of the buffer.
func (b *msTextBuffer) _estimateOffsetOfLastCommittedRow() int {
	return b._height - 1
}

// TextBuffer::GetLastNonSpaceCharacter (viewport = the whole buffer size,
// which is the GetSize() default of the original).
func (b *msTextBuffer) GetLastNonSpaceCharacter() msPoint {
	var coordEndOfText msPoint
	// Search the given viewport by starting at the bottom.
	coordEndOfText.y = b._height - 1
	if e := b._estimateOffsetOfLastCommittedRow(); e < coordEndOfText.y {
		coordEndOfText.y = e
	}

	currRow := b.GetRowByOffset(coordEndOfText.y)
	// The X position of the end of the valid text is the Right draw boundary (which is one beyond the final valid character)
	coordEndOfText.x = currRow.MeasureRight() - 1

	// If the X coordinate turns out to be -1, the row was empty, we need to search backwards for the real end of text.
	viewportTop := 0

	// while (this row is empty, and we're not at the top)
	for coordEndOfText.x < 0 && coordEndOfText.y > viewportTop {
		coordEndOfText.y--
		backupRow := b.GetRowByOffset(coordEndOfText.y)
		// We need to back up to the previous row if this line is empty, AND there are more rows
		coordEndOfText.x = backupRow.MeasureRight() - 1
	}

	// don't allow negative results
	if coordEndOfText.y < 0 {
		coordEndOfText.y = 0
	}
	if coordEndOfText.x < 0 {
		coordEndOfText.x = 0
	}

	return coordEndOfText
}
