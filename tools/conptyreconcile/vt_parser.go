// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Source-faithful Go transcription of the parser/dispatch path in the pinned
// Microsoft Terminal source e9b4e2e18fb1b9cee6839969d42cd0f95d228926:
// src/terminal/parser/stateMachine.cpp, OutputStateMachineEngine.cpp, and
// the exercised AdaptDispatch callbacks.

package main

import (
	"bytes"
	"fmt"
)

type parserState uint8

const (
	stateGround parserState = iota
	stateEscape
	stateEscapeIntermediate
	stateCSIEntry
	stateCSIParam
	stateCSIIntermediate
	stateCSIIgnore
	stateOSCParam
	stateOSCString
	stateOSCTermination
	stateSS3Entry
	stateSS3Param
	stateDCSEntry
	stateDCSParam
	stateDCSIntermediate
	stateDCSIgnore
	stateDCSPassThrough
	stateSosPmApc
	stateVT52Param
)

type vtParser struct {
	buffer            *textBuffer
	mainBuffer        *textBuffer
	altBuffer         *textBuffer
	state             parserState
	params            []int
	paramValue        int
	paramStarted      bool
	parameterLimit    bool
	private           byte
	intermediate      []uint16
	oscParam          int
	oscDigits         []byte
	oscString         []uint16
	dcsPassThrough    bool
	dcsData           []uint16
	wideParser        *utf8WideParser
	newLineAutoReturn bool
	ansiMode          bool
	vt52Params        []uint16
	title             string
	bells             int
	lastPrinted       uint16
	printed           bool
	sgrStack          []textAttribute
	responses         [][]byte
	cursorKeysMode    bool
	keypadMode        bool
	deccolmSupport    bool
	screenMode        bool
	mouseModes        map[int]bool
	bracketedPaste    bool
	win32InputMode    bool
	failed            error
}

func (p *vtParser) standardErase() textAttribute {
	erase := p.buffer.currentAttrs
	erase.setStandardErase()
	return erase
}

func newVTParser(width, height int) *vtParser {
	b := newTextBuffer(width, height)
	b.vtMode = true
	return &vtParser{buffer: b, mainBuffer: b, state: stateGround, wideParser: newUTF8WideParser(), newLineAutoReturn: true, ansiMode: true, mouseModes: make(map[int]bool)}
}

func (p *vtParser) reset() {
	p.buffer = newTextBuffer(p.buffer.width, p.buffer.height)
	p.mainBuffer = p.buffer
	p.altBuffer = nil
	p.state = stateGround
	p.params = nil
	p.paramValue = 0
	p.paramStarted = false
	p.parameterLimit = false
	p.private = 0
	p.intermediate = nil
	p.oscParam = 0
	p.oscDigits = nil
	p.oscString = nil
	p.dcsPassThrough = false
	p.dcsData = nil
	p.wideParser = newUTF8WideParser()
	p.newLineAutoReturn = true
	p.ansiMode = true
	p.vt52Params = nil
	p.title = ""
	p.bells = 0
	p.lastPrinted = 0
	p.printed = false
	p.sgrStack = nil
	p.responses = nil
	p.cursorKeysMode = false
	p.keypadMode = false
	p.deccolmSupport = false
	p.screenMode = false
	p.mouseModes = make(map[int]bool)
	p.bracketedPaste = false
	p.win32InputMode = false
	p.failed = nil
}

func (p *vtParser) feed(data []byte) error {
	if p.failed != nil {
		return p.failed
	}
	units, err := p.wideParser.feed(data)
	if err != nil {
		p.failed = err
		return err
	}
	p.processUnits(units)
	return p.failed
}

func (p *vtParser) finish() error {
	units, err := p.wideParser.finish()
	if err != nil {
		p.failed = err
		return err
	}
	p.processUnits(units)
	return p.failed
}

// processUnits mirrors StateMachine::ProcessString's ground-state fast path:
// a run of non-actionable UTF-16 units is handed to ActionPrintString as one
// sequence, while controls are fed through ProcessCharacter individually.
func (p *vtParser) processUnits(units []uint16) {
	for len(units) > 0 {
		if p.state == stateGround {
			start := 0
			for start < len(units) && !isActionableGroundUnit(units[start]) {
				start++
			}
			if start != 0 {
				p.printUnits(units[:start])
				units = units[start:]
				continue
			}
		}
		if p.state == stateOSCString && units[0] > 0x9f {
			end := 1
			for end < len(units) && units[end] > 0x9f {
				end++
			}
			p.oscString = append(p.oscString, units[:end]...)
			units = units[end:]
			continue
		}
		p.consumeUnit(units[0])
		units = units[1:]
	}
}

func isActionableGroundUnit(unit uint16) bool {
	return unit <= 0x1f || unit == 0x7f || unit >= 0x80 && unit <= 0x9f
}

func (p *vtParser) printUnits(units []uint16) {
	p.buffer.returnOnNewline = p.newLineAutoReturn
	if err := writeDefaultString(p.buffer, units); err != nil {
		p.failed = err
		return
	}
	if len(units) != 0 {
		last := units[len(units)-1]
		if last >= unicodeSpace {
			p.lastPrinted = last
			p.printed = true
		}
	}
}

func (p *vtParser) consumeUnit(unit uint16) {
	if unit == 0x18 || unit == 0x1a {
		p.interruptDCS()
		p.state = stateGround
		p.clearSequence()
		p.execute(byte(unit))
		return
	}
	if unit >= 0x80 && unit <= 0x9f {
		p.consumeUnit(0x1b)
		p.consumeUnit(unit - 0x40)
		return
	}
	if unit == 0x1b {
		if p.state == stateOSCString {
			p.state = stateOSCTermination
		} else if p.state == stateDCSPassThrough {
			p.interruptDCS()
			p.state = stateEscape
			p.clearSequence()
		} else {
			p.state = stateEscape
			p.clearSequence()
		}
		return
	}
	if unit > 0xff {
		p.consumeNonASCIIUnit(unit)
		return
	}
	p.consumeStateByte(byte(unit))
}

func (p *vtParser) consumeNonASCIIUnit(unit uint16) {
	switch p.state {
	case stateGround:
		p.printUnits([]uint16{unit})
	case stateEscape:
		if p.ansiMode {
			p.dispatchEscape(unit)
		} else {
			p.dispatchVT52(' ')
		}
	case stateEscapeIntermediate:
		if p.ansiMode {
			p.dispatchEscape(unit)
		} else {
			p.dispatchVT52(' ')
		}
	case stateCSIEntry, stateCSIParam, stateCSIIntermediate:
		// A non-ASCII wchar is neither a C0/delete, intermediate, invalid
		// parameter, nor parameter delimiter in the pinned predicates. It is
		// therefore the final wchar passed to ActionCsiDispatch.
		p.dispatchCSI(0)
		p.state = stateGround
		p.clearSequence()
	case stateCSIIgnore:
		p.state = stateGround
	case stateOSCParam:
		// _EventOscParam -> _ActionIgnore.
	case stateOSCString:
		p.oscString = append(p.oscString, unit)
	case stateOSCTermination:
		p.state = stateEscape
		p.clearSequence()
		p.consumeNonASCIIUnit(unit)
	case stateSS3Entry, stateSS3Param:
		p.dispatchSS3(unit)
	case stateVT52Param:
		p.vt52Params = append(p.vt52Params, unit)
		if len(p.vt52Params) == 2 {
			p.dispatchVT52('Y')
		}
	case stateDCSEntry, stateDCSParam, stateDCSIntermediate:
		p.dispatchDCS(unit)
	case stateDCSIgnore, stateSosPmApc:
		// ignored by the pinned state machine
	case stateDCSPassThrough:
		// StateMachine::_EventDcsPassThrough forwards both C0 controls and
		// printable ASCII to the active handler. A UTF-16 unit outside those
		// ranges takes the ignore action.
		// _EventDcsPassThrough accepts only C0 and 0x20..0x7e. A
		// non-ASCII UTF-16 unit is ignored by the pinned predicate.
	}
}

func (p *vtParser) consumeStateByte(b byte) {
	if b == 0x18 || b == 0x1a {
		p.state = stateGround
		p.clearSequence()
		return
	}
	if b == 0x1b {
		if p.state == stateOSCString {
			p.state = stateOSCTermination
		} else {
			p.state = stateEscape
			p.clearSequence()
		}
		return
	}
	if b == 0x7f && p.state != stateDCSParam {
		return
	}
	if b < 0x20 {
		switch p.state {
		case stateOSCParam:
			p.consumeOSCParam(b)
		case stateOSCString:
			p.consumeOSCString(b)
		case stateDCSEntry, stateDCSIntermediate, stateDCSIgnore, stateSosPmApc:
			// DCS and SOS/PM/APC string states ignore C0 controls.
		case stateDCSParam:
			// This is intentionally the source's two-stage branch in
			// StateMachine::_EventDcsParam: the initial C0/Delete action is
			// followed by the independent parameter/final dispatch chain.
			p.dispatchDCS(uint16(b))
		case stateDCSPassThrough:
			// _EventDcsPassThrough passes C0 controls to the active source
			// handler. RequestSetting accepts them and remains active.
			p.dcsData = append(p.dcsData, uint16(b))
		default:
			p.execute(b)
		}
		return
	}
	switch p.state {
	case stateEscape:
		p.consumeEscape(b)
	case stateEscapeIntermediate:
		if b >= 0x20 && b <= 0x2f {
			p.intermediate = append(p.intermediate, uint16(b))
		} else if b >= 0x30 && b <= 0x7e {
			if p.ansiMode {
				p.dispatchEscape(uint16(b))
			} else {
				p.dispatchVT52(b)
			}
		} else {
			p.state = stateGround
		}
	case stateCSIEntry, stateCSIParam, stateCSIIntermediate, stateCSIIgnore:
		p.consumeCSI(b)
	case stateOSCParam:
		p.consumeOSCParam(b)
	case stateOSCString:
		p.consumeOSCString(b)
	case stateOSCTermination:
		if b == '\\' {
			p.dispatchOSC()
		} else {
			// _EventOscTermination re-enters Escape and reprocesses the
			// non-terminating byte as an Escape event.
			p.state = stateEscape
			p.clearSequence()
			p.consumeEscape(b)
		}
	case stateSS3Entry:
		p.dispatchSS3(uint16(b))
	case stateDCSEntry, stateDCSParam, stateDCSIntermediate, stateDCSIgnore, stateDCSPassThrough:
		p.consumeDCS(b)
	case stateSosPmApc:
		// SOS/PM/APC strings are ignored until an ESC starts their ST.
	case stateVT52Param:
		p.vt52Params = append(p.vt52Params, uint16(b))
		if len(p.vt52Params) == 2 {
			p.dispatchVT52('Y')
		}
	}
}

func (p *vtParser) consumeEscape(b byte) {
	switch b {
	case '[':
		p.state = stateCSIEntry
		p.params = nil
		p.paramValue = 0
		p.paramStarted = false
		p.private = 0
		p.intermediate = nil
	case ']':
		p.state = stateOSCParam
		p.oscParam = 0
		p.oscDigits = nil
		p.oscString = nil
	case 'O':
		if p.ansiMode {
			// OutputStateMachineEngine::ParseControlSequenceAfterSs3 returns
			// false in the pinned output engine, so SS3 dispatches immediately as
			// ESC O and the following character is processed from Ground.
			p.dispatchEscape('O')
		} else {
			p.dispatchVT52('O')
		}
	case '=':
		p.keypadMode = true
		p.state = stateGround
		p.clearSequence()
		p.clearLastPrinted()
	case '>':
		p.keypadMode = false
		p.state = stateGround
		p.clearSequence()
		p.clearLastPrinted()
	case 'P':
		p.state = stateDCSEntry
	case 'X', '^', '_':
		p.state = stateSosPmApc
	case ' ', '#':
		p.intermediate = []uint16{uint16(b)}
		p.state = stateEscapeIntermediate
	case 'Y':
		if p.ansiMode {
			p.dispatchEscape(uint16(b))
		} else {
			p.vt52Params = nil
			p.state = stateVT52Param
		}
	case '7':
		p.buffer.saveCursor()
		p.state = stateGround
		p.clearSequence()
		p.clearLastPrinted()
	case '8':
		p.buffer.restoreCursor()
		p.state = stateGround
		p.clearSequence()
		p.clearLastPrinted()
	case 'c':
		p.reset()
	default:
		if p.ansiMode {
			p.dispatchEscape(uint16(b))
		} else {
			p.dispatchVT52(b)
		}
	}
}

func (p *vtParser) consumeDCS(b byte) {
	if p.state == stateDCSPassThrough {
		// StateMachine::_EventDcsPassThrough forwards C0 and 0x20..0x7e
		// to AdaptDispatch::RequestSetting. The handler collects only
		// intermediates and finalizes on the first final byte.
		if b < 0x20 {
			return
		}
		if b >= 0x20 && b <= 0x2f {
			p.dcsData = append(p.dcsData, uint16(b))
			return
		}
		if b >= 0x40 && b <= 0x7e {
			p.dcsData = append(p.dcsData, uint16(b))
			p.reportRequestedSetting()
			p.dcsPassThrough = false
			p.state = stateDCSIgnore
		}
		return
	}
	if p.state == stateDCSIgnore {
		return
	}
	if p.state == stateDCSEntry && b == ':' {
		p.state = stateDCSIgnore
		return
	}
	if b >= '0' && b <= '9' && p.state != stateDCSIntermediate {
		p.addParameter(b)
		p.state = stateDCSParam
		return
	}
	if b == ';' && p.state != stateDCSIntermediate {
		p.addParameter(b)
		p.state = stateDCSParam
		return
	}
	if b >= 0x20 && b <= 0x2f {
		p.intermediate = append(p.intermediate, uint16(b))
		p.state = stateDCSIntermediate
		return
	}
	if b >= 0x30 && b <= 0x3f && p.state != stateDCSEntry {
		p.state = stateDCSIgnore
		return
	}
	if p.state == stateDCSEntry && b >= 0x30 && b <= 0x3f {
		p.dispatchDCS(uint16(b))
		return
	}
	if b >= 0x40 && b <= 0x7e {
		p.dispatchDCS(uint16(b))
	} else if p.state == stateDCSParam {
		// The pinned _EventDcsParam has an independent final-dispatch chain
		// after its C0/Delete handling, so its default also receives bytes
		// outside the parameter ranges (including DEL).
		p.dispatchDCS(uint16(b))
	}
}

// reportRequestedSetting is AdaptDispatch::RequestSetting and its two
// reporting helpers. It writes to the parser response channel, which is the
// standalone equivalent of the pinned ConPTY input response path.
func (p *vtParser) reportRequestedSetting() {
	if len(p.dcsData) == 0 {
		p.responses = append(p.responses, []byte("\x1bP0$r\x1b\\"))
		return
	}
	final := rune(p.dcsData[len(p.dcsData)-1])
	intermediates := p.dcsData[:len(p.dcsData)-1]
	identifier := string(runesFromUTF16(append(append([]uint16(nil), intermediates...), uint16(final))))
	switch identifier {
	case "m":
		p.reportSGRSetting()
	case "r":
		p.reportDECSTBMSetting()
	default:
		p.responses = append(p.responses, []byte("\x1bP0$r\x1b\\"))
	}
}

func (p *vtParser) reportSGRSetting() {
	attr := p.buffer.currentAttrs
	response := "\x1bP1$r0"
	add := func(parameter string, enabled bool) {
		if enabled {
			response += parameter
		}
	}
	add(";1", attr.hasFlag(extBold))
	add(";2", attr.hasFlag(extFaint))
	add(";3", attr.hasFlag(extItalics))
	add(";4", attr.hasFlag(extUnderlined))
	add(";5", attr.hasFlag(extBlinking))
	add(";7", attr.isReverseVideo())
	add(";8", attr.hasFlag(extInvisible))
	add(";9", attr.hasFlag(extCrossedOut))
	add(";21", attr.hasFlag(extDoublyUnderlined))
	add(";53", attr.legacy&commonLVBGridHorizontal != 0)
	addColor := func(base int, color textColor) {
		switch color.kind {
		case textColorIndex16:
			index := xtermToWindowsIndex(int(color.index))
			parameter := base + int(index)%8
			if index >= 8 {
				parameter += 60
			}
			response += fmt.Sprintf(";%d", parameter)
		case textColorIndex256:
			response += fmt.Sprintf(";%d;5;%d", base+8, xterm256ToWindowsIndex(int(color.index)))
		case textColorRGB:
			red := color.rgb & 0xff
			green := (color.rgb >> 8) & 0xff
			blue := (color.rgb >> 16) & 0xff
			response += fmt.Sprintf(";%d;2;%d;%d;%d", base+8, red, green, blue)
		}
	}
	addColor(30, attr.foreground)
	addColor(40, attr.background)
	response += "m\x1b\\"
	p.responses = append(p.responses, []byte(response))
}

func (p *vtParser) reportDECSTBMSetting() {
	top := p.buffer.scrollTop + 1
	bottom := p.buffer.scrollBottom + 1
	if !p.buffer.marginsSet() {
		top = 1
		// The pinned AdaptDispatch reports the exclusive viewport height
		// (srWindow.Bottom - srWindow.Top), not the last zero-based row.
		bottom = p.buffer.viewportHeight
	}
	response := fmt.Sprintf("\x1bP1$r%d;%dr\x1b\\", top, bottom)
	p.responses = append(p.responses, []byte(response))
}

func (p *vtParser) addParameter(b byte) {
	// This is StateMachine::_ActionParam with MAX_PARAMETER_COUNT from the
	// pinned stateMachine.hpp (32). Empty parameters are represented by zero
	// here; the dispatch defaults in this probe treat zero as an omitted value.
	if p.parameterLimit {
		return
	}
	if len(p.params) == 0 {
		p.params = append(p.params, 0)
	}
	if b == ';' {
		if len(p.params) >= 32 {
			p.parameterLimit = true
			return
		}
		p.params = append(p.params, 0)
		p.paramValue = 0
		p.paramStarted = false
		return
	}
	p.paramValue = p.paramValue*10 + int(b-'0')
	p.params[len(p.params)-1] = p.paramValue
	p.paramStarted = true
}

func (p *vtParser) dispatchDCS(final uint16) {
	// OutputStateMachineEngine::ActionDcsDispatch recognizes DECRQSS (`$q`)
	// and returns a string handler. Other identifiers return nullptr and enter
	// DcsIgnore.
	if string(runesFromUTF16(p.intermediate)) == "$" && final == 'q' {
		p.dcsPassThrough = true
		p.dcsData = nil
		p.intermediate = nil
		p.state = stateDCSPassThrough
	} else {
		p.state = stateDCSIgnore
	}
}

func (p *vtParser) interruptDCS() {
	if p.state == stateDCSPassThrough {
		// StateMachine::_ActionInterrupt sends ESC to the source handler and
		// clears it. RequestSetting's response is not screen text.
		p.dcsPassThrough = false
	}
}

func (p *vtParser) dispatchEscape(final uint16) {
	identifier := append(append([]uint16(nil), p.intermediate...), final)
	switch string(runesFromUTF16(identifier)) {
	case "#8":
		for y := 0; y < p.buffer.height; y++ {
			for x := 0; x < p.buffer.width; x++ {
				p.buffer.rowByOffset(y).charRow.setGlyph(x, []uint16{'E'})
			}
		}
	case "#3":
		p.buffer.setCurrentLineRendition(lineRenditionDoubleHeightTop)
	case "#4":
		p.buffer.setCurrentLineRendition(lineRenditionDoubleHeightBottom)
	case "#5":
		p.buffer.setCurrentLineRendition(lineRenditionSingle)
	case "#6":
		p.buffer.setCurrentLineRendition(lineRenditionDoubleWidth)
	case "D":
		if err := p.buffer.lineFeed(false); err != nil {
			p.failed = err
		}
	case "E":
		p.buffer.carriageReturn()
		if err := p.buffer.lineFeed(true); err != nil {
			p.failed = err
		}
	case "M":
		p.reverseLineFeed()
	case "H":
		p.buffer.setTab(p.buffer.cursor.position.x)
	case "c":
		p.reset()
	}
	p.state = stateGround
	p.clearSequence()
	p.clearLastPrinted()
}

func (p *vtParser) dispatchVT52(final byte) {
	switch final {
	case 'A':
		p.buffer.cursorMove(-1, 0, false, false, true)
	case 'B':
		p.buffer.cursorMove(1, 0, false, false, true)
	case 'C':
		p.buffer.cursorMove(0, 1, false, false, true)
	case 'D':
		p.buffer.cursorMove(0, -1, false, false, true)
	case 'H':
		p.buffer.cursorMove(0, 0, true, true, false)
	case 'I':
		p.reverseLineFeed()
	case 'J':
		p.eraseDisplay(0)
	case 'K':
		p.eraseLine(0)
	case 'Y':
		if len(p.vt52Params) == 2 {
			row := int(p.vt52Params[0]) - int(' ')
			col := int(p.vt52Params[1]) - int(' ')
			p.buffer.cursorMove(row, col, true, true, false)
		}
	case '<':
		p.ansiMode = true
	case 'O', 'F', 'G', 'Z', '=', '>':
		// These pinned OutputStateMachineEngine actions affect character
		// sets, device identification, or keypad state, none of which changes
		// the text buffer in this probe.
	}
	p.state = stateGround
	p.vt52Params = nil
	p.clearSequence()
	p.clearLastPrinted()
}

func (p *vtParser) consumeCSI(b byte) {
	if p.state == stateCSIIgnore {
		if b >= 0x40 && b <= 0x7e {
			p.state = stateGround
		}
		return
	}
	if b >= 0x30 && b <= 0x39 && p.state != stateCSIIntermediate {
		p.addParameter(b)
		p.state = stateCSIParam
		return
	}
	if b == ';' && p.state != stateCSIIntermediate {
		p.addParameter(b)
		p.state = stateCSIParam
		return
	}
	if b >= '<' && b <= '?' && p.state == stateCSIEntry {
		p.private = b
		p.state = stateCSIParam
		return
	}
	if b >= 0x20 && b <= 0x2f {
		p.intermediate = append(p.intermediate, uint16(b))
		p.state = stateCSIIntermediate
		return
	}
	if b >= 0x40 && b <= 0x7e {
		p.dispatchCSI(b)
		p.state = stateGround
		p.clearSequence()
		return
	}
	p.state = stateCSIIgnore
}

func (p *vtParser) consumeOSCParam(b byte) {
	if b >= '0' && b <= '9' {
		// StateMachine::_ActionOscParam accumulates into size_t one digit at
		// a time. Do not cap the textual digits: that would change overflow
		// and delimiter behavior relative to the pinned source.
		p.oscParam = p.oscParam*10 + int(b-'0')
		return
	}
	if b == ';' {
		p.state = stateOSCString
		return
	}
	if b == 0x07 {
		// In OscParam, BEL terminates the incomplete sequence without
		// dispatching it.  Dispatch only occurs after the ';' delimiter.
		p.state = stateGround
		return
	}
	// _EventOscParam ignores all other non-parameter bytes and remains in
	// OscParam until a later escape/control sequence interrupts it.
}

func (p *vtParser) consumeOSCString(b byte) {
	if b == 0x07 {
		p.dispatchOSC()
		return
	}
	if b == 0x1b {
		p.state = stateOSCTermination
		return
	}
	if b <= 0x17 || b == 0x19 || (b >= 0x1c && b <= 0x1f) {
		return
	}
	if len(p.oscString) < 1<<20 {
		p.oscString = append(p.oscString, uint16(b))
	}
}

func (p *vtParser) dispatchOSC() {
	if p.oscParam == 0 || p.oscParam == 1 || p.oscParam == 2 {
		p.title = string(runesFromUTF16(p.oscString))
	}
	p.state = stateGround
	p.clearSequence()
	p.clearLastPrinted()
}

func (p *vtParser) dispatchSS3(b uint16) {
	switch b {
	case 'A':
		p.moveCursor(-1, 0)
	case 'B':
		p.moveCursor(1, 0)
	case 'C':
		p.moveCursor(0, 1)
	case 'D':
		p.moveCursor(0, -1)
	}
	p.state = stateGround
	p.clearLastPrinted()
}

func (p *vtParser) dispatchCSI(final byte) {
	parameterValues := func() []int {
		if len(p.params) == 0 {
			return []int{0}
		}
		return p.params
	}
	n := func(index, defaultValue int) int {
		if index >= len(p.params) || p.params[index] == 0 {
			return defaultValue
		}
		return p.params[index]
	}
	raw := func(index int) int {
		if index >= len(p.params) {
			return 0
		}
		return p.params[index]
	}
	switch final {
	case 'A':
		p.buffer.cursorMove(-n(0, 1), 0, false, false, true)
	case 'B', 'e':
		p.buffer.cursorMove(n(0, 1), 0, false, false, final == 'B')
	case 'C', 'a':
		p.buffer.cursorMove(0, n(0, 1), false, false, final == 'C')
	case 'D':
		p.buffer.cursorMove(0, -n(0, 1), false, false, true)
	case 'E':
		p.buffer.cursorMove(n(0, 1), 0, false, true, true)
	case 'F':
		p.buffer.cursorMove(-n(0, 1), 0, false, true, true)
	case 'G', '`':
		p.buffer.cursorMove(0, n(0, 1)-1, false, true, false)
	case 'd':
		p.buffer.cursorMove(n(0, 1)-1, 0, true, false, false)
	case 'H', 'f':
		p.buffer.cursorMove(n(0, 1)-1, n(1, 1)-1, true, true, false)
	case 'J':
		for _, mode := range parameterValues() {
			p.eraseDisplay(mode)
		}
	case 'K':
		for _, mode := range parameterValues() {
			p.eraseLine(mode)
		}
	case 'I':
		p.buffer.tabForward(n(0, 1))
	case 'Z':
		p.buffer.tabBackward(n(0, 1))
	case 'g':
		for _, mode := range parameterValues() {
			p.buffer.clearTabs(mode)
		}
	case 'X':
		p.buffer.clearRange(p.buffer.cursor.position.y, p.buffer.cursor.position.x, p.buffer.cursor.position.x+n(0, 1)-1)
	case '@':
		p.insertCells(n(0, 1))
	case 'P':
		p.deleteCells(n(0, 1))
	case 'L':
		p.insertLines(n(0, 1))
	case 'M':
		p.deleteLines(n(0, 1))
	case 'S':
		p.scrollUp(n(0, 1))
	case 'T':
		p.scrollDown(n(0, 1))
	case 'r':
		p.buffer.setScrollingMargins(raw(0), raw(1))
	case 's':
		if len(p.params) == 0 {
			p.buffer.saveCursor()
		}
	case 'u':
		if len(p.params) == 0 {
			p.buffer.restoreCursor()
		}
	case 'm':
		p.setGraphicsRendition(parameterValues())
	case 'h', 'l':
		p.privateMode(final == 'h', p.params)
	case 'b':
		p.repeatPrevious(n(0, 1))
	case 'c':
		if p.private == 0 && raw(0) == 0 {
			p.responses = append(p.responses, []byte("\x1b[?1;0c"))
		}
	case 'q':
		p.setCursorStyle(raw(0))
	case 'n':
		p.deviceStatusReport(raw(0))
	case 'p':
		if len(p.intermediate) == 0 && p.private == 0 {
			p.softReset()
		}
	}
	p.clearLastPrinted()
}

func (p *vtParser) privateMode(enable bool, params []int) {
	if p.private != '?' {
		return
	}
	for _, mode := range params {
		switch mode {
		case 1:
			p.cursorKeysMode = enable
		case 6:
			p.buffer.originMode = enable
			p.buffer.cursorMove(0, 0, true, true, false)
		case 2:
			p.ansiMode = enable
		case 7:
			p.buffer.wrapAtEOL = enable
		case 3:
			if p.deccolmSupport {
				width := 80
				if enable {
					width = 132
				}
				if resized, err := resizeBuffer(p.buffer, width, p.buffer.height); err == nil {
					p.replaceActiveBuffer(resized)
				} else {
					p.failed = err
				}
			}
		case 5:
			p.screenMode = enable
		case 12:
			p.buffer.cursor.blinkingAllowed = enable
		case 25:
			p.buffer.cursor.visible = enable
		case 40:
			p.deccolmSupport = enable
		case 1000, 1002, 1003, 1005, 1006, 1007:
			if p.mouseModes == nil {
				p.mouseModes = make(map[int]bool)
			}
			p.mouseModes[mode] = enable
		case 2004:
			p.bracketedPaste = enable
		case 9001:
			p.win32InputMode = enable
		case 1049:
			if enable {
				p.useAlternate()
			} else {
				p.useMain()
			}
		}
	}
}

func (p *vtParser) setGraphicsRendition(options []int) {
	attr := p.buffer.currentAttrs
	for i := 0; i < len(options); i++ {
		option := options[i]
		switch {
		case option == 0:
			attr.setDefaultForeground()
			attr.setDefaultBackground()
			attr.setDefaultMetaAttrs()
		case option == 1:
			attr.setBold(true)
		case option == 2:
			attr.setFaint(true)
		case option == 3:
			attr.setItalic(true)
		case option == 4:
			attr.setUnderlined(true)
		case option == 5 || option == 6:
			attr.setBlinking(true)
		case option == 7:
			attr.setReverseVideo(true)
		case option == 8:
			attr.setInvisible(true)
		case option == 9:
			attr.setCrossedOut(true)
		case option == 21:
			attr.setDoublyUnderlined(true)
		case option == 22:
			attr.setBold(false)
			attr.setFaint(false)
		case option == 23:
			attr.setItalic(false)
		case option == 24:
			attr.setUnderlined(false)
			attr.setDoublyUnderlined(false)
		case option == 25:
			attr.setBlinking(false)
		case option == 27:
			attr.setReverseVideo(false)
		case option == 28:
			attr.setInvisible(false)
		case option == 29:
			attr.setCrossedOut(false)
		case option >= 30 && option <= 37:
			attr.setIndexedForeground(uint8(option - 30))
		case option == 38:
			consumed := 1
			if i+1 < len(options) && options[i+1] == 2 {
				red, green, blue := parameterAt(options, i+2), parameterAt(options, i+3), parameterAt(options, i+4)
				if red <= 255 && green <= 255 && blue <= 255 {
					attr.setColor(uint32(red)|uint32(green)<<8|uint32(blue)<<16, true)
				}
				// AdaptDispatch::_SetRgbColorsHelper consumes all three RGB
				// slots even when the VT parameter vector omits them.
				consumed = 4
			} else if i+1 < len(options) && options[i+1] == 5 {
				index := parameterAt(options, i+2)
				if index <= 255 {
					attr.setIndexedForeground256(uint8(index))
				}
				consumed = 2
			}
			i += consumed
		case option == 39:
			attr.setDefaultForeground()
		case option >= 40 && option <= 47:
			attr.setIndexedBackground(uint8(option - 40))
		case option == 48:
			consumed := 1
			if i+1 < len(options) && options[i+1] == 2 {
				red, green, blue := parameterAt(options, i+2), parameterAt(options, i+3), parameterAt(options, i+4)
				if red <= 255 && green <= 255 && blue <= 255 {
					attr.setColor(uint32(red)|uint32(green)<<8|uint32(blue)<<16, false)
				}
				consumed = 4
			} else if i+1 < len(options) && options[i+1] == 5 {
				index := parameterAt(options, i+2)
				if index <= 255 {
					attr.setIndexedBackground256(uint8(index))
				}
				consumed = 2
			}
			i += consumed
		case option == 49:
			attr.setDefaultBackground()
		case option == 53:
			attr.setOverlined(true)
		case option == 55:
			attr.setOverlined(false)
		case option >= 90 && option <= 97:
			attr.setIndexedForeground(uint8(option - 90 + 8))
		case option >= 100 && option <= 107:
			attr.setIndexedBackground(uint8(option - 100 + 8))
		}
	}
	p.buffer.currentAttrs = attr
}

func parameterAt(options []int, index int) int {
	if index < 0 || index >= len(options) {
		return 0
	}
	return options[index]
}

func (p *vtParser) setCursorStyle(style int) {
	p.buffer.cursor.blinkingAllowed = false
	switch style {
	case 0:
		p.buffer.cursor.style = cursorLegacy
		p.buffer.cursor.blinkingAllowed = true
	case 1:
		p.buffer.cursor.style = cursorFullBox
		p.buffer.cursor.blinkingAllowed = true
	case 2:
		p.buffer.cursor.style = cursorFullBox
	case 3:
		p.buffer.cursor.style = cursorUnderscore
		p.buffer.cursor.blinkingAllowed = true
	case 4:
		p.buffer.cursor.style = cursorUnderscore
	case 5:
		p.buffer.cursor.style = cursorVerticalBar
		p.buffer.cursor.blinkingAllowed = true
	case 6:
		p.buffer.cursor.style = cursorVerticalBar
	}
}

func (p *vtParser) deviceStatusReport(status int) {
	if p.private != 0 {
		return
	}
	switch status {
	case 5:
		p.responses = append(p.responses, []byte("\x1b[0n"))
	case 6:
		position := p.buffer.cursor.position
		row := position.y + 1
		if p.buffer.originMode && p.buffer.marginsSet() {
			top, _ := p.buffer.absoluteScrollMargins()
			row -= top
		}
		p.responses = append(p.responses, []byte(fmt.Sprintf("\x1b[%d;%dR", row, position.x+1)))
	}
}

func (p *vtParser) softReset() {
	p.buffer.wrapAtEOL = true
	p.buffer.originMode = false
	p.cursorKeysMode = false
	p.keypadMode = false
	p.buffer.scrollTop = 0
	p.buffer.scrollBottom = 0
	p.buffer.cursor.visible = true
	p.buffer.cursor.blinkingAllowed = true
	p.buffer.currentAttrs = textAttribute{}
	p.setGraphicsRendition([]int{0})
	p.buffer.savedCursorState = cursorState{}
	if p.altBuffer != nil {
		p.altBuffer.savedCursorState = cursorState{}
	}
}

func (p *vtParser) moveCursor(dy, dx int) {
	p.buffer.cursorMove(dy, dx, false, false, true)
}

func (p *vtParser) eraseDisplay(mode int) {
	pos := p.buffer.cursor.position
	eraseFullLine := func(row int) {
		p.buffer.rowByOffset(row).lineRendition = lineRenditionSingle
		p.buffer.clearRangeWithAttr(row, 0, p.buffer.lineWidth(row)-1, p.standardErase())
	}
	switch mode {
	case 0:
		if pos.x == 0 {
			eraseFullLine(pos.y)
		} else {
			p.buffer.clearRangeWithAttr(pos.y, pos.x, p.buffer.lineWidth(pos.y)-1, p.standardErase())
		}
		for y := pos.y + 1; y < p.buffer.height; y++ {
			eraseFullLine(y)
		}
	case 1:
		for y := 0; y < pos.y; y++ {
			eraseFullLine(y)
		}
		p.buffer.clearRangeWithAttr(pos.y, 0, pos.x, p.standardErase())
	case 2, 3:
		for y := 0; y < p.buffer.height; y++ {
			eraseFullLine(y)
		}
	}
}

func (p *vtParser) eraseLine(mode int) {
	pos := p.buffer.cursor.position
	switch mode {
	case 0:
		p.buffer.clearRangeWithAttr(pos.y, pos.x, p.buffer.lineWidth(pos.y)-1, p.standardErase())
	case 1:
		p.buffer.clearRangeWithAttr(pos.y, 0, pos.x, p.standardErase())
	case 2:
		p.buffer.rowByOffset(pos.y).lineRendition = lineRenditionSingle
		p.buffer.clearRangeWithAttr(pos.y, 0, p.buffer.lineWidth(pos.y)-1, p.standardErase())
	}
}

func (p *vtParser) insertCells(count int) {
	row := p.buffer.rowByOffset(p.buffer.cursor.position.y)
	start := p.buffer.cursor.position.x
	lineWidth := p.buffer.lineWidth(p.buffer.cursor.position.y)
	if count > lineWidth-start {
		count = lineWidth - start
	}
	p.shiftCells(row, start, count, true)
}

func (p *vtParser) deleteCells(count int) {
	row := p.buffer.rowByOffset(p.buffer.cursor.position.y)
	start := p.buffer.cursor.position.x
	lineWidth := p.buffer.lineWidth(p.buffer.cursor.position.y)
	if count > lineWidth-start {
		count = lineWidth - start
	}
	p.shiftCells(row, start, count, false)
}

// shiftCells is the cell-wise equivalent of the pinned PrivateScrollRegion
// horizontal move. Copying through glyphAt/setGlyph is required because a
// direct Go struct assignment would leave UnicodeStorage keyed to the old
// column rather than performing ROW's cell-reference copy.
func (p *vtParser) shiftCells(row *msRow, start, count int, insert bool) {
	if count <= 0 || start < 0 || start >= len(row.charRow.data) {
		return
	}
	end := p.buffer.lineWidth(p.buffer.cursor.position.y)
	if end > len(row.charRow.data) {
		end = len(row.charRow.data)
	}
	oldGlyphs := make([][]uint16, end)
	oldAttrs := make([]dbcsAttribute, end)
	oldTextAttrs := append([]textAttribute(nil), row.attrs[:end]...)
	for x := 0; x < end; x++ {
		oldGlyphs[x] = row.charRow.glyphAt(x)
		oldAttrs[x] = row.charRow.data[x].attr
		row.charRow.clearCell(x)
	}
	for x := 0; x < end; x++ {
		source := x
		if insert {
			source = x - count
			if x >= start && x < start+count {
				source = -1
			}
		} else if x >= start {
			source = x + count
			if source >= end {
				source = -1
			}
		}
		if x < start && insert || x < start && !insert {
			source = x
		}
		if source >= 0 && source < end {
			row.charRow.setGlyph(x, oldGlyphs[source])
			row.charRow.data[x].attr = oldAttrs[source]
			row.attrs[x] = oldTextAttrs[source]
		} else {
			row.charRow.clearCell(x)
			row.attrs[x] = p.standardErase()
		}
	}
}

func (p *vtParser) insertLines(count int) {
	y := p.buffer.cursor.position.y
	bottom := p.buffer.viewportBottom()
	if p.buffer.marginsSet() {
		top, marginBottom := p.buffer.absoluteScrollMargins()
		if y >= top && y <= marginBottom {
			bottom = marginBottom
		}
	}
	if count > bottom-y+1 {
		count = bottom - y + 1
	}
	p.buffer.scrollRegion(y, bottom, count, true)
	p.buffer.carriageReturn()
}

func (p *vtParser) deleteLines(count int) {
	y := p.buffer.cursor.position.y
	bottom := p.buffer.viewportBottom()
	if p.buffer.marginsSet() {
		top, marginBottom := p.buffer.absoluteScrollMargins()
		if y >= top && y <= marginBottom {
			bottom = marginBottom
		}
	}
	if count > bottom-y+1 {
		count = bottom - y + 1
	}
	p.buffer.scrollRegion(y, bottom, count, false)
	p.buffer.carriageReturn()
}

func (p *vtParser) scrollUp(count int) {
	p.buffer.scrollRegionUp(count)
}

func (p *vtParser) scrollDown(count int) {
	p.buffer.scrollRegionDown(count)
}

func (p *vtParser) reverseLineFeed() {
	old := p.buffer.cursor.position
	viewportTop := p.buffer.viewportTop
	newPosition := coordinate{x: old.x, y: old.y - 1}
	newPosition = p.buffer.clampPositionWithinLine(newPosition)
	if old.y > viewportTop {
		if err := adjustCursorPosition(p.buffer, newPosition, true); err != nil {
			p.failed = err
		}
		return
	}
	// At the top of the viewport, RI inserts a blank row only when the
	// cursor is in the active scrolling region. With no margins that is the
	// entire viewport; with margins it is exactly the margin rectangle.
	if !p.buffer.marginsSet() {
		p.buffer.scrollRegion(viewportTop, p.buffer.viewportBottom(), 1, true)
	} else {
		top, bottom := p.buffer.absoluteScrollMargins()
		if old.y >= top && old.y <= bottom {
			p.buffer.scrollRegion(top, bottom, 1, true)
		}
	}
}

func (p *vtParser) useAlternate() {
	// AdaptDispatch::UseAlternateScreenBuffer first saves the active cursor,
	// then SCREEN_INFORMATION::_CreateAltBuffer creates an initially erased
	// buffer with the main viewport dimensions and copies only the cursor style,
	// visibility, blinking policy, and viewport-relative position.
	p.buffer.saveCursor()
	main := p.mainBuffer
	height := main.viewportHeight
	if height <= 0 {
		height = main.height
	}
	initAttributes := main.currentAttrs
	initAttributes.setStandardErase()
	p.altBuffer = newTextBufferWithAttributes(main.width, height, initAttributes)
	p.altBuffer.vtMode = main.vtMode
	p.altBuffer.cursor.size = main.cursor.size
	p.altBuffer.cursor.style = main.cursor.style
	p.altBuffer.cursor.color = main.cursor.color
	p.altBuffer.cursor.visible = main.cursor.visible
	p.altBuffer.cursor.blinkingAllowed = main.cursor.blinkingAllowed
	altPosition := main.cursor.position
	altPosition.y -= main.virtualViewportTop()
	p.altBuffer.cursor.position = altPosition
	p.altBuffer.cursor.delayed = false
	p.buffer = p.altBuffer
}

func (p *vtParser) useMain() {
	// SCREEN_INFORMATION::UseMainScreenBuffer copies the alternate cursor's
	// style/visibility/blinking policy to main. AdaptDispatch then restores the
	// saved main cursor state (position, origin, rendition).
	if p.buffer == p.mainBuffer {
		return
	}
	alt := p.altBuffer
	p.buffer = p.mainBuffer
	if alt != nil {
		p.mainBuffer.cursor.size = alt.cursor.size
		p.mainBuffer.cursor.style = alt.cursor.style
		p.mainBuffer.cursor.color = alt.cursor.color
		p.mainBuffer.cursor.visible = alt.cursor.visible
		p.mainBuffer.cursor.blinkingAllowed = alt.cursor.blinkingAllowed
	}
	p.mainBuffer.restoreCursor()
	p.altBuffer = nil
}

func (p *vtParser) printRune(r rune) {
	units := utf16Units(string(r))
	p.buffer.returnOnNewline = p.newLineAutoReturn
	if err := writeDefaultString(p.buffer, units); err != nil {
		p.failed = err
		return
	}
	if len(units) != 0 && units[len(units)-1] >= unicodeSpace {
		// OutputStateMachineEngine::_lastPrintedChar stores the last UTF-16
		// code unit, not a Unicode scalar or grapheme.
		p.lastPrinted = units[len(units)-1]
		p.printed = true
	}
}

func (p *vtParser) consumeRune(r rune) {
	if r >= 0x80 && r <= 0x9f {
		p.consumeStateByte(0x1b)
		p.consumeStateByte(byte(r - 0x40))
		return
	}
	p.printRune(r)
}

func (p *vtParser) repeatPrevious(count int) {
	if !p.printed {
		return
	}
	if count < 0 {
		count = 0
	}
	units := make([]uint16, count)
	for i := range units {
		units[i] = p.lastPrinted
	}
	if err := writeDefaultString(p.buffer, units); err != nil {
		p.failed = err
	}
}

func (p *vtParser) execute(b byte) {
	switch b {
	case 0x00:
		// OutputStateMachineEngine::ActionExecute explicitly filters NUL.
	case 0x7f:
		// AdaptDispatch::Print filters DEL unless a designated 96-character
		// set translates it. The probe keeps the pinned default ASCII set.
	case 0x07:
		p.bells++
	case 0x08:
		p.buffer.backspace()
	case 0x09:
		p.buffer.tab()
	case 0x0a, 0x0b, 0x0c:
		if err := p.buffer.lineFeed(p.newLineAutoReturn); err != nil {
			p.failed = err
		}
	case 0x0d:
		p.buffer.carriageReturn()
	case 0x0e, 0x0f:
		// SI/SO are LockingShift callbacks in the pinned output engine.
		// The text buffer has no charset state, so the callback has no text
		// side effect.
	case 0x9b:
		p.state = stateCSIEntry
	case 0x9d:
		p.state = stateOSCParam
	default:
		if b >= 0x80 && b <= 0x9f {
			// C1 controls not used by the probe are consumed by the pinned
			// parser's execute action.
		} else {
			// OutputStateMachineEngine::ActionExecute routes unhandled C0
			// characters to ITermDispatch::Print.
			if err := writeDefaultString(p.buffer, []uint16{uint16(b)}); err != nil {
				p.failed = err
			}
		}
	}
	p.clearLastPrinted()
}

func (p *vtParser) clearLastPrinted() {
	p.lastPrinted = 0
	p.printed = false
}

func (p *vtParser) clearSequence() {
	p.params = nil
	p.paramValue = 0
	p.paramStarted = false
	p.parameterLimit = false
	p.private = 0
	p.intermediate = nil
	p.oscParam = 0
	p.oscDigits = nil
	p.oscString = nil
	p.dcsPassThrough = false
	p.dcsData = nil
}

func (p *vtParser) snapshot() terminalSnapshot {
	return terminalSnapshot{Text: p.buffer.text(), Cursor: p.buffer.cursor.position, Width: p.buffer.width, Height: p.buffer.height, Title: p.title, Bells: p.bells}
}

func (p *vtParser) resize(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid resize %dx%d", width, height)
	}
	oldBuffer := p.buffer
	resized, err := resizeBuffer(oldBuffer, width, height)
	if err != nil {
		return err
	}
	p.replaceActiveBuffer(resized)
	return nil
}

func (p *vtParser) replaceActiveBuffer(resized *textBuffer) {
	oldBuffer := p.buffer
	p.buffer = resized
	if oldBuffer == p.mainBuffer {
		p.mainBuffer = resized
	} else if oldBuffer == p.altBuffer {
		p.altBuffer = resized
	}
}

func resizeBuffer(old *textBuffer, width, height int) (*textBuffer, error) {
	newBuffer := newTextBuffer(width, height)
	newBuffer.vtMode = old.vtMode
	newBuffer.wrapAtEOL = old.wrapAtEOL
	newBuffer.processedOutput = old.processedOutput
	newBuffer.returnOnNewline = old.returnOnNewline
	// SCREEN_INFORMATION::ResizeWithReflow constructs the replacement buffer
	// with default attributes.  The old current attributes are restored only
	// after TextBuffer::Reflow, so overflow rows are erased with the new
	// buffer's defaults exactly as in the pinned path.
	cursorHeightBefore := old.cursor.position.y - old.viewportTop
	if err := reflow(old, newBuffer); err != nil {
		return nil, err
	}
	newBuffer.currentAttrs = old.currentAttrs
	newBuffer.cursorSize = old.cursorSize
	newBuffer.viewportHeight = height
	if newBuffer.viewportHeight > newBuffer.height {
		newBuffer.viewportHeight = newBuffer.height
	}
	if newBuffer.viewportHeight < 1 {
		newBuffer.viewportHeight = 1
	}
	newBuffer.viewportTop = old.viewportTop + (newBuffer.cursor.position.y - cursorHeightBefore - old.viewportTop)
	if maxTop := newBuffer.height - newBuffer.viewportHeight; newBuffer.viewportTop > maxTop {
		newBuffer.viewportTop = maxTop
	}
	if newBuffer.viewportTop < 0 {
		newBuffer.viewportTop = 0
	}
	newBuffer.virtualBottom = newBuffer.viewportBottom()
	return newBuffer, nil
}

type terminalSnapshot struct {
	Text   string
	Cursor coordinate
	Width  int
	Height int
	Title  string
	Bells  int
}

func normalizeVTData(data []byte) []byte { return bytes.Clone(data) }
