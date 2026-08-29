// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of the legacy write path of microsoft/terminal
// src/host/_stream.cpp (commit 079d1cc423336c89c1e220701c94b320cecb603a):
// AdjustCursorPosition, _writeCharsLegacyUnprocessed, WriteCharsLegacy,
// plus ROW::NavigateToPrevious and TextBuffer::SetWrapForced from
// src/buffer/out that they call.
//
// This is the path a WriteConsoleW child actually takes through conhost --
// NOT the VT dispatch of msdispatch.go. The two differ exactly where the
// field dump of seed 1788002866976838800 diverged from the mock: on LF the
// legacy path clears wrapForced on the row the cursor has ALREADY moved to
// ("textBuffer.GetMutableRowByOffset(pos.y).SetWrapForced(false)" runs after
// the wrap of the preceding write advanced pos.y), so a line that filled its
// last row to the edge keeps that row's wrapForced and conhost's buffer holds
// it merged with the next line. There is also no delayed EOL wrap here: the
// cursor wraps immediately, via the modulo arithmetic of
// AdjustCursorPosition.
//
// Recorded non-ports at their original call sites: the VT passthrough writer
// (gci.GetVtWriterForBuffer and every "if (writer)" block -- the probe reads
// conhost's real passthrough from the wire instead), accessibility and
// renderer notifications, SnapOnOutput, SendNotifyBeep, and the default-case
// MultiByteToWideChar OEM glyph conversion (no stream this tool generates
// carries C0 bytes other than BEL/BS/TAB/LF/CR; a byte that would reach that
// case is dropped, which is the result==1 failure branch of the original).
// ENABLE_PROCESSED_OUTPUT is set and DISABLE_NEWLINE_AUTO_RETURN is clear,
// which is the mode a plain WriteConsoleW child runs under.

package main

// msScreenInfo is the used subset of SCREEN_INFORMATION: the buffer, its
// cursor, and the two OutputMode flags the ported code tests.
type msScreenInfo struct {
	textBuffer *msTextBuffer
	cursor     *msCursor

	wrapAtEOL       bool // ENABLE_WRAP_AT_EOL_OUTPUT
	processedOutput bool // ENABLE_PROCESSED_OUTPUT
	newlineAutoRet  bool // !DISABLE_NEWLINE_AUTO_RETURN
}

func newMsScreenInfo(width, height int) *msScreenInfo {
	return &msScreenInfo{
		textBuffer:      newMsTextBuffer(width, height),
		cursor:          &msCursor{},
		wrapAtEOL:       true,
		processedOutput: true,
		newlineAutoRet:  true,
	}
}

// ROW::NavigateToPrevious
func (r *msROW) NavigateToPrevious(column int) int {
	return r._adjustBackward(r._clampedColumn(column - 1))
}

// TextBuffer::SetWrapForced
func (b *msTextBuffer) SetWrapForced(y int, wrap bool) {
	b.GetMutableRowByOffset(y).SetWrapForced(wrap)
}

// AdjustCursorPosition (_stream.cpp)
func msAdjustCursorPosition(screenInfo *msScreenInfo, coordCursor msPoint, psScrollY *int) {
	bufferWidth := screenInfo.textBuffer.Width()
	bufferHeight := screenInfo.textBuffer.Height()
	if coordCursor.x < 0 {
		if coordCursor.y > 0 {
			coordCursor.x = bufferWidth + coordCursor.x
			coordCursor.y = coordCursor.y - 1
		} else {
			coordCursor.x = 0
		}
	} else if coordCursor.x >= bufferWidth {
		// at end of line. if wrap mode, wrap cursor.  otherwise leave it where it is.
		if screenInfo.wrapAtEOL {
			coordCursor.y += coordCursor.x / bufferWidth
			coordCursor.x = coordCursor.x % bufferWidth
		} else {
			coordCursor.x = screenInfo.cursor.GetPosition().x
		}
	}

	if coordCursor.y >= bufferHeight {
		buffer := screenInfo.textBuffer
		buffer.IncrementCircularBuffer(TextAttribute{})

		// accessibility/renderer scroll notifications: recorded non-port.

		if psScrollY != nil {
			*psScrollY += 1
		}

		coordCursor.y = bufferHeight - 1
	}

	screenInfo.cursor.SetPosition(coordCursor)
}

// _writeCharsLegacyUnprocessed (_stream.cpp): "As the name implies, this
// writes text without processing its control characters."
func msWriteCharsLegacyUnprocessed(screenInfo *msScreenInfo, text []uint16, psScrollY *int) bool {
	wrapAtEOL := screenInfo.wrapAtEOL
	textBuffer := screenInfo.textBuffer
	wrapped := false

	state := msRowWriteState{
		text:        text,
		columnLimit: textBuffer.Width(),
	}

	for len(state.text) != 0 {
		cursorPosition := screenInfo.cursor.GetPosition()

		state.columnBegin = cursorPosition.x
		textBuffer.Replace(cursorPosition.y, TextAttribute{}, &state)
		cursorPosition.x = state.columnEnd
		wrapped = wrapAtEOL && state.columnEnd >= state.columnLimit

		if wrapped {
			textBuffer.SetWrapForced(cursorPosition.y, true)
		}

		msAdjustCursorPosition(screenInfo, cursorPosition, psScrollY)
	}

	return wrapped
}

func msControlCharPredicate(wch uint16) bool { return wch < 0x20 || wch == 0x7f }

const (
	msUnicodeNull           = 0x0000
	msUnicodeBell           = 0x0007
	msUnicodeBackspace      = 0x0008
	msUnicodeTab            = 0x0009
	msUnicodeLinefeed       = 0x000a
	msUnicodeCarriageReturn = 0x000d
)

// WriteCharsLegacy (_stream.cpp). psScrollY may be nil.
func msWriteCharsLegacy(screenInfo *msScreenInfo, text []uint16, psScrollY *int) {
	tabSpaces := []uint16{' ', ' ', ' ', ' ', ' ', ' ', ' ', ' '}

	textBuffer := screenInfo.textBuffer
	width := textBuffer.Width()
	cursor := screenInfo.cursor
	wrapAtEOL := screenInfo.wrapAtEOL
	beg := 0
	end := len(text)
	it := beg

	// VT passthrough writer, accessibility events, SnapOnOutput: recorded
	// non-ports (file header).

	// If we enter this if condition, then someone wrote text in VT mode and now switched to non-VT mode.
	// Since the Console APIs don't support delayed EOL wrapping, we need to first put the cursor back
	// to a position that the Console APIs expect (= not delayed).
	if delayed := cursor.GetDelayEOLWrap(); delayed != nil && wrapAtEOL {
		pos := cursor.GetPosition()
		cursor.ResetDelayEOLWrap()
		if *delayed == pos {
			pos.x = 0
			pos.y++
			msAdjustCursorPosition(screenInfo, pos, psScrollY)
		}
	}

	// If ENABLE_PROCESSED_OUTPUT is set we search for C0 control characters and handle them like backspace, tab, etc.
	// If it's not set, we can just straight up give everything to WriteCharsLegacyUnprocessed.
	if !screenInfo.processedOutput {
		msWriteCharsLegacyUnprocessed(screenInfo, text, psScrollY)
		return
	}

	for it != end {
		nextControlChar := it
		for nextControlChar != end && !msControlCharPredicate(text[nextControlChar]) {
			nextControlChar++
		}
		if nextControlChar != it {
			chunk := text[it:nextControlChar]
			msWriteCharsLegacyUnprocessed(screenInfo, chunk, psScrollY)
			it = nextControlChar
		}

		if it == end {
			break
		}

		for {
			wch := text[it]

			switch wch {
			case msUnicodeNull:
				msWriteCharsLegacyUnprocessed(screenInfo, tabSpaces[:1], psScrollY)
			case msUnicodeBell:
				// SendNotifyBeep: recorded non-port.
			case msUnicodeBackspace:
				pos := cursor.GetPosition()
				pos.x = textBuffer.GetRowByOffset(pos.y).NavigateToPrevious(pos.x)
				msAdjustCursorPosition(screenInfo, pos, psScrollY)
			case msUnicodeTab:
				pos := cursor.GetPosition()
				remaining := width - pos.x
				tabCount := 8 - (pos.x & 7)
				if remaining < tabCount {
					tabCount = remaining
				}
				if tabCount < 0 {
					tabCount = 0
				}
				msWriteCharsLegacyUnprocessed(screenInfo, tabSpaces[:tabCount], psScrollY)
			case msUnicodeLinefeed:
				pos := cursor.GetPosition()

				// If DISABLE_NEWLINE_AUTO_RETURN is not set, any LF behaves like a CRLF.
				if screenInfo.newlineAutoRet {
					pos.x = 0
				}

				textBuffer.SetWrapForced(pos.y, false)
				pos.y = pos.y + 1
				msAdjustCursorPosition(screenInfo, pos, psScrollY)
			case msUnicodeCarriageReturn:
				pos := cursor.GetPosition()
				pos.x = 0
				msAdjustCursorPosition(screenInfo, pos, psScrollY)
			default:
				// MultiByteToWideChar OEM glyph conversion: recorded
				// non-port; the character is dropped, which is the
				// original's result != 1 branch.
			}

			it++
			if !(it != end && msControlCharPredicate(text[it])) {
				break
			}
		}
	}
}
