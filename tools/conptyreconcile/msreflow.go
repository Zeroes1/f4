// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of TextBuffer::Reflow from microsoft/terminal
// src/buffer/out/textBuffer.cpp
// (commit 079d1cc423336c89c1e220701c94b320cecb603a),
// lastCharacterViewport == nullptr, positionInfo == nullptr.
//
// Recorded non-ports at their original call sites: ImageSlice::CopyRow,
// the attribute (color) copies, ScrollbarData, CopyProperties /
// CopyHyperlinkMaps -- all outside the text/wrap model (msrow.go header).

package main

const msCoordTypeMax = 0x7fffffff

func msTextBufferReflow(oldBuffer, newBuffer *msTextBuffer, oldCursor *msCursor, newCursor *msCursor) {
	oldCursorPos := oldCursor.GetPosition()
	var newCursorPos msPoint

	// BODGY: We use oldCursorPos in two critical places below:
	// * To compute an oldHeight that includes, at a minimum, the cursor row
	// * For REFLOW_JANK_CURSOR_WRAP (see comment below)
	oldCursorPos.x = msClamp(oldCursorPos.x, 0, oldBuffer._width-1)
	oldCursorPos.y = msClamp(oldCursorPos.y, 0, oldBuffer._height-1)

	lastRowWithText := oldBuffer.GetLastNonSpaceCharacter().y

	oldY := 0
	newY := 0
	newX := 0
	newWidth := newBuffer.Width()
	newYLimit := msCoordTypeMax

	oldHeight := lastRowWithText
	if oldCursorPos.y > oldHeight {
		oldHeight = oldCursorPos.y
	}
	oldHeight++
	newHeight := newBuffer.Height()

	// Copy oldBuffer into newBuffer until oldBuffer has been fully consumed.
	for ; oldY < oldHeight && newY < newYLimit; oldY++ {
		oldRow := oldBuffer.GetRowByOffset(oldY)

		// A pair of double height rows should optimally wrap as a union (i.e. after wrapping there should be 4 lines).
		// But for this initial implementation I chose the alternative approach: Just truncate them.
		if oldRow.GetLineRendition() != LineRenditionSingleWidth {
			// Since rows with a non-standard line rendition should be truncated it's important
			// that we pretend as if the previous row ended in a newline, even if it didn't.
			if newX != 0 {
				newX = 0
				newY++
			}

			newRow := newBuffer.GetMutableRowByOffset(newY)

			// See the comment marked with "REFLOW_RESET".
			if newY >= newHeight {
				newRow.Reset(TextAttribute{})
			}

			newRow.CopyFrom(oldRow)
			newRow.SetWrapForced(false)

			if oldY == oldCursorPos.y {
				newCursorPos = msPoint{newRow.AdjustToGlyphStart(oldCursorPos.x), newY}
			}

			newY++
			continue
		}

		// Rows don't store any information for what column the last written character is in.
		// We simply truncate all trailing whitespace in this implementation.
		oldRowLimit := oldRow.MeasureRight()
		if oldY == oldCursorPos.y {
			// REFLOW_JANK_CURSOR_WRAP:
			// Pretending as if there's always at least whitespace in front of the cursor has the benefit that
			// * the cursor retains its distance from any preceding text.
			// * when a client application starts writing on this new, empty line,
			//   enlarging the buffer unwraps the text onto the preceding line.
			if m := oldCursorPos.x + 1; m > oldRowLimit {
				oldRowLimit = m
			}
		}

		oldX := 0

		// Copy oldRow into newBuffer until oldRow has been fully consumed.
		// We use a do-while loop to ensure that line wrapping occurs and
		// that attributes are copied over even for seemingly empty rows.
		for {
			// This if condition handles line wrapping.
			// Only if we write past the last column we should wrap and as such this if
			// condition is in front of the text insertion code instead of behind it.
			// A SetWrapForced of false implies an explicit newline, which is the default.
			if newX >= newWidth {
				newBuffer.GetMutableRowByOffset(newY).SetWrapForced(true)
				newX = 0
				newY++
			}

			// REFLOW_RESET:
			// If we shrink the buffer vertically, for instance from 100 rows to 90 rows, we will write 10 rows in the
			// new buffer twice. We need to reset them before copying text, or otherwise we'll see the previous contents.
			if newY >= newHeight && newX == 0 {
				// We need to ensure not to overwrite the row containing the cursor.
				if newY >= newYLimit {
					break
				}
				newBuffer.GetMutableRowByOffset(newY).Reset(TextAttribute{})
			}

			newRow := newBuffer.GetMutableRowByOffset(newY)

			state := msRowCopyTextFromState{
				source:            oldRow,
				columnBegin:       newX,
				columnLimit:       msCoordTypeMax,
				sourceColumnBegin: oldX,
				sourceColumnLimit: oldRowLimit,
			}
			newRow.CopyTextFrom(&state)

			if oldY == oldCursorPos.y && oldCursorPos.x >= oldX {
				// In theory AdjustToGlyphStart ensures we don't put the cursor on a trailing wide glyph.
				newCursorPos = msPoint{newRow.AdjustToGlyphStart(oldCursorPos.x - oldX + newX), newY}
				// This implements the second option. There's no fundamental reason why this is better.
				newYLimit = newY + newHeight
			}

			oldX = state.sourceColumnEnd
			newX = state.columnEnd

			if !(oldX < oldRowLimit) {
				break
			}
		}

		// If the row had an explicit newline we also need to newline. :)
		if !oldRow.WasWrapForced() {
			newX = 0
			newY++
		}
	}

	// The for loop right after this if condition will copy entire rows of attributes at a time.
	if newX != 0 {
		newX = 0
		newY++
	}

	// (The attribute back-fill loop of the original operates on color runs
	// only; colors are the recorded non-port of msrow.go.)

	// Since we didn't use IncrementCircularBuffer() we need to compute the proper
	// _firstRow offset now, in a way that replicates IncrementCircularBuffer().
	// We need to do the same for newCursorPos.y for basically the same reason.
	if newY > newHeight {
		newBuffer._firstRow = newY % newHeight
		newCursorPos.y = (newCursorPos.y - newBuffer._firstRow + newHeight) % newHeight
	}

	newCursor.SetPosition(newCursorPos)
}
