package main

import (
	"fmt"
	"strings"
)

type frameCluster struct {
	Units   []uint16
	Columns int
}

// frameEmitter contains the state tracked by the pinned XtermEngine/VtEngine
// output path.  It emits the same cursor-control spellings used by
// VtSequences.cpp; it does not infer a frame from a text fixture.
type frameEmitter struct {
	lastText            coordinate
	viewport            viewport
	resizeQuirk         bool
	resized             bool
	wrappedRow          *int
	delayedEOLWrap      bool
	deferredCursorPos   *coordinate
	virtualTop          int
	circled             bool
	newBottomLine       bool
	newBottomBGMatched  bool
	clearedAll          bool
	needToDisableCursor bool
	lastCursorVisible   bool
	nextCursorVisible   bool
	firstPaint          bool
	output              []byte
}

func newFrameEmitter(width, height, top int) *frameEmitter {
	return &frameEmitter{
		lastText:           coordinate{x: -1, y: -1},
		viewport:           viewport{Top: top, Width: width, Height: height},
		newBottomBGMatched: true,
		nextCursorVisible:  true,
		firstPaint:         true,
	}
}

func (e *frameEmitter) write(data string) { e.output = append(e.output, data...) }

func (e *frameEmitter) cursorHome() { e.write("\x1b[H") }

func (e *frameEmitter) cursorPosition(pos coordinate) {
	e.write(fmt.Sprintf("\x1b[%d;%dH", pos.y+1, pos.x+1))
}

func (e *frameEmitter) cursorForward(distance int) {
	e.write(fmt.Sprintf("\x1b[%dC", distance))
}

// startPaint is the pinned VtEngine::StartPaint/XtermEngine::StartPaint
// first-frame branch. The caller supplies a frame only when there is work to
// paint, so the quick-return branch is not represented here.
func (e *frameEmitter) startPaint() {
	e.nextCursorVisible = false
	if e.firstPaint {
		e.write("\x1b[2J")
		e.clearedAll = true
		e.firstPaint = false
	}
}

func (e *frameEmitter) eraseCharacter(count int) {
	e.write(fmt.Sprintf("\x1b[%dX", count))
}

func (e *frameEmitter) eraseLine() { e.write("\x1b[K") }

func (e *frameEmitter) endPaint() {
	if e.needToDisableCursor && e.lastCursorVisible {
		e.output = append([]byte("\x1b[?25l"), e.output...)
		e.lastCursorVisible = false
	}
	if e.nextCursorVisible && !e.lastCursorVisible {
		e.write("\x1b[?25h")
	} else if !e.nextCursorVisible && e.lastCursorVisible {
		e.write("\x1b[?25l")
	}
	e.lastCursorVisible = e.nextCursorVisible
	// VtEngine::EndPaint clears per-frame state and updates the virtual top
	// before consuming a deferred cursor move.
	e.clearedAll = false
	e.resized = false
	if e.circled && e.virtualTop > 0 {
		e.virtualTop--
	}
	e.circled = false
	if e.deferredCursorPos != nil {
		pos := *e.deferredCursorPos
		e.moveCursor(pos)
	}
	e.deferredCursorPos = nil
	e.needToDisableCursor = false
}

// moveCursor is XtermEngine::_MoveCursor transcribed from its branch order.
func (e *frameEmitter) moveCursor(pos coordinate) {
	if pos.x != e.lastText.x || pos.y != e.lastText.y {
		switch {
		case pos.x == 0 && pos.y == 0:
			e.needToDisableCursor = true
			e.cursorHome()
		case e.resized && e.resizeQuirk:
			e.cursorPosition(pos)
		case pos.x == 0 && pos.y == e.lastText.y+1:
			previousLineWrapped := e.wrappedRow != nil && pos.y == *e.wrappedRow+1
			if !previousLineWrapped {
				e.write("\r\n")
			}
		case e.delayedEOLWrap:
			e.cursorPosition(pos)
		case pos.x == 0 && pos.y == e.lastText.y:
			e.write("\r")
		case pos.x == e.lastText.x && pos.y == e.lastText.y+1:
			e.write("\n")
		case pos.x == e.lastText.x-1 && pos.y == e.lastText.y:
			e.write("\b")
		case pos.y == e.lastText.y && pos.x > e.lastText.x:
			e.cursorForward(pos.x - e.lastText.x)
		default:
			e.needToDisableCursor = true
			e.cursorPosition(pos)
		}
		e.lastText = pos
	}
	e.wrappedRow = nil
	e.delayedEOLWrap = false
	// XtermEngine::_MoveCursor clears a deferred cursor position on every
	// cursor move; the deferred position is only consumed by EndPaint before
	// this point.
	e.deferredCursorPos = nil
}

// paint is VtEngine::_PaintUtf8BufferLine for the state that affects the
// emitted stream: trailing-space accounting, cursor movement, wrapped-row
// tracking, and delayed EOL. PaintBufferGridLines is separately represented as
// the pinned source's no-op and therefore emits no bytes.
func (e *frameEmitter) paint(clusters []frameCluster, pos coordinate, lineWrapped bool) {
	if pos.y < e.virtualTop {
		return
	}
	lineUnits := make([]uint16, 0)
	totalWidth := 0
	for _, cluster := range clusters {
		lineUnits = append(lineUnits, cluster.Units...)
		totalWidth += cluster.Columns
	}
	cchLine := len(lineUnits)
	lastNonSpace := 0
	foundNonspace := false
	for i, unit := range lineUnits {
		if unit != ' ' {
			lastNonSpace = i
			foundNonspace = true
		}
	}
	numSpaces := cchLine - lastNonSpace
	if foundNonspace {
		numSpaces--
	}
	optimalToUseECH := numSpaces > 8
	useEraseChar := optimalToUseECH && !e.newBottomLine && !e.clearedAll
	printingBottomLine := pos.y == e.viewport.Top+e.viewport.Height-1
	removeSpaces := !lineWrapped && (useEraseChar || e.clearedAll || (e.newBottomLine && printingBottomLine && e.newBottomBGMatched))
	actualUnits := lineUnits
	actualWidth := totalWidth
	if removeSpaces {
		actualUnits = lineUnits[:len(lineUnits)-numSpaces]
		actualWidth -= numSpaces
	}
	if len(actualUnits) == 0 {
		e.wrappedRow = nil
	}
	e.moveCursor(pos)
	e.write(string(runesFromUTF16(actualUnits)))
	if lineWrapped {
		row := pos.y
		e.wrappedRow = &row
	}
	if e.lastText.x < e.viewport.Left+e.viewport.Width {
		e.lastText.x += actualWidth
	}
	if e.lastText.x >= e.viewport.Left+e.viewport.Width-1 {
		e.delayedEOLWrap = true
	}
	if useEraseChar {
		position := coordinate{x: e.lastText.x + numSpaces, y: e.lastText.y}
		e.deferredCursorPos = &position
		if position.x <= e.viewport.Left+e.viewport.Width-1 {
			e.eraseCharacter(numSpaces)
		} else {
			e.eraseLine()
		}
	} else if e.newBottomLine && printingBottomLine && optimalToUseECH {
		position := coordinate{x: e.lastText.x + numSpaces, y: e.lastText.y}
		e.deferredCursorPos = &position
	} else if e.newBottomLine && printingBottomLine && numSpaces > 0 && removeSpaces {
		e.write(strings.Repeat(" ", numSpaces))
		e.lastText.x += numSpaces
	}
	if printingBottomLine {
		e.newBottomLine = false
	}
}

// paintCursor follows Renderer::_GetCursorInfo, XtermEngine::PaintCursor, and
// VtEngine::PaintCursor for the cursor visibility and deferred-wrap branches.
func (e *frameEmitter) paintCursor(cursor cursorState) {
	if !cursor.visible || cursor.popupShown {
		return
	}
	position := cursor.position
	position.y -= e.viewport.Top
	if position.x < e.viewport.Left-1 || position.x > e.viewport.Left+e.viewport.Width-1 || position.y < 0 || position.y >= e.viewport.Height {
		return
	}
	e.nextCursorVisible = true
	cursorIsInDeferredWrap := position.x == e.lastText.x-1 && position.y == e.lastText.y
	if !((cursorIsInDeferredWrap || e.circled) && e.delayedEOLWrap && e.wrappedRow != nil) {
		e.moveCursor(position)
	}
}
