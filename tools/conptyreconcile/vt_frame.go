package main

import (
	"fmt"
	"strings"
)

type frameCluster struct {
	Units   []uint16
	Columns int
	Attr    textAttribute
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
	lastTextAttributes  textAttribute
	clearedAll          bool
	needToDisableCursor bool
	lastCursorVisible   bool
	nextCursorVisible   bool
	firstPaint          bool
	output              []byte
	failed              error
}

func newFrameEmitter(width, height, top int) *frameEmitter {
	return &frameEmitter{
		lastText: coordinate{x: -1, y: -1},
		viewport: viewport{Top: top, Width: width, Height: height},
		lastTextAttributes: textAttribute{
			foreground: textColor{kind: textColorRGB, rgb: 0xffffffff},
			background: textColor{kind: textColorRGB, rgb: 0xffffffff},
		},
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

func (e *frameEmitter) setGraphicsRendition16Color(index uint8, foreground bool) {
	vtIndex := 30
	if !foreground {
		vtIndex += 10
	}
	if index&0x08 != 0 {
		vtIndex += 60
	}
	if index&0x04 != 0 {
		vtIndex++
	}
	if index&0x02 != 0 {
		vtIndex += 2
	}
	if index&0x01 != 0 {
		vtIndex += 4
	}
	e.write(fmt.Sprintf("\x1b[%dm", vtIndex))
}

func (e *frameEmitter) setGraphicsRendition256Color(index uint8, foreground bool) {
	which := 4
	if foreground {
		which = 3
	}
	e.write(fmt.Sprintf("\x1b[%d8;5;%dm", which, xterm256ToWindowsIndex(int(index))))
}

func (e *frameEmitter) updateDrawingBrushes(attr textAttribute) {
	// This is Xterm256Engine::UpdateDrawingBrushes in source order:
	// _RgbUpdateDrawingBrushes, hyperlink update, then _UpdateExtendedAttrs.
	// The hyperlink numeric/process identity is not available in the pinned
	// source tree (see AUDIT_LEDGER.md). Do not silently emit a frame without
	// the source's OSC 8 transition: that would turn an unresolved boundary
	// into a false pass.
	fg := attr.foreground
	bg := attr.background
	lastFG := e.lastTextAttributes.foreground
	lastBG := e.lastTextAttributes.background
	if fg.kind == textColorDefault && bg.kind == textColorDefault &&
		!(lastFG.kind == textColorDefault && lastBG.kind == textColorDefault) {
		e.write("\x1b[m")
		e.lastTextAttributes.setDefaultBackground()
		e.lastTextAttributes.setDefaultForeground()
		e.lastTextAttributes.setDefaultMetaAttrs()
		lastFG = textColor{}
		lastBG = textColor{}
	}
	if fg != lastFG {
		switch fg.kind {
		case textColorDefault:
			e.write("\x1b[39m")
		case textColorIndex16:
			e.setGraphicsRendition16Color(fg.index, true)
		case textColorIndex256:
			e.setGraphicsRendition256Color(fg.index, true)
		case textColorRGB:
			e.write(fmt.Sprintf("\x1b[38;2;%d;%d;%dm", fg.rgb&0xff, (fg.rgb>>8)&0xff, (fg.rgb>>16)&0xff))
		}
		e.lastTextAttributes.foreground = fg
	}
	if bg != lastBG {
		switch bg.kind {
		case textColorDefault:
			e.write("\x1b[49m")
		case textColorIndex16:
			e.setGraphicsRendition16Color(bg.index, false)
		case textColorIndex256:
			e.setGraphicsRendition256Color(bg.index, false)
		case textColorRGB:
			e.write(fmt.Sprintf("\x1b[48;2;%d;%d;%dm", bg.rgb&0xff, (bg.rgb>>8)&0xff, (bg.rgb>>16)&0xff))
		}
		e.lastTextAttributes.background = bg
	}
	if attr.hyperlinkID != e.lastTextAttributes.hyperlinkID {
		e.failed = fmt.Errorf("pinned renderer hyperlink output is unavailable: hyperlink id %d", attr.hyperlinkID)
		return
	}

	// Xterm256Engine::_UpdateExtendedAttrs. Bold/faint and the two underline
	// forms share the source reset sequences, so the assignment order matters.
	last := &e.lastTextAttributes
	if (!attr.isBold() && last.isBold()) || (!attr.isFaint() && last.isFaint()) {
		e.write("\x1b[22m")
		last.setBold(false)
		last.setFaint(false)
	}
	if attr.isBold() && !last.isBold() {
		e.write("\x1b[1m")
		last.setBold(true)
	}
	if attr.isFaint() && !last.isFaint() {
		e.write("\x1b[2m")
		last.setFaint(true)
	}
	if (!attr.isUnderlined() && last.isUnderlined()) || (!attr.isDoublyUnderlined() && last.isDoublyUnderlined()) {
		e.write("\x1b[24m")
		last.setUnderlined(false)
		last.setDoublyUnderlined(false)
	}
	if attr.isUnderlined() && !last.isUnderlined() {
		e.write("\x1b[4m")
		last.setUnderlined(true)
	}
	if attr.isDoublyUnderlined() && !last.isDoublyUnderlined() {
		e.write("\x1b[21m")
		last.setDoublyUnderlined(true)
	}
	if attr.isOverlined() != last.isOverlined() {
		e.write(mapBoolSequence(attr.isOverlined(), "\x1b[53m", "\x1b[55m"))
		last.setOverlined(attr.isOverlined())
	}
	if attr.isItalic() != last.isItalic() {
		e.write(mapBoolSequence(attr.isItalic(), "\x1b[3m", "\x1b[23m"))
		last.setItalic(attr.isItalic())
	}
	if attr.isBlinking() != last.isBlinking() {
		e.write(mapBoolSequence(attr.isBlinking(), "\x1b[5m", "\x1b[25m"))
		last.setBlinking(attr.isBlinking())
	}
	if attr.isInvisible() != last.isInvisible() {
		e.write(mapBoolSequence(attr.isInvisible(), "\x1b[8m", "\x1b[28m"))
		last.setInvisible(attr.isInvisible())
	}
	if attr.isCrossedOut() != last.isCrossedOut() {
		e.write(mapBoolSequence(attr.isCrossedOut(), "\x1b[9m", "\x1b[29m"))
		last.setCrossedOut(attr.isCrossedOut())
	}
	if attr.isReverseVideo() != last.isReverseVideo() {
		e.write(mapBoolSequence(attr.isReverseVideo(), "\x1b[7m", "\x1b[27m"))
		last.setReverseVideo(attr.isReverseVideo())
	}
	last.setHyperlinkID(attr.hyperlinkID)
}

func mapBoolSequence(enabled bool, on, off string) string {
	if enabled {
		return on
	}
	return off
}

func (e *frameEmitter) endPaint() {
	if e.failed != nil {
		return
	}
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
		// The pinned renderer narrows every cluster width to short and then
		// uses ShortAdd for the running total. Preserve both failure points;
		// an int accumulator must not turn an HRESULT failure into a pass.
		if cluster.Columns < 0 || cluster.Columns > maxInt16 || totalWidth > maxInt16-cluster.Columns {
			e.failed = fmt.Errorf("pinned renderer cluster width overflow: total=%d cluster=%d", totalWidth, cluster.Columns)
			return
		}
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
	if numSpaces > maxInt16 {
		e.failed = fmt.Errorf("pinned renderer trailing-space count overflow: %d", numSpaces)
		return
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
	if len(clusters) != 0 {
		e.updateDrawingBrushes(clusters[0].Attr)
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

const maxInt16 = 1<<15 - 1

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
