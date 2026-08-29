package main

import (
	"fmt"
)

const (
	wcDestructiveBackspace = 0x01
	wcKeepCursorVisible    = 0x02
	wcPrintableControl     = 0x04
	wcLimitBackspace       = 0x10
	wcDelayEOLWrap         = 0x80
)

// adjustCursorPosition is AdjustCursorPosition from src/host/_stream.cpp.
// Render invalidation and accessibility notifications are not state-bearing
// operations in this text-only transcription; every buffer mutation and
// coordinate transition remains represented here.
func adjustCursorPosition(buffer *textBuffer, pos coordinate, keepCursorVisible bool) error {
	if buffer == nil {
		return fmt.Errorf("nil screen buffer")
	}
	inVtMode := buffer.vtMode
	bufferSizeX, bufferSizeY := buffer.width, buffer.height
	if pos.x < 0 {
		if pos.y > 0 {
			pos.x = bufferSizeX + pos.x
			pos.y--
		} else {
			pos.x = 0
		}
	} else if pos.x >= bufferSizeX {
		if buffer.wrapAtEOL {
			pos.y += pos.x / bufferSizeX
			pos.x %= bufferSizeX
		} else {
			if !inVtMode {
				pos.x = buffer.cursor.position.x
			} else {
				pos.x = bufferSizeX - 1
			}
		}
	}

	// The VT standard requires newly revealed rows to keep the current
	// background but lose meta attributes.
	fillAttributes := buffer.currentAttrs
	fillAttributes.setStandardErase()
	relativeTop := 0
	if buffer.marginsSet() {
		relativeTop = buffer.scrollTop
	}
	viewportTop := buffer.viewportTop
	viewportBottom := buffer.viewportBottom()
	marginsTop, marginsBottom := buffer.absoluteScrollMargins()
	marginsSet := marginsBottom > marginsTop
	currentCursor := buffer.cursor.position
	cursorInMargins := currentCursor.y >= marginsTop && currentCursor.y <= marginsBottom
	cursorAboveViewport := pos.y < 0 && inVtMode
	scrollUp := marginsSet && cursorInMargins && pos.y < marginsTop
	scrollUpWithoutMargins := !marginsSet && cursorAboveViewport
	if scrollUpWithoutMargins {
		scrollUp = true
		marginsTop = viewportTop
		marginsBottom = viewportBottom
	}
	scrollDown := marginsSet && cursorInMargins && pos.y > marginsBottom
	scrollDownAtTop := scrollDown && relativeTop == 0

	if scrollDownAtTop {
		delta := pos.y - marginsBottom
		scrollTop := marginsBottom + 1
		moveToY := scrollTop + delta
		newViewTop := viewportTop + delta
		newRows := (viewportBottom + 1 + delta) - bufferSizeY
		for i := 0; i < newRows; i++ {
			if !buffer.incrementCircularBuffer() {
				return fmt.Errorf("circular buffer increment failed")
			}
			moveToY--
			newViewTop--
			scrollTop--
		}
		_ = moveToY // The target origin is consumed by the clipped scroll below.
		if scrollTop <= bufferSizeY-1 {
			if delta > 0 {
				buffer.scrollRegionWithAttr(scrollTop, bufferSizeY-1, delta, true, fillAttributes)
			}
		}
		if err := buffer.setViewportOrigin(true, coordinate{y: newViewTop}, true); err != nil {
			return err
		}
		viewportTop = buffer.viewportTop
		viewportBottom = buffer.viewportBottom()
		if newRows > 0 {
			currentCursor.y -= newRows
			pos.y -= newRows
		}
		marginsTop, marginsBottom = buffer.absoluteScrollMargins()
	}

	if scrollUp || (scrollDown && !scrollDownAtTop) {
		boundary := marginsTop
		if !scrollUp {
			boundary = marginsBottom
		}
		diff := pos.y - boundary
		if diff != 0 {
			top, bottom := marginsTop, marginsBottom
			if top < 0 {
				top = 0
			}
			if bottom >= bufferSizeY {
				bottom = bufferSizeY - 1
			}
			if top <= bottom {
				if diff > 0 {
					buffer.scrollRegionWithAttr(top, bottom, diff, false, fillAttributes)
				} else {
					buffer.scrollRegionWithAttr(top, bottom, -diff, true, fillAttributes)
				}
			}
			pos.y -= diff
		}
	}

	if marginsSet && pos.y > viewportBottom {
		pos.y = viewportBottom
	}

	if pos.y >= bufferSizeY {
		if pos.y != bufferSizeY {
			return fmt.Errorf("cursor moved beyond buffer by %d rows", pos.y-bufferSizeY)
		}
		if !buffer.incrementCircularBuffer(buffer.vtMode) {
			return fmt.Errorf("circular buffer increment failed")
		}
		pos.y += bufferSizeY - pos.y - 1
	}

	cursorMovedPastViewport := pos.y > buffer.viewportBottom()
	cursorMovedPastVirtualViewport := pos.y > buffer.virtualViewportBottom()
	if cursorMovedPastViewport {
		if err := buffer.setViewportOrigin(false, coordinate{y: pos.y - buffer.viewportBottom()}, true); err != nil {
			return err
		}
	}
	if keepCursorVisible {
		buffer.makeCursorVisible(pos, true)
	}
	buffer.setCursor(pos)
	if inVtMode && cursorMovedPastViewport && cursorMovedPastVirtualViewport {
		buffer.initializeCursorRowAttributes()
	}
	return nil
}

// writeCharsLegacy follows the source's 1024-code-unit fast path and its
// processed-control boundaries.  It is kept separate from the VT parser just
// as _stream.cpp is separate from the parser/adapter path.
func writeCharsLegacy(buffer *textBuffer, input []uint16, flags uint32) (consumed, spaces int, err error) {
	if buffer == nil {
		return 0, 0, fmt.Errorf("nil screen buffer")
	}
	const localBufferSize = 1024
	const printableControlChars = uint32(wcPrintableControl)
	fUnprocessed := !buffer.processedOutput
	fWrapAtEOL := buffer.wrapAtEOL
	position := buffer.cursor.position
	originalXPosition := position.x
	lineWidth := buffer.width
	if buffer.vtMode {
		lineWidth = buffer.lineWidth(position.y)
	}

	for consumed < len(input) {
		if buffer.cursor.delayed && fWrapAtEOL {
			delayedAt := buffer.cursor.delayedAt
			buffer.cursor.delayed = false
			if delayedAt == position {
				position.x = 0
				position.y++
				if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
					return consumed, spaces, err
				}
				position = buffer.cursor.position
				if buffer.vtMode {
					lineWidth = buffer.lineWidth(position.y)
				}
			}
		}

		xPosition := buffer.cursor.position.x
		local := make([]uint16, 0, localBufferSize)
		for consumed < len(input) && len(local) < localBufferSize && xPosition < lineWidth {
			char := input[consumed]
			if isGlyphChar(char) || fUnprocessed {
				if isGlyphFullWidth(char) {
					if len(local) < localBufferSize-1 && xPosition < lineWidth-1 {
						local = append(local, char)
						xPosition += 2
						consumed++
					} else {
						break
					}
				} else {
					local = append(local, char)
					xPosition++
					consumed++
				}
				continue
			}

			switch char {
			case 0x07:
				if flags&printableControlChars != 0 {
					if len(local) >= localBufferSize-1 {
						goto endWhile
					}
					local = append(local, '^', char+'@')
					xPosition += 2
					consumed++
					break
				}
				consumed++
			case 0x08:
				goto endWhile
			case 0x09:
				tabSize := numberOfSpacesInTab(xPosition)
				xPosition += tabSize
				if xPosition >= lineWidth {
					goto endWhile
				}
				for j := 0; j < tabSize && len(local) < localBufferSize; j++ {
					local = append(local, unicodeSpace)
				}
				consumed++
			case 0x0a, 0x0d:
				goto endWhile
			default:
				if flags&printableControlChars != 0 && isControlChar(char) {
					if len(local) >= localBufferSize-1 {
						goto endWhile
					}
					local = append(local, '^', char+'@')
					xPosition += 2
					consumed++
				} else {
					if char == 0 {
						local = append(local, unicodeSpace)
					} else {
						// The pinned default branch calls GetStringTypeW and,
						// for C1 controls, ConvertOutputToUnicode.  The probe
						// has no non-UTF-8 output code page, so a C1 unit is
						// retained exactly at this conversion boundary.
						local = append(local, char)
					}
					xPosition++
					consumed++
				}
			}
		}

	endWhile:
		if len(local) != 0 {
			available := lineWidth - buffer.cursor.position.x
			if len(local) > available {
				local = local[:available]
			}
			_, cellDistance := buffer.write(outputCellsFromUTF16WithAttr(local, buffer.currentAttrs), buffer.cursor.position, nil)
			spaces += cellDistance
			position = buffer.cursor.position
			position.x = xPosition
			if flags&wcDelayEOLWrap != 0 && position.x >= lineWidth && fWrapAtEOL {
				position.x = lineWidth - 1
				buffer.setCursor(position)
				buffer.cursor.delayed = true
				buffer.cursor.delayedAt = position
			} else if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
				return consumed, spaces, err
			}
			continue
		}
		if consumed >= len(input) {
			break
		}

		switch input[consumed] {
		case 0x08:
			// This is the source's exceptional path after a fast run.  It
			// reconstructs the already-consumed input to find the cell
			// distance of the character being erased.
			spaces--
			if consumed == 0 {
				position.x--
			} else {
				last := lastNonBackspace(input[:consumed])
				switch {
				case last == 0x09:
					position.x -= retrieveNumberOfSpaces(originalXPosition, input[:consumed], len(input[:consumed])-1)
					if position.x < 0 {
						position.x = ((lineWidth - 1) / 8) * 8
						position.x++
						position.y--
						if position.y >= 0 {
							buffer.rowByOffset(position.y).wrapForced = false
						}
					}
				case isControlChar(last):
					position.x--
					spaces--
					if flags&wcDestructiveBackspace != 0 {
						writeSpaceAt(buffer, position)
					}
					position.x--
				case isGlyphFullWidth(last):
					position.x--
					spaces--
					if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
						return consumed, spaces, err
					}
					if flags&wcDestructiveBackspace != 0 {
						writeSpaceAt(buffer, buffer.cursor.position)
					}
					position.x--
				default:
					position.x--
				}
			}
			if flags&wcLimitBackspace != 0 && position.x < 0 {
				position.x = 0
			}
			consumed++
			if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
				return consumed, spaces, err
			}
			if flags&wcDestructiveBackspace != 0 {
				writeSpaceAt(buffer, buffer.cursor.position)
			}
			if buffer.cursor.position.x == 0 && fWrapAtEOL && consumed > 0 && checkBisectProcess(buffer, input[:consumed], lineWidth-originalXPosition, originalXPosition, flags&printableControlChars != 0) {
				position.x = lineWidth - 1
				position.y = buffer.cursor.position.y - 1
				if position.y >= 0 {
					buffer.rowByOffset(position.y).wrapForced = false
				}
				if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
					return consumed, spaces, err
				}
			}
		case 0x09:
			tabSize := numberOfSpacesInTab(buffer.cursor.position.x)
			position.x = buffer.cursor.position.x + tabSize
			consumed++
			spaces += tabSize
			numChars := tabSize
			if position.x >= lineWidth {
				numChars = lineWidth - buffer.cursor.position.x
				position.x = 0
				position.y = buffer.cursor.position.y + 1
				buffer.rowByOffset(buffer.cursor.position.y).wrapForced = true
			}
			if numChars > 0 {
				writeSpacesAt(buffer, buffer.cursor.position, numChars)
			}
			if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
				return consumed, spaces, err
			}
		case 0x0d:
			consumed++
			position.x = 0
			position.y = buffer.cursor.position.y
			if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
				return consumed, spaces, err
			}
		case 0x0a:
			consumed++
			if buffer.returnOnNewline {
				position.x = 0
			}
			position.y = buffer.cursor.position.y + 1
			buffer.rowByOffset(buffer.cursor.position.y).wrapForced = false
			if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
				return consumed, spaces, err
			}
		default:
			// The pinned default case is only reached after the fast path
			// has stopped on a full-width character at the right edge.  Its
			// body clears a trailing DBCS cell, marks forced wrap/padding,
			// and retries the same input character on the next row.
			char := input[consumed]
			if char >= unicodeSpace && isGlyphFullWidth(char) && position.x >= lineWidth-1 && fWrapAtEOL {
				target := buffer.cursor.position
				if target.x >= 0 && target.x < buffer.width && buffer.rowByOffset(target.y).charRow.data[target.x].attr.isTrailing() {
					writeSpacesAt(buffer, coordinate{x: target.x - 1, y: target.y}, 2)
				}
				position.x = 0
				position.y = target.y + 1
				buffer.rowByOffset(target.y).wrapForced = true
				buffer.rowByOffset(target.y).doubleBytePadded = true
				if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
					return consumed, spaces, err
				}
				continue
			}
			consumed++
			position.x++
			if err = adjustCursorPosition(buffer, position, flags&wcKeepCursorVisible != 0); err != nil {
				return consumed, spaces, err
			}
		}
	}
	return consumed, spaces, nil
}

func isGlyphChar(unit uint16) bool { return unit >= unicodeSpace && unit != 0x7f }

func isControlChar(unit uint16) bool { return unit < unicodeSpace }

func isGlyphFullWidth(unit uint16) bool { return bufferWidth.IsWide([]uint16{unit}) }

func numberOfSpacesInTab(position int) int { return 8 - (position & 7) }

func lastNonBackspace(input []uint16) uint16 {
	logical := make([]uint16, 0, len(input))
	for _, unit := range input {
		if unit == 0x08 {
			if len(logical) > 0 {
				logical = logical[:len(logical)-1]
			}
		} else {
			logical = append(logical, unit)
		}
	}
	if len(logical) == 0 {
		return unicodeSpace
	}
	return logical[len(logical)-1]
}

func retrieveNumberOfSpaces(originalX int, input []uint16, current int) int {
	if current < 0 || current >= len(input) {
		return 0
	}
	char := input[current]
	if char == 0x09 {
		spaces := 0
		x := originalX
		for i := 0; i <= current; i++ {
			char = input[i]
			switch {
			case char == 0x09:
				spaces = numberOfSpacesInTab(x)
			case isControlChar(char):
				spaces = 2
			case isGlyphFullWidth(char):
				spaces = 2
			default:
				spaces = 1
			}
			x += spaces
		}
		return spaces
	}
	if isControlChar(char) || isGlyphFullWidth(char) {
		return 2
	}
	return 1
}

func checkBisectProcess(buffer *textBuffer, input []uint16, cBytes, originalX int, printableControlChars bool) bool {
	if !buffer.processedOutput {
		return false
	}
	words := len(input)
	for words > 0 && cBytes > 0 {
		char := input[len(input)-words]
		if char >= unicodeSpace {
			if isGlyphFullWidth(char) {
				if cBytes < 2 {
					return true
				}
				words--
				cBytes -= 2
				originalX += 2
			} else {
				words--
				cBytes--
				originalX++
			}
			continue
		}
		words--
		switch char {
		case 0x07:
			if printableControlChars {
				if cBytes < 2 {
					return true
				}
				cBytes -= 2
				originalX += 2
			}
		case 0x08, 0x0a, 0x0d:
		case 0x09:
			tabSize := numberOfSpacesInTab(originalX)
			originalX += tabSize
			if cBytes < tabSize {
				return true
			}
			cBytes -= tabSize
		default:
			if printableControlChars {
				if cBytes < 2 {
					return true
				}
				cBytes -= 2
				originalX += 2
			} else {
				cBytes--
				originalX++
			}
		}
	}
	return false
}

func writeSpaceAt(buffer *textBuffer, target coordinate) {
	if target.x < 0 || target.y < 0 || target.x >= buffer.width || target.y >= buffer.height {
		return
	}
	_, _ = buffer.write([]outputCell{{glyph: []uint16{unicodeSpace}, attr: buffer.currentAttrs, behavior: attrStored}}, target, nil)
}

func writeSpacesAt(buffer *textBuffer, target coordinate, count int) {
	if count <= 0 {
		return
	}
	units := make([]uint16, count)
	for i := range units {
		units[i] = unicodeSpace
	}
	_, _ = buffer.write(outputCellsFromUTF16WithAttr(units, buffer.currentAttrs), target, nil)
}

var bufferWidth = newWidthDetector()

func outputCellsFromUTF16(units []uint16) []outputCell {
	result := make([]outputCell, 0, len(units)*2)
	// OutputCellIterator::operator++ advances by the current view's UTF-16
	// length, not by the number of malformed units skipped by ParseNext.  A
	// leading view is then exposed a second time as its trailing half without
	// advancing the source position. Preserve that exact iterator behavior.
	for offset := 0; offset < len(units); {
		glyph := utf16ParseNext(units[offset:])
		if bufferWidth.IsWide(glyph) {
			result = append(result,
				outputCell{glyph: glyph, dbcs: dbcsAttribute{kind: dbcsLeading}, behavior: attrStored},
				outputCell{glyph: glyph, dbcs: dbcsAttribute{kind: dbcsTrailing}, behavior: attrStored},
			)
		} else {
			result = append(result, outputCell{glyph: glyph, behavior: attrStored})
		}
		offset += len(glyph)
	}
	return result
}

func outputCellsFromUTF16WithAttr(units []uint16, attr textAttribute) []outputCell {
	cells := outputCellsFromUTF16(units)
	for i := range cells {
		cells[i].attr = attr
	}
	return cells
}

func writeDefaultString(buffer *textBuffer, input []uint16) error {
	_, _, err := writeCharsLegacy(buffer, input, wcLimitBackspace|wcDelayEOLWrap)
	return err
}
