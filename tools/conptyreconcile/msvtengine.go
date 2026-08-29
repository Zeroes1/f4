// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of the ConPTY output renderer of microsoft/terminal at the pinned
// tag v1.12.10982.0 (docs/PINNED_CONSOLE.md): src/renderer/vt/paint.cpp
// (StartPaint, EndPaint, _PaintUtf8BufferLine), src/renderer/vt/XtermEngine.cpp
// (_MoveCursor) and the sequences of src/renderer/vt/VtSequences.cpp that
// they emit.
//
// WHY THIS FILE EXISTS. The mock in mock.go composes ConPTY output by hand --
// it decides where an ESC[K goes, where a CRLF goes, when a line is padded.
// Every one of those decisions is a guess about conhost, and THE RULE forbids
// guesses where Microsoft's source exists. It exists: this is the code that
// produces those bytes. With it ported, debugging locally against the mock is
// debugging against the pinned host, and a Windows run stops being the way we
// learn things and becomes a final confirmation.
//
// The three behaviours the field dumps show, and where they come from here:
//   - "a full row carries no ESC[K, a short row does" (P6) is not a rule
//     anyone wrote down: it falls out of numSpaces / useEraseChar below. A row
//     with no trailing spaces has numSpaces == 0, so neither _EraseCharacter
//     nor _EraseLine is emitted.
//   - "wrapped rows are joined by CRLF" (P12) falls out of _MoveCursor: the
//     "down one line, start of line" branch emits "\r\n" unless the previous
//     row was marked wrapped, in which case it emits nothing at all.
//   - the delayed EOL wrap state that makes the cursor at the right margin
//     force a full CUP instead of a clever short sequence.
//
// Mechanical transformations required by Go, and nothing else:
//   - HRESULT plumbing (RETURN_IF_FAILED) becomes straight-line code; there is
//     no failing write to a byte slice
//   - std::optional<short> _wrappedRow becomes a *int
//   - the tracing calls (_trace.*) are dropped: they emit ETW events, not bytes
//
// Recorded non-ports: the ASCII path (_PaintAsciiBufferLine; the probe's host
// is xterm-256color, which takes the UTF-8 path), colours/SGR
// (_lastTextAttributes and GH#5502's bgMatched, which are colour decisions --
// bgMatched is therefore always true here, its value when no colour changed),
// the scroll optimisation (_MoveCursorScroll / InvalidateScroll), soft-wrap
// tracing, and the resize quirk (_resizeQuirk is off: winconpty passes no
// PSEUDOCONSOLE_RESIZE_QUIRK, see msconpty.go).

package main

import (
	"fmt"
	"strings"
)

// ERASE_CHARACTER_STRING_LENGTH (vtrenderer.hpp): the length in bytes of the
// shortest useful "ESC[nX ESC[nC" pair, the threshold that decides whether
// erasing beats printing the spaces.
const eraseCharacterStringLength = 8

type vtCoord struct{ X, Y int }

var invalidCoords = vtCoord{-1, -1}

// vtEngine is VtEngine + XtermEngine, restricted to the state the ported
// methods read and write.
type vtEngine struct {
	out strings.Builder

	_lastText           vtCoord
	_lastViewportRight  int // _lastViewport.RightInclusive()
	_lastViewportBottom int // _lastViewport.BottomInclusive()
	_virtualTop         int

	_wrappedRow        *int
	_delayedEolWrap    bool
	_deferredCursorPos vtCoord

	_clearedAllThisFrame bool
	_newBottomLine       bool
	_needToDisableCursor bool
	_resized             bool
	_resizeQuirk         bool
}

func newVtEngine(width, height int) *vtEngine {
	return &vtEngine{
		_lastViewportRight:  width - 1,
		_lastViewportBottom: height - 1,
		_deferredCursorPos:  invalidCoords,
	}
}

// _lastViewport.RightExclusive()
func (e *vtEngine) rightExclusive() int { return e._lastViewportRight + 1 }

func (e *vtEngine) _Write(s string) { e.out.WriteString(s) }

// VtSequences.cpp
func (e *vtEngine) _EraseLine()                 { e._Write("\x1b[K") }
func (e *vtEngine) _EraseCharacter(chars int)   { e._Write(fmt.Sprintf("\x1b[%dX", chars)) }
func (e *vtEngine) _CursorForward(chars int)    { e._Write(fmt.Sprintf("\x1b[%dC", chars)) }
func (e *vtEngine) _CursorHome()                { e._Write("\x1b[H") }
func (e *vtEngine) _CursorPosition(c vtCoord)   { e._Write(fmt.Sprintf("\x1b[%d;%dH", c.Y+1, c.X+1)) }
func (e *vtEngine) _WriteTerminalUtf8(s string) { e._Write(s) }

// XtermEngine::_MoveCursor
func (e *vtEngine) _MoveCursor(coord vtCoord) {
	if coord.X != e._lastText.X || coord.Y != e._lastText.Y {
		switch {
		case coord.X == 0 && coord.Y == 0:
			e._needToDisableCursor = true
			e._CursorHome()

		case e._resized && e._resizeQuirk:
			e._CursorPosition(coord)

		case coord.X == 0 && coord.Y == e._lastText.Y+1:
			// Down one line, at the start of the line.
			//
			// If the previous line wrapped, then the cursor is already at this
			// position, we just don't know it yet. Don't emit anything.
			previousLineWrapped := false
			if e._wrappedRow != nil {
				previousLineWrapped = coord.Y == *e._wrappedRow+1
			}
			if !previousLineWrapped {
				e._Write("\r\n")
			}

		case e._delayedEolWrap:
			// GH#1245, GH#357 - If we were in the delayed EOL wrap state, make
			// sure to _manually_ position the cursor now, with a full CUP
			// sequence, don't try and be clever with \b or \r or other control
			// sequences.
			e._CursorPosition(coord)

		case coord.X == 0 && coord.Y == e._lastText.Y:
			// Start of this line
			e._Write("\r")

		case coord.X == e._lastText.X && coord.Y == e._lastText.Y+1:
			// Down one line, same X position
			e._Write("\n")

		case coord.X == e._lastText.X-1 && coord.Y == e._lastText.Y:
			// Back one char, same Y position
			e._Write("\b")

		case coord.Y == e._lastText.Y && coord.X > e._lastText.X:
			// Same line, forward some distance
			e._CursorForward(coord.X - e._lastText.X)

		default:
			e._needToDisableCursor = true
			e._CursorPosition(coord)
		}

		e._lastText = coord
	}

	e._deferredCursorPos = invalidCoords
	e._wrappedRow = nil
	e._delayedEolWrap = false
}

// VtEngine::_PaintUtf8BufferLine. `clusters` is the row's text; columns is the
// total width in cells (they differ for wide glyphs).
func (e *vtEngine) _PaintUtf8BufferLine(text string, totalWidth int, coord vtCoord, lineWrapped bool) {
	if coord.Y < e._virtualTop {
		return
	}

	bufferLine := []rune(text)
	cchLine := len(bufferLine)

	foundNonspace := false
	lastNonSpace := 0
	for i := 0; i < cchLine; i++ {
		if bufferLine[i] != ' ' {
			lastNonSpace = i
			foundNonspace = true
		}
	}
	nonSpaceAdjust := 0
	if foundNonspace {
		nonSpaceAdjust = 1
	}
	numSpaces := cchLine - lastNonSpace - nonSpaceAdjust

	// Optimizations: if there are lots of spaces at the end of the line, we can
	// try to Erase Character that number of spaces, then move the cursor
	// forward. Also, if we already erased the entire display this frame, then
	// don't do ANYTHING with erasing at all.
	optimalToUseECH := numSpaces > eraseCharacterStringLength
	useEraseChar := optimalToUseECH && !e._newBottomLine && !e._clearedAllThisFrame
	printingBottomLine := coord.Y == e._lastViewportBottom

	// GH#5502's bgMatched: a colour decision, and colours are a recorded
	// non-port. Its value when no colour changed is true.
	bgMatched := true

	// GH#5291: DON'T remove spaces when the row wrapped. We might need those
	// spaces to preserve the wrap state of this line, or the cursor position.
	removeSpaces := !lineWrapped && (useEraseChar ||
		e._clearedAllThisFrame ||
		(e._newBottomLine && printingBottomLine && bgMatched))

	cchActual := cchLine
	columnsActual := totalWidth
	if removeSpaces {
		cchActual = cchLine - numSpaces
		columnsActual = totalWidth - numSpaces
	}

	if cchActual == 0 {
		// If the previous row wrapped, but this line is empty, then we actually
		// do want to move the cursor down.
		e._wrappedRow = nil
	}

	// Move the cursor to the start of this run.
	e._MoveCursor(coord)

	// Write the actual text string
	e._WriteTerminalUtf8(string(bufferLine[:cchActual]))

	// GH#4415, GH#5181: if the renderer told us that this was a wrapped line,
	// mark that we've wrapped this line.
	if lineWrapped {
		y := coord.Y
		e._wrappedRow = &y
	}

	// GH#1245: RightExclusive, not inclusive.
	if e._lastText.X < e.rightExclusive() {
		e._lastText.X += columnsActual
	}
	// GH#1245: if we wrote the exactly last char of the row, we're in the
	// "delayed EOL wrap" state.
	if e._lastText.X >= e._lastViewportRight {
		e._delayedEolWrap = true
	}

	if useEraseChar {
		// ECH doesn't actually move the cursor itself.
		e._deferredCursorPos = vtCoord{e._lastText.X + numSpaces, e._lastText.Y}
		if e._deferredCursorPos.X <= e._lastViewportRight {
			e._EraseCharacter(numSpaces)
		} else {
			e._EraseLine()
		}
	} else if e._newBottomLine && printingBottomLine {
		// If we're on a new line, then we don't need to erase the line.
		if optimalToUseECH {
			e._deferredCursorPos = vtCoord{e._lastText.X + numSpaces, e._lastText.Y}
		} else if numSpaces > 0 && removeSpaces {
			// if we deleted the spaces... re-add them
			e._WriteTerminalUtf8(strings.Repeat(" ", numSpaces))
			e._lastText.X += numSpaces
		}
	}

	if printingBottomLine {
		e._newBottomLine = false
	}
}

// VtEngine::EndPaint, reduced to the deferred-cursor move the frame needs.
func (e *vtEngine) EndPaint() {
	if e._deferredCursorPos != invalidCoords {
		e._MoveCursor(e._deferredCursorPos)
	}
	e._resized = false
	e._clearedAllThisFrame = false
}
