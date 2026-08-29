// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of WriteCharsLegacy and AdjustCursorPosition from
// microsoft/terminal src/host/_stream.cpp at the pinned tag v1.12.10982.0
// (docs/PINNED_CONSOLE.md).
//
// This REPLACES msstream.go, which ported the same function from today's
// `main`. The two are not variations on one algorithm. In `main` the text is
// handed to a RowWriteState and ROW::ReplaceText decides where it lands; here
// the characters are accumulated into a local buffer while XPosition is
// tracked by hand, the buffer is written with TextBuffer::Write, and the
// cursor is then moved to the *estimated* XPosition. The wrap flag is set in
// two different places for two different reasons -- ROW::WriteCells when a
// write fills the last column of a row, and TextBuffer::IncrementCursor when
// the cursor walks past it -- and cleared by AdjustCursorPosition's newline
// handling. The merge behaviour of P13/§17 is the sum of those three.
//
// Mechanical transformations required by Go, and nothing else:
//   - the `goto EndWhile` that ends the accumulation loop becomes a labelled
//     break; the control flow is identical, since every goto in that loop
//     jumps to the same label at the loop's end
//   - pointer walking (lpString/pwchBuffer/pwchRealUnicode, which the source
//     itself notes move in lockstep) becomes one index
//   - NTSTATUS returns become plain returns: no allocation can fail here
//
// Recorded non-ports, at their original call sites: WC_PRINTABLE_CONTROL_CHARS
// (the CtrlChar label; the probe's streams carry no control characters that
// would be printed as ^X), the OEM glyph conversion for C1 controls
// (GetStringTypeW / ConvertOutputToUnicode), the backspace-rewind path that
// re-scans the caller's backup buffer (COOKED_READ_DATA's editing, which a
// ConPTY child never exercises), accessibility eventing, and colours.

package main

import (
	"unicode/utf16"
)

// The WC_* flags of _stream.h that this path reads.
type wcFlags struct {
	delayEolWrap bool // WC_DELAY_EOL_WRAP
}

// v12Screen is the used subset of SCREEN_INFORMATION.
type v12Screen struct {
	buffer *v12TextBuffer

	wrapAtEOL      bool // ENABLE_WRAP_AT_EOL_OUTPUT
	vtProcessing   bool // ENABLE_VIRTUAL_TERMINAL_PROCESSING
	newlineAutoRet bool // !DISABLE_NEWLINE_AUTO_RETURN
}

func newV12Screen(width, height int) *v12Screen {
	return &v12Screen{
		buffer:         newV12TextBuffer(width, height),
		wrapAtEOL:      true,
		vtProcessing:   false,
		newlineAutoRet: true,
	}
}

func (s *v12Screen) cursor() *msCursor { return s.buffer.cursor }

// AdjustCursorPosition (_stream.cpp). The scroll bookkeeping is the caller's
// psScrollY; the probe passes nil.
func v12AdjustCursorPosition(screen *v12Screen, coordCursor msPoint, psScrollY *int) {
	buffer := screen.buffer
	bufferWidth := buffer.width
	bufferHeight := buffer.height

	if coordCursor.x < 0 {
		if coordCursor.y > 0 {
			coordCursor.x = bufferWidth + coordCursor.x
			coordCursor.y = coordCursor.y - 1
		} else {
			coordCursor.x = 0
		}
	} else if coordCursor.x >= bufferWidth {
		// at end of line. if wrap mode, wrap cursor. otherwise leave it where it is.
		if screen.wrapAtEOL {
			coordCursor.y += coordCursor.x / bufferWidth
			coordCursor.x = coordCursor.x % bufferWidth
		} else {
			coordCursor.x = buffer.cursor.GetPosition().x
		}
	}

	if coordCursor.y >= bufferHeight {
		buffer.IncrementCircularBuffer()
		if psScrollY != nil {
			*psScrollY += 1
		}
		coordCursor.y = bufferHeight - 1
	}

	buffer.cursor.SetPosition(coordCursor)
}

// IS_GLYPH_CHAR (_stream.h): anything that is not a C0 control or DEL.
func isGlyphChar(wch uint16) bool { return wch >= 0x20 && wch != 0x7f }

// WriteCharsLegacy, the printing path.
func v12WriteCharsLegacy(screen *v12Screen, text string, flags wcFlags, psScrollY *int) {
	buffer := screen.buffer
	cursor := buffer.cursor

	units := utf16.Encode([]rune(text))
	pcb := 0

	cursorPosition := cursor.GetPosition()
	bufferWidth := buffer.width
	// In VT mode, the width at which we wrap is determined by the line
	// rendition attribute.
	if screen.vtProcessing {
		bufferWidth = buffer.GetLineWidth(cursorPosition.y)
	}

	for pcb < len(units) {
		// correct for delayed EOL
		if delayed := cursor.GetDelayEOLWrap(); delayed != nil && screen.wrapAtEOL {
			coordDelayedAt := *delayed
			cursor.ResetDelayEOLWrap()
			// Only act on a delayed EOL if we didn't move the cursor to a
			// different position from where the EOL was marked.
			if coordDelayedAt == cursorPosition {
				cursorPosition.x = 0
				cursorPosition.y++

				v12AdjustCursorPosition(screen, cursorPosition, psScrollY)

				cursorPosition = cursor.GetPosition()
				if screen.vtProcessing {
					bufferWidth = buffer.GetLineWidth(cursorPosition.y)
				}
			}
		}

		// As an optimization, collect characters in buffer and print out all at once.
		xPosition := cursor.GetPosition().x
		var local []outputCell
		sawControl := false

		for pcb < len(units) && xPosition < bufferWidth {
			ch := units[pcb]

			if !isGlyphChar(ch) {
				// The control characters are handled after the accumulation
				// loop; every case in the original jumps to EndWhile.
				sawControl = true
				break
			}

			// IsGlyphFullWidth. The source notes it operates on a single code
			// unit and so mis-measures surrogate pairs; the ported width
			// detector is fed the same single unit, which reproduces that.
			if v12GlyphWidth(string(utf16.Decode([]uint16{ch}))) == 2 {
				if xPosition < bufferWidth-1 {
					s := string(utf16.Decode([]uint16{ch}))
					local = append(local,
						outputCell{chars: s, dbcs: dbcsLeading},
						outputCell{chars: s, dbcs: dbcsTrailing})
					// cursor adjusted by 2 because the char is double width
					xPosition += 2
				} else {
					break
				}
			} else {
				local = append(local, outputCell{chars: string(utf16.Decode([]uint16{ch})), dbcs: dbcsSingle})
				xPosition++
			}
			pcb++
		}

		// EndWhile:
		if len(local) != 0 {
			cursorPosition = cursor.GetPosition()

			// Make sure we don't write past the end of the buffer.
			if len(local) > bufferWidth-cursorPosition.x {
				local = local[:bufferWidth-cursorPosition.x]
			}

			// "line was wrapped if we're writing up to the end of the current
			// row" -- TextBuffer::Write's wrap parameter defaults to true
			// (textBuffer.hpp), which is the value this call site uses.
			wrapTrue := true
			buffer.Write(local, cursorPosition, &wrapTrue)

			// The source uses the "estimated" X position delta rather than the
			// iterator's, and says so.
			cursorPosition.x = xPosition

			// enforce a delayed newline if we're about to pass the end and the
			// WC_DELAY_EOL_WRAP flag is set.
			if flags.delayEolWrap && cursorPosition.x >= bufferWidth && screen.wrapAtEOL {
				// Our cursor position as of this time is going to remain on the
				// last position in this column.
				cursorPosition.x = bufferWidth - 1
				cursor.SetPosition(cursorPosition)
				cursor.DelayEOLWrap()
			} else {
				v12AdjustCursorPosition(screen, cursorPosition, psScrollY)
			}

			if pcb == len(units) {
				return
			}
			continue
		}

		if pcb >= len(units) {
			return
		}
		if !sawControl {
			// The accumulation loop stopped because the row is full but the
			// glyph is wide: the original loops back and the delayed-EOL or
			// wrap handling above moves to the next row.
			cursorPosition = cursor.GetPosition()
			cursorPosition.x = bufferWidth
			v12AdjustCursorPosition(screen, cursorPosition, psScrollY)
			continue
		}

		// The control-character switch of the second half of WriteCharsLegacy.
		switch units[pcb] {
		case msUnicodeBackspace:
			cursorPosition = cursor.GetPosition()
			cursorPosition.x--
			v12AdjustCursorPosition(screen, cursorPosition, psScrollY)

		case msUnicodeCarriageReturn:
			// Carriage return moves the cursor to the beginning of the line.
			// We don't need to worry about handling cr or lf for
			// formatting, since these are handled by the caller.
			cursorPosition = cursor.GetPosition()
			cursorPosition.x = 0
			v12AdjustCursorPosition(screen, cursorPosition, psScrollY)

		case msUnicodeLinefeed:
			cursorPosition = cursor.GetPosition()
			// If we're not in VT mode, then a line feed is a carriage return
			// as well, per DISABLE_NEWLINE_AUTO_RETURN being clear.
			if screen.newlineAutoRet {
				cursorPosition.x = 0
			}
			// The row the cursor is leaving ends here: clear its wrap flag.
			buffer.GetRowByOffset(cursorPosition.y).SetWrapForced(false)
			cursorPosition.y++
			v12AdjustCursorPosition(screen, cursorPosition, psScrollY)

		case msUnicodeTab:
			cursorPosition = cursor.GetPosition()
			tabCount := 8 - (cursorPosition.x & 7)
			if remaining := bufferWidth - cursorPosition.x; remaining < tabCount {
				tabCount = remaining
			}
			spaces := make([]outputCell, tabCount)
			for i := range spaces {
				spaces[i] = outputCell{chars: " ", dbcs: dbcsSingle}
			}
			wrapTrue := true
			buffer.Write(spaces, cursorPosition, &wrapTrue)
			cursorPosition.x += tabCount
			v12AdjustCursorPosition(screen, cursorPosition, psScrollY)

		case msUnicodeBell:
			// SendNotifyBeep: recorded non-port.

		default:
			// The OEM glyph conversion: recorded non-port; the character is
			// dropped, which is the original's failure branch.
		}
		pcb++
	}
}
