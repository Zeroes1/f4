// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of the text buffer of microsoft/terminal at the pinned tag
// v1.12.10982.0 (docs/PINNED_CONSOLE.md): src/buffer/out/CharRowCell.{hpp,cpp},
// CharRow.{hpp,cpp}, Row.{hpp,cpp} and the write path of textBuffer.cpp
// (InsertCharacter, _SetWrapOnCurrentRow, _AdjustWrapOnCurrentRow,
// IncrementCursor, NewlineCursor, IncrementCircularBuffer, GetLineWidth).
//
// This REPLACES the model in msrow.go / mstextbuffer.go, which was ported from
// today's `main` and describes a buffer that exists in no shipping console:
// between v1.12 and `main` Microsoft replaced the per-cell CharRow with a ROW
// carrying a _charOffsets index and a WriteHelper. The pinned host has the
// model below -- a flat array of cells, each a glyph plus a DBCS attribute --
// and the wrap flag lives on the ROW, set from exactly one place.
//
// The merge that this whole project exists to undo (P13, §17) is visible here
// in four lines: IncrementCursor advances the cursor, and when it passes the
// final column it calls _SetWrapOnCurrentRow() and then NewlineCursor(). The
// flag is set on the row the cursor was on. Nothing else in the write path
// sets it, and an explicit newline (AdjustCursorPosition in _stream.cpp) is
// what clears it. That is the whole rule, and it is not a rule anyone
// inferred -- it is four lines of Microsoft's source.
//
// Mechanical transformations required by Go, and nothing else:
//   - boost::container::small_vector<CharRowCell, 120> -> a Go slice
//   - the glyph is a std::wstring_view of one or more UTF-16 units in the
//     original (CharRowCell holds a wchar_t plus an overflow indirection for
//     surrogate pairs); here it is a string, which carries the same content
//   - the TextAttribute / AttrRow colour machinery is a recorded non-port, as
//     in the rest of this tool: colours take no part in line structure
//   - exceptions and bool success returns collapse into plain statements
//
// Recorded non-ports: AttrRow (colours), hyperlinks, ImageSlice (absent in
// this version anyway), the double-byte assertion helpers
// _AssertValidDoubleByteSequence / _PrepareForDoubleByteSequence (the probe
// never writes a trailing byte without its leader, which is the only case
// they repair), and the renderer notifications.

package main

import (
	"strings"
	"unicode/utf16"
)

// DbcsAttribute (DbcsAttribute.hpp): which half of a wide glyph a cell holds.
type dbcsAttribute int

const (
	dbcsSingle dbcsAttribute = iota
	dbcsLeading
	dbcsTrailing
)

// CharRowCell: one cell, a glyph and its DBCS attribute.
type charRowCell struct {
	wch  string
	attr dbcsAttribute
}

// CharRowCell::Reset / the UNICODE_SPACE default of _wch.
func (c *charRowCell) Reset() {
	c.wch = " "
	c.attr = dbcsSingle
}

// CharRowCell::IsSpace
func (c *charRowCell) IsSpace() bool { return c.wch == " " }

// CharRow
type charRow struct {
	data []charRowCell
}

func newCharRow(width int) *charRow {
	r := &charRow{data: make([]charRowCell, width)}
	r.Reset()
	return r
}

// CharRow::Reset
func (r *charRow) Reset() {
	for i := range r.data {
		r.data[i].Reset()
	}
}

// CharRow::MeasureRight: walk back over trailing spaces.
func (r *charRow) MeasureRight() int {
	it := len(r.data)
	for it > 0 && r.data[it-1].IsSpace() {
		it--
	}
	return it
}

// CharRow::GlyphAt / DbcsAttrAt / ClearCell
func (r *charRow) GlyphAt(column int) string           { return r.data[column].wch }
func (r *charRow) SetGlyphAt(column int, s string)     { r.data[column].wch = s }
func (r *charRow) DbcsAttrAt(column int) dbcsAttribute { return r.data[column].attr }
func (r *charRow) SetDbcsAttrAt(column int, a dbcsAttribute) {
	r.data[column].attr = a
}
func (r *charRow) ClearCell(column int) { r.data[column].Reset() }
func (r *charRow) size() int            { return len(r.data) }

// ROW (Row.hpp), text and wrap members.
type v12Row struct {
	charRow       *charRow
	wrapForced    bool
	doubleBytePad bool
	lineRendition LineRendition
}

func newV12Row(width int) *v12Row {
	return &v12Row{charRow: newCharRow(width)}
}

// ROW::Reset
func (r *v12Row) Reset() {
	r.charRow.Reset()
	r.wrapForced = false
	r.doubleBytePad = false
	r.lineRendition = LineRenditionSingleWidth
}

func (r *v12Row) GetCharRow() *charRow            { return r.charRow }
func (r *v12Row) SetWrapForced(w bool)            { r.wrapForced = w }
func (r *v12Row) WasWrapForced() bool             { return r.wrapForced }
func (r *v12Row) SetDoubleBytePadded(p bool)      { r.doubleBytePad = p }
func (r *v12Row) WasDoubleBytePadded() bool       { return r.doubleBytePad }
func (r *v12Row) GetLineRendition() LineRendition { return r.lineRendition }

// The row's text up to MeasureRight, with the trailing halves of wide glyphs
// dropped -- the reading the renderer does when it paints a line.
func (r *v12Row) text() string {
	var sb strings.Builder
	n := r.charRow.MeasureRight()
	for i := 0; i < n; i++ {
		if r.charRow.DbcsAttrAt(i) == dbcsTrailing {
			continue
		}
		sb.WriteString(r.charRow.GlyphAt(i))
	}
	return sb.String()
}

// TextBuffer, write path.
type v12TextBuffer struct {
	storage  []*v12Row
	firstRow int
	width    int
	height   int
	cursor   *msCursor

	// onRecycle is not in the original: the read-only tap the mirror needs,
	// because conhost keeps no scrollback (P16). Recorded in msengine.go.
	onRecycle func(*v12Row)
}

func newV12TextBuffer(width, height int) *v12TextBuffer {
	b := &v12TextBuffer{width: width, height: height, cursor: &msCursor{}}
	b.storage = make([]*v12Row, height)
	for i := range b.storage {
		b.storage[i] = newV12Row(width)
	}
	return b
}

// TextBuffer::GetRowByOffset
func (b *v12TextBuffer) GetRowByOffset(index int) *v12Row {
	offset := (b.firstRow + index) % b.height
	if offset < 0 {
		offset += b.height
	}
	return b.storage[offset]
}

// TextBuffer::GetLineWidth
func (b *v12TextBuffer) GetLineWidth(row int) int {
	scale := 0
	if b.GetRowByOffset(row).GetLineRendition() != LineRenditionSingleWidth {
		scale = 1
	}
	return b.width >> scale
}

// TextBuffer::_SetWrapOnCurrentRow / _AdjustWrapOnCurrentRow
func (b *v12TextBuffer) setWrapOnCurrentRow() { b.adjustWrapOnCurrentRow(true) }
func (b *v12TextBuffer) adjustWrapOnCurrentRow(set bool) {
	// "The vertical position of the cursor represents the current row we're
	// manipulating."
	b.GetRowByOffset(b.cursor.GetPosition().y).SetWrapForced(set)
}

// TextBuffer::IncrementCircularBuffer
func (b *v12TextBuffer) IncrementCircularBuffer() {
	if b.onRecycle != nil {
		b.onRecycle(b.GetRowByOffset(0))
	}
	b.GetRowByOffset(0).Reset()
	b.firstRow++
	if b.firstRow >= b.height {
		b.firstRow = 0
	}
}

// TextBuffer::NewlineCursor
func (b *v12TextBuffer) NewlineCursor() {
	finalRowIndex := b.height - 1

	// Reset the cursor position to 0 and move down one line
	p := b.cursor.GetPosition()
	p.x = 0
	p.y++
	b.cursor.SetPosition(p)

	// If we've passed the final valid row...
	if b.cursor.GetPosition().y > finalRowIndex {
		// Stay on the final logical/offset row of the buffer.
		b.cursor.SetYPosition(finalRowIndex)
		// Instead increment the circular buffer.
		b.IncrementCircularBuffer()
	}
}

// TextBuffer::IncrementCursor
func (b *v12TextBuffer) IncrementCursor() {
	finalColumnIndex := b.GetLineWidth(b.cursor.GetPosition().y) - 1

	// Move the cursor one position to the right
	p := b.cursor.GetPosition()
	p.x++
	b.cursor.SetPosition(p)

	// If we've passed the final valid column...
	if b.cursor.GetPosition().x > finalColumnIndex {
		// Then mark that we've been forced to wrap
		b.setWrapOnCurrentRow()
		// Then move the cursor to a new line
		b.NewlineCursor()
	}
}

// TextBuffer::InsertCharacter
func (b *v12TextBuffer) InsertCharacter(chars string, attr dbcsAttribute) {
	// Get the current cursor position
	iRow := b.cursor.GetPosition().y
	iCol := b.cursor.GetPosition().x

	// Get the row associated with the given logical position
	row := b.GetRowByOffset(iRow)
	cr := row.GetCharRow()

	// Store character and double byte data
	cr.SetGlyphAt(iCol, chars)
	cr.SetDbcsAttrAt(iCol, attr)

	// Advance the cursor
	b.IncrementCursor()
}

// The width in cells of one grapheme cluster, measured by the ported
// CodepointWidthDetector (mscwd.go) -- the same measurement conhost makes.
func v12GlyphWidth(s string) int {
	str := utf16.Encode([]rune(s))
	var st msGraphemeState
	msGraphemeNext(&st, str)
	if st.width < 1 {
		return 1
	}
	return st.width
}

// outputCell is the OutputCellIterator element the write path consumes:
// a glyph and its DBCS attribute. Colours are the recorded non-port.
type outputCell struct {
	chars string
	dbcs  dbcsAttribute
}

// ROW::WriteCells. `wrap` is std::optional<bool>: nil means "don't change the
// wrap value", true "we're filling cells as a stream, consider this a wrap",
// false "we're filling cells as a block, unwrap".
func (r *v12Row) WriteCells(cells []outputCell, index int, wrap *bool, limitRight *int) int {
	finalColumnInRow := r.charRow.size() - 1
	if limitRight != nil {
		finalColumnInRow = *limitRight
	}

	it := 0
	currentIndex := index

	for it < len(cells) && currentIndex <= finalColumnInRow {
		fillingLastColumn := currentIndex == finalColumnInRow

		// If we're trying to fill the first cell with a trailing byte, pad it
		// out instead by clearing it. Don't increment iterator.
		if currentIndex == 0 && cells[it].dbcs == dbcsTrailing {
			r.charRow.ClearCell(currentIndex)
		} else if fillingLastColumn && cells[it].dbcs == dbcsLeading {
			// If we're trying to fill the last cell with a leading byte, pad it
			// out instead by clearing it.
			r.charRow.ClearCell(currentIndex)
			r.SetDoubleBytePadded(true)
		} else {
			r.charRow.SetDbcsAttrAt(currentIndex, cells[it].dbcs)
			r.charRow.SetGlyphAt(currentIndex, cells[it].chars)
			it++
		}

		// If we're asked to (un)set the wrap status and we just filled the last
		// column with some text...
		if wrap != nil && fillingLastColumn {
			r.SetWrapForced(*wrap)
		}

		currentIndex++
	}

	return it
}

// TextBuffer::WriteLine
func (b *v12TextBuffer) WriteLine(cells []outputCell, target msPoint, wrap *bool) int {
	if target.y < 0 || target.y >= b.height || target.x < 0 || target.x >= b.width {
		return 0
	}
	row := b.GetRowByOffset(target.y)
	return row.WriteCells(cells, target.x, wrap, nil)
}

// TextBuffer::Write
func (b *v12TextBuffer) Write(cells []outputCell, target msPoint, wrap *bool) int {
	consumed := 0
	lineTarget := target
	for consumed < len(cells) && lineTarget.y >= 0 && lineTarget.y < b.height &&
		lineTarget.x >= 0 && lineTarget.x < b.width {
		n := b.WriteLine(cells[consumed:], lineTarget, wrap)
		if n == 0 {
			break
		}
		consumed += n
		lineTarget.x = 0
		lineTarget.y++
	}
	return consumed
}
