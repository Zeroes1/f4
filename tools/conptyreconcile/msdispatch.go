// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of the used subset of microsoft/terminal
// src/terminal/adapter/adaptDispatch.cpp
// (commit 079d1cc423336c89c1e220701c94b320cecb603a):
// PrintString/_WriteToBuffer, _DoLineFeed, LineFeed, CarriageReturn,
// CursorUp/Down/Forward/Backward, CursorPosition/_CursorMovePosition +
// Offset, CursorHorizontalPositionAbsolute, VerticalLinePositionAbsolute,
// EraseInDisplay (_EraseAll, ToEnd, FromBeginning), EraseInLine,
// _FillRect, _ScrollRectVertically, _GetVerticalMargins,
// _GetHorizontalMargins.
//
// The Page model: this terminal is ConPTY's conhost, whose page is a
// viewport that pans over the circular buffer; here the buffer height is
// the console height, so page.BufferHeight() == page.Height() and the
// "pan down" branch of _DoLineFeed is unreachable exactly as it is in a
// conhost whose window equals its buffer -- the IncrementCircularBuffer
// branch is the one that runs. The page members are ported as accessors
// over that state, not re-derived.
//
// Recorded non-ports (see docs/CONPTY_RESEARCH.md, "THE RULE" ledger):
//   - DECSTBM/DECSLRM margin setters: no stream this tool parses carries
//     them; the margin *getters* are ported and report the unset state.
//   - Mode::Origin, Mode::InsertReplace, Mode::LineFeed toggles: the
//     mode tests are ported at their call sites; the setters (SM/RM
//     handlers) are not routed (msengine.go lists routed finals). The
//     defaults are the originals': Origin off, InsertReplace off
//     (replace), LineFeed off (LF does not imply CR), AutoWrap on.
//   - _EraseScrollback, alternate screen, attribute (SGR) semantics:
//     colors are the recorded non-port of msrow.go.
//   - The narrow-rect (cell walk) branch of _ScrollRectVertically: the
//     only caller shapes here are full-width, as in _DoLineFeed and
//     ReverseLineFeed without horizontal margins.

package main

var msWhitespace = []uint16{' '}

type msOffset struct {
	Value      int
	IsAbsolute bool
}

func msOffsetAbsolute(v int) msOffset  { return msOffset{Value: v, IsAbsolute: true} }
func msOffsetForward(v int) msOffset   { return msOffset{Value: v} }
func msOffsetBackward(v int) msOffset  { return msOffset{Value: -v} }
func msOffsetUnchanged() msOffset      { return msOffset{} }

type msEraseType int

const (
	msEraseToEnd         msEraseType = 0
	msEraseFromBeginning msEraseType = 1
	msEraseAllType       msEraseType = 2
	msEraseScrollback    msEraseType = 3
)

// msPage carries the Page members _WriteToBuffer and friends read.
type msPage struct {
	buffer *msTextBuffer
	cursor *msCursor
	top    int // page.Top(): the viewport top; 0 with buffer == window
}

func (p *msPage) Buffer() *msTextBuffer { return p.buffer }
func (p *msPage) Cursor() *msCursor     { return p.cursor }
func (p *msPage) Width() int            { return p.buffer.Width() }
func (p *msPage) Height() int           { return p.buffer.Height() }
func (p *msPage) BufferHeight() int     { return p.buffer.Height() }
func (p *msPage) Top() int              { return p.top }
func (p *msPage) Bottom() int           { return p.top + p.Height() }
func (p *msPage) Attributes() TextAttribute { return TextAttribute{} }
func (p *msPage) XPanOffset() int       { return 0 }
func (p *msPage) MoveViewportDown()     { p.top++ }

type msAdaptDispatch struct {
	page msPage

	// _modes (adaptDispatch.hpp): the ones the ported subset tests.
	modeOrigin        bool
	modeInsertReplace bool
	modeLineFeed      bool
	modeAutoWrap      bool

	// _tabStopColumns / _initDefaultTabStops (adaptDispatch.hpp)
	tabStopColumns         []bool
	initDefaultTabStopsOff bool
}

func newMsAdaptDispatch(width, height int) *msAdaptDispatch {
	return &msAdaptDispatch{
		page: msPage{
			buffer: newMsTextBuffer(width, height),
			cursor: &msCursor{},
		},
		modeAutoWrap: true,
	}
}

// AdaptDispatch::_GetVerticalMargins (absolute=true), with no DECSTBM set:
// "When no margins are set, the returned values will be the top and bottom
// of the page" -- ported to the unset state.
func (d *msAdaptDispatch) _GetVerticalMargins(page *msPage, absolute bool) (int, int) {
	return page.Top(), page.Bottom() - 1
}

// AdaptDispatch::_GetHorizontalMargins, DECSLRM unset.
func (d *msAdaptDispatch) _GetHorizontalMargins(pageWidth int) (int, int) {
	return 0, pageWidth - 1
}

// AdaptDispatch::_GetEraseAttributes: colors are the recorded non-port.
func (d *msAdaptDispatch) _GetEraseAttributes(page *msPage) TextAttribute {
	return TextAttribute{}
}

// AdaptDispatch::_api.SetViewportPosition
func (d *msAdaptDispatch) setViewportPosition(x, y int) { d.page.top = y }

// AdaptDispatch::PrintString
func (d *msAdaptDispatch) PrintString(str []uint16) {
	d._WriteToBuffer(str)
}

// AdaptDispatch::_WriteToBuffer
func (d *msAdaptDispatch) _WriteToBuffer(str []uint16) {
	page := &d.page
	textBuffer := page.Buffer()
	cursor := page.Cursor()
	cursorPosition := cursor.GetPosition()
	wrapAtEOL := d.modeAutoWrap
	attributes := page.Attributes()

	topMargin, bottomMargin := d._GetVerticalMargins(page, true)
	_, rightMargin := d._GetHorizontalMargins(page.Width())

	lineWidth := textBuffer.GetLineWidth(cursorPosition.y)
	if cursorPosition.x <= rightMargin && cursorPosition.y >= topMargin && cursorPosition.y <= bottomMargin {
		if m := rightMargin + 1; m < lineWidth {
			lineWidth = m
		}
	}

	state := msRowWriteState{
		text:        str,
		columnLimit: lineWidth,
	}

	for len(state.text) != 0 {
		if delayedCursorPosition := cursor.GetDelayEOLWrap(); delayedCursorPosition != nil && wrapAtEOL {
			cursor.ResetDelayEOLWrap()
			// Only act on a delayed EOL if we didn't move the cursor to a
			// different position from where the EOL was marked.
			if *delayedCursorPosition == cursorPosition {
				if d._DoLineFeed(page, true, true) {
					// If the line feed caused the viewport to move down, we
					// need to adjust the page viewport and margins to match.
					page.MoveViewportDown()
					topMargin, bottomMargin = d._GetVerticalMargins(page, true)
				}

				cursorPosition = cursor.GetPosition()
				// We need to recalculate the width when moving to a new line.
				lineWidth = textBuffer.GetLineWidth(cursorPosition.y)
				if cursorPosition.y >= topMargin && cursorPosition.y <= bottomMargin {
					if m := rightMargin + 1; m < lineWidth {
						lineWidth = m
					}
				}
				state.columnLimit = lineWidth
			}
		}

		state.columnBegin = cursorPosition.x

		textLenBefore := len(state.text)
		if d.modeInsertReplace {
			textBuffer.Insert(cursorPosition.y, attributes, &state)
		} else {
			textBuffer.Replace(cursorPosition.y, attributes, &state)
		}
		textLenAfter := len(state.text)

		// If we're past the end of the line, we need to clamp the cursor
		// back into range, and if wrapping is enabled, set the delayed wrap
		// flag. The wrapping only occurs once another character is output.
		isWrapping := state.columnEnd >= state.columnLimit
		if isWrapping {
			cursorPosition.x = state.columnLimit - 1
		} else {
			cursorPosition.x = state.columnEnd
		}
		cursor.SetPosition(cursorPosition)

		if isWrapping {
			// We want to wrap, but we failed to write even a single character into the row.
			// (see the original comment block for the two circumstances)
			if textLenBefore == textLenAfter && (state.columnBegin == 0 || !wrapAtEOL) {
				state.text = state.text[msGraphemeLen(state.text):]
			}

			if wrapAtEOL {
				cursor.DelayEOLWrap()
			}
		}
	}
}

// TextBuffer::GraphemeNext at offset 0, as _WriteToBuffer uses it: the
// length of the first grapheme cluster of the string.
func msGraphemeLen(str []uint16) int {
	var s msGraphemeState
	msGraphemeNext(&s, str)
	if s.len < 1 {
		return 1
	}
	return s.len
}

// AdaptDispatch::_DoLineFeed
func (d *msAdaptDispatch) _DoLineFeed(page *msPage, withReturn, wrapForced bool) bool {
	textBuffer := page.Buffer()
	pageWidth := page.Width()
	bufferHeight := page.BufferHeight()
	topMargin, bottomMargin := d._GetVerticalMargins(page, true)
	leftMargin, rightMargin := d._GetHorizontalMargins(pageWidth)
	viewportMoved := false

	cursor := page.Cursor()
	currentPosition := cursor.GetPosition()
	newPosition := currentPosition

	// If the line was forced to wrap, set the wrap status.
	// When explicitly moving down a row, clear the wrap status.
	textBuffer.GetMutableRowByOffset(currentPosition.y).SetWrapForced(wrapForced)

	// If a carriage return was requested, we move to the leftmost column or
	// the left margin, depending on whether we started within the margins.
	if withReturn {
		clampToMargin := currentPosition.y >= topMargin &&
			currentPosition.y <= bottomMargin &&
			currentPosition.x >= leftMargin
		if clampToMargin {
			newPosition.x = leftMargin
		} else {
			newPosition.x = 0
		}
	}

	if currentPosition.y != bottomMargin || newPosition.x < leftMargin || newPosition.x > rightMargin {
		// If we're not at the bottom margin, or outside the horizontal margins,
		// then there's no scrolling, so we make sure we don't move past the
		// bottom of the page.
		newPosition.y = currentPosition.y + 1
		if b := page.Bottom() - 1; newPosition.y > b {
			newPosition.y = b
		}
		newPosition = textBuffer.ClampPositionWithinLine(newPosition)
	} else if topMargin > page.Top() || leftMargin > 0 || rightMargin < pageWidth-1 {
		// If the top margin isn't at the top of the page, or the
		// horizontal margins are set, then we're just scrolling the margin
		// area and the cursor stays where it is.
		d._ScrollRectVertically(page, msRect{leftMargin, topMargin, rightMargin + 1, bottomMargin + 1}, -1)
	} else if page.Bottom() < bufferHeight {
		// If the top margin is at the top of the page, then we'll scroll
		// the content up by panning the viewport down, and also move the cursor
		// down a row. But we only do this if the viewport hasn't yet reached
		// the end of the buffer.
		d.setViewportPosition(page.XPanOffset(), page.Top()+1)
		newPosition.y++
		viewportMoved = true

		// And if the bottom margin didn't cover the full page, we copy the
		// lower part of the page down so it remains static. But for a full
		// pan we reset the newly revealed row with the erase attributes.
		if bottomMargin < page.Bottom()-1 {
			d._ScrollRectVertically(page, msRect{0, bottomMargin + 1, pageWidth, page.Bottom() + 1}, 1)
		} else {
			eraseAttributes := d._GetEraseAttributes(page)
			textBuffer.GetMutableRowByOffset(newPosition.y).Reset(eraseAttributes)
		}
	} else {
		// If the viewport has reached the end of the buffer, we can't pan down,
		// so we cycle the row coordinates, which effectively scrolls the buffer
		// content up. In this case we don't need to move the cursor down.
		eraseAttributes := d._GetEraseAttributes(page)
		textBuffer.IncrementCircularBuffer(eraseAttributes)

		// And again, if the bottom margin didn't cover the full page, we
		// copy the lower part of the page down so it remains static.
		if bottomMargin < page.Bottom()-1 {
			d._ScrollRectVertically(page, msRect{0, bottomMargin, pageWidth, bufferHeight}, 1)
		}
	}

	cursor.SetPosition(newPosition)
	return viewportMoved
}

// AdaptDispatch::LineFeed(DependsOnMode)
func (d *msAdaptDispatch) LineFeed() {
	d._DoLineFeed(&d.page, d.modeLineFeed, false)
}

// AdaptDispatch::CarriageReturn
func (d *msAdaptDispatch) CarriageReturn() {
	d._CursorMovePosition(msOffsetUnchanged(), msOffsetAbsolute(1), true)
}

// AdaptDispatch::CursorUp / CursorDown / CursorForward / CursorBackward
func (d *msAdaptDispatch) CursorUp(distance int) {
	d._CursorMovePosition(msOffsetBackward(distance), msOffsetUnchanged(), true)
}
func (d *msAdaptDispatch) CursorDown(distance int) {
	d._CursorMovePosition(msOffsetForward(distance), msOffsetUnchanged(), true)
}
func (d *msAdaptDispatch) CursorForward(distance int) {
	d._CursorMovePosition(msOffsetUnchanged(), msOffsetForward(distance), true)
}
func (d *msAdaptDispatch) CursorBackward(distance int) {
	d._CursorMovePosition(msOffsetUnchanged(), msOffsetBackward(distance), true)
}

// AdaptDispatch::CursorHorizontalPositionAbsolute (CHA) /
// VerticalLinePositionAbsolute (VPA)
func (d *msAdaptDispatch) CursorHorizontalPositionAbsolute(column int) {
	d._CursorMovePosition(msOffsetUnchanged(), msOffsetAbsolute(column), false)
}
func (d *msAdaptDispatch) VerticalLinePositionAbsolute(line int) {
	d._CursorMovePosition(msOffsetAbsolute(line), msOffsetUnchanged(), false)
}

// AdaptDispatch::CursorPosition
func (d *msAdaptDispatch) CursorPosition(line, column int) {
	d._CursorMovePosition(msOffsetAbsolute(line), msOffsetAbsolute(column), false)
}

// AdaptDispatch::_CursorMovePosition
func (d *msAdaptDispatch) _CursorMovePosition(rowOffset, colOffset msOffset, clampInMargins bool) {
	// First retrieve some information about the buffer
	page := &d.page
	cursor := page.Cursor()
	pageWidth := page.Width()
	cursorPosition := cursor.GetPosition()
	topMargin, bottomMargin := d._GetVerticalMargins(page, true)
	leftMargin, rightMargin := d._GetHorizontalMargins(pageWidth)

	// For relative movement, the given offsets will be relative to
	// the current cursor position.
	row := cursorPosition.y
	col := cursorPosition.x

	// But if the row is absolute, it will be relative to the top of the
	// page, or the top margin, depending on the origin mode.
	if rowOffset.IsAbsolute {
		if d.modeOrigin {
			row = topMargin
		} else {
			row = page.Top()
		}
	}

	// And if the column is absolute, it'll be relative to column 0,
	// or the left margin, depending on the origin mode.
	// Horizontal positions are not affected by the viewport.
	if colOffset.IsAbsolute {
		if d.modeOrigin {
			col = leftMargin
		} else {
			col = 0
		}
	}

	// Adjust the base position by the given offsets and clamp the results.
	// The row is constrained within the page's vertical boundaries,
	// while the column is constrained by the buffer width.
	row = msClamp(row+rowOffset.Value, page.Top(), page.Bottom()-1)
	col = msClamp(col+colOffset.Value, 0, pageWidth-1)

	// If the operation needs to be clamped inside the margins, or the origin
	// mode is relative (which always requires margin clamping), then the row
	// and column may need to be adjusted further.
	if clampInMargins || d.modeOrigin {
		if cursorPosition.x >= leftMargin && cursorPosition.x <= rightMargin {
			if cursorPosition.y >= topMargin {
				if row < topMargin {
					row = topMargin
				}
			}
			if cursorPosition.y <= bottomMargin {
				if row > bottomMargin {
					row = bottomMargin
				}
			}
		}
		if row >= topMargin && row <= bottomMargin {
			if cursorPosition.x >= leftMargin {
				if col < leftMargin {
					col = leftMargin
				}
			}
			if cursorPosition.x <= rightMargin {
				if col > rightMargin {
					col = rightMargin
				}
			}
		}
	}

	// Finally, attempt to set the adjusted cursor position back into the console.
	cursor.SetPosition(page.Buffer().ClampPositionWithinLine(msPoint{col, row}))
}

// AdaptDispatch::EraseInDisplay
func (d *msAdaptDispatch) EraseInDisplay(eraseType msEraseType) {
	if eraseType > msEraseScrollback {
		return
	}

	if eraseType == msEraseScrollback {
		// _EraseScrollback: recorded non-port (no stream here carries CSI 3 J).
		return
	} else if eraseType == msEraseAllType {
		d._EraseAll()
		return
	}

	page := &d.page
	textBuffer := page.Buffer()
	pageWidth := page.Width()
	row := page.Cursor().GetPosition().y
	col := page.Cursor().GetPosition().x

	// The ED control is expected to reset the delayed wrap flag.
	page.Cursor().ResetDelayEOLWrap()

	eraseAttributes := d._GetEraseAttributes(page)

	if eraseType == msEraseFromBeginning {
		d._FillRect(page, msRect{0, page.Top(), pageWidth, row}, msWhitespace, eraseAttributes)
		d._FillRect(page, msRect{0, row, col + 1, row + 1}, msWhitespace, eraseAttributes)
	}
	if eraseType == msEraseToEnd {
		d._FillRect(page, msRect{col, row, pageWidth, row + 1}, msWhitespace, eraseAttributes)
		d._FillRect(page, msRect{0, row + 1, pageWidth, page.Bottom()}, msWhitespace, eraseAttributes)
	}
	_ = textBuffer
}

// AdaptDispatch::EraseInLine
func (d *msAdaptDispatch) EraseInLine(eraseType msEraseType) {
	page := &d.page
	textBuffer := page.Buffer()
	row := page.Cursor().GetPosition().y
	col := page.Cursor().GetPosition().x

	// The EL control is expected to reset the delayed wrap flag.
	page.Cursor().ResetDelayEOLWrap()

	eraseAttributes := d._GetEraseAttributes(page)
	switch eraseType {
	case msEraseFromBeginning:
		d._FillRect(page, msRect{0, row, col + 1, row + 1}, msWhitespace, eraseAttributes)
	case msEraseToEnd:
		d._FillRect(page, msRect{col, row, textBuffer.GetLineWidth(row), row + 1}, msWhitespace, eraseAttributes)
	case msEraseAllType:
		d._FillRect(page, msRect{0, row, textBuffer.GetLineWidth(row), row + 1}, msWhitespace, eraseAttributes)
	}
}

// AdaptDispatch::_EraseAll
func (d *msAdaptDispatch) _EraseAll() {
	page := &d.page
	pageWidth := page.Width()
	pageHeight := page.Height()
	bufferHeight := page.BufferHeight()
	textBuffer := page.Buffer()

	// Stash away the current position of the cursor within the page.
	cursor := page.Cursor()
	row := cursor.GetPosition().y - page.Top()

	// Calculate new page position. Typically we want to move one line below
	// the last non-space row, but if the last non-space character is the very
	// start of the buffer, then we shouldn't move down at all.
	lastChar := textBuffer.GetLastNonSpaceCharacter()
	newPageTop := 0
	if lastChar != (msPoint{}) {
		newPageTop = lastChar.y + 1
	}
	newPageBottom := newPageTop + pageHeight
	delta := newPageBottom - bufferHeight
	if delta > 0 {
		for i := 0; i < delta; i++ {
			textBuffer.IncrementCircularBuffer(TextAttribute{})
		}
		newPageTop -= delta
		newPageBottom -= delta
	}
	// Move the viewport if necessary.
	if newPageTop != page.Top() {
		d.setViewportPosition(page.XPanOffset(), newPageTop)
	}
	// Restore the relative cursor position
	cursor.SetYPosition(row + newPageTop)

	// Erase all the rows in the current page.
	eraseAttributes := d._GetEraseAttributes(page)
	d._FillRect(page, msRect{0, newPageTop, pageWidth, newPageBottom}, msWhitespace, eraseAttributes)
}

// AdaptDispatch::_ScrollRectVertically, full-width path (see file header).
func (d *msAdaptDispatch) _ScrollRectVertically(page *msPage, scrollRect msRect, delta int) {
	textBuffer := page.Buffer()
	absoluteDelta := delta
	if absoluteDelta < 0 {
		absoluteDelta = -absoluteDelta
	}
	if h := scrollRect.height(); absoluteDelta > h {
		absoluteDelta = h
	}
	if absoluteDelta < scrollRect.height() {
		top := scrollRect.top
		if delta <= 0 {
			top = scrollRect.top + absoluteDelta
		}
		width := scrollRect.width()
		height := scrollRect.height() - absoluteDelta
		actualDelta := absoluteDelta
		if delta <= 0 {
			actualDelta = -absoluteDelta
		}
		if width == page.Width() {
			// If the scrollRect is the full width of the buffer, we can scroll
			// more efficiently by rotating the row storage.
			textBuffer.ScrollRows(top, height, actualDelta)
		}
		// The narrow-rect cell-walk branch: recorded non-port (file header).
	}

	// Rows revealed by the scroll are filled with standard erase attributes.
	eraseRect := scrollRect
	if delta > 0 {
		eraseRect.top = scrollRect.top
	} else {
		eraseRect.top = scrollRect.bottom - absoluteDelta
	}
	eraseRect.bottom = eraseRect.top + absoluteDelta
	eraseAttributes := d._GetEraseAttributes(page)
	d._FillRect(page, eraseRect, msWhitespace, eraseAttributes)
}

// AdaptDispatch::_FillRect
func (d *msAdaptDispatch) _FillRect(page *msPage, fillRect msRect, fillChar []uint16, fillAttrs TextAttribute) {
	page.Buffer().FillRect(fillRect, fillChar, fillAttrs)
}
