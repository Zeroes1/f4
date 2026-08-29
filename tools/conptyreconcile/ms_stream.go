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

// adjustCursorPosition is the pinned AdjustCursorPosition path with the
// SCREEN_INFORMATION-only services represented by textBuffer state. The
// viewport in this standalone buffer is the complete backing buffer; the
// source's render invalidation and accessibility notifications have no text
// or cursor effect and are intentionally not called here.
func adjustCursorPosition(buffer *textBuffer, pos coordinate) error {
	if buffer == nil {
		return fmt.Errorf("nil screen buffer")
	}
	inVtMode := buffer.vtMode
	// AdjustCursorPosition uses the backing buffer width. The VT legacy write
	// path selects a rendition width separately before calling it.
	lineWidth := buffer.width
	originalX := buffer.cursor.position.x
	if pos.x < 0 {
		if pos.y > 0 {
			pos.x = buffer.width + pos.x
			pos.y--
		} else {
			pos.x = 0
		}
	} else if pos.x >= lineWidth {
		if buffer.wrapAtEOL {
			pos.y += pos.x / lineWidth
			pos.x %= lineWidth
		} else {
			pos.x = buffer.width - 1
			if !inVtMode {
				// AdjustCursorPosition leaves a legacy cursor at the X
				// position held by the screen cursor when wrapping is off.
				pos.x = originalX
			}
		}
	}

	// This is the pinned margin-scroll branch. A cursor moved below the
	// bottom margin scrolls the margin contents up; a cursor moved above the
	// top margin scrolls them down. The source also applies the same upward
	// path to a VT reverse-line-feed above the viewport when no margins exist.
	top, bottom := 0, buffer.height-1
	marginsSet := buffer.marginsSet()
	if marginsSet {
		top, bottom = buffer.scrollTop, buffer.scrollBottom
	}
	currentY := buffer.cursor.position.y
	cursorInMargins := currentY >= top && currentY <= bottom
	if (!marginsSet && inVtMode && pos.y < 0) || (marginsSet && cursorInMargins && pos.y < top) {
		diff := pos.y - top
		count := -diff
		if count > 0 {
			buffer.scrollRegion(top, bottom, count, true)
			pos.y -= diff
		}
	}
	if marginsSet && cursorInMargins && pos.y > bottom {
		diff := pos.y - bottom
		if diff > 0 {
			buffer.scrollRegion(top, bottom, diff, false)
			pos.y -= diff
		}
	}
	if marginsSet && pos.y > buffer.height-1 {
		pos.y = buffer.height - 1
	}
	for pos.y >= buffer.height {
		if !buffer.incrementCircularBuffer() {
			return fmt.Errorf("circular buffer increment failed")
		}
		pos.y--
	}
	if pos.y < 0 {
		pos.y = 0
	}
	buffer.setCursor(pos)
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
				if err = adjustCursorPosition(buffer, position); err != nil {
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
			itEnd := buffer.write(outputCellsFromUTF16WithAttr(local, buffer.currentAttrs), buffer.cursor.position, nil)
			spaces += itEnd
			position = buffer.cursor.position
			position.x = xPosition
			if flags&wcDelayEOLWrap != 0 && position.x >= lineWidth && fWrapAtEOL {
				position.x = lineWidth - 1
				buffer.setCursor(position)
				buffer.cursor.delayed = true
				buffer.cursor.delayedAt = position
			} else if err = adjustCursorPosition(buffer, position); err != nil {
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
					if err = adjustCursorPosition(buffer, position); err != nil {
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
			if err = adjustCursorPosition(buffer, position); err != nil {
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
				if err = adjustCursorPosition(buffer, position); err != nil {
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
			if err = adjustCursorPosition(buffer, position); err != nil {
				return consumed, spaces, err
			}
		case 0x0d:
			consumed++
			position.x = 0
			position.y = buffer.cursor.position.y
			if err = adjustCursorPosition(buffer, position); err != nil {
				return consumed, spaces, err
			}
		case 0x0a:
			consumed++
			if buffer.returnOnNewline {
				position.x = 0
			}
			position.y = buffer.cursor.position.y + 1
			buffer.rowByOffset(buffer.cursor.position.y).wrapForced = false
			if err = adjustCursorPosition(buffer, position); err != nil {
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
				if err = adjustCursorPosition(buffer, position); err != nil {
					return consumed, spaces, err
				}
				continue
			}
			consumed++
			position.x++
			if err = adjustCursorPosition(buffer, position); err != nil {
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
	buffer.write([]outputCell{{glyph: []uint16{unicodeSpace}, attr: buffer.currentAttrs, behavior: attrValue}}, target, nil)
}

func writeSpacesAt(buffer *textBuffer, target coordinate, count int) {
	if count <= 0 {
		return
	}
	units := make([]uint16, count)
	for i := range units {
		units[i] = unicodeSpace
	}
	buffer.write(outputCellsFromUTF16WithAttr(units, buffer.currentAttrs), target, nil)
}

var bufferWidth = newWidthDetector()

func outputCellsFromUTF16(units []uint16) []outputCell {
	result := make([]outputCell, 0, len(units)*2)
	for offset := 0; offset < len(units); {
		glyph := utf16ParseNext(units[offset:])
		consumed := 1
		if len(glyph) == 2 && offset+1 < len(units) && glyph[0] == units[offset] && glyph[1] == units[offset+1] {
			consumed = 2
		} else if len(glyph) == 1 && glyph[0] != 0xfffd && glyph[0] == units[offset] {
			consumed = 1
		} else {
			glyph = []uint16{0xfffd}
		}
		if bufferWidth.IsWide(glyph) {
			result = append(result,
				outputCell{glyph: glyph, dbcs: dbcsAttribute{kind: dbcsLeading}, behavior: attrValue},
				outputCell{glyph: glyph, dbcs: dbcsAttribute{kind: dbcsTrailing}, behavior: attrValue},
			)
		} else {
			result = append(result, outputCell{glyph: glyph, behavior: attrValue})
		}
		offset += consumed
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
