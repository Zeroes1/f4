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
	"strings"
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

// vtIDBuilder is the Go transcription of DispatchTypes::VTIDBuilder. VTIDs
// are little-endian byte accumulators with an eight-byte ceiling; adding an
// eighth intermediate clears the accumulator without advancing the shift,
// exactly as the pinned source does.
type vtIDBuilder struct {
	accumulator uint64
	shift       uint
}

func (b *vtIDBuilder) clear() {
	b.accumulator = 0
	b.shift = 0
}

func (b *vtIDBuilder) addIntermediate(unit uint16) {
	if b.shift+8 >= 64 {
		b.accumulator = 0
	} else {
		b.accumulator += uint64(unit) << b.shift
		b.shift += 8
	}
}

func (b vtIDBuilder) finalize(final uint16) uint64 {
	return b.accumulator + (uint64(final) << b.shift)
}

func vtIDFromString(value string) uint64 {
	var result uint64
	for index := len(value) - 1; index >= 0; index-- {
		result = (result << 8) + uint64(value[index])
	}
	return result
}

type vtParser struct {
	buffer            *textBuffer
	mainBuffer        *textBuffer
	altBuffer         *textBuffer
	state             parserState
	termOutput        terminalOutput
	savedCursorState  [2]dispatchCursorState
	params            []int
	paramValue        int
	paramStarted      bool
	parameterLimit    bool
	private           byte
	intermediate      []uint16
	identifier        vtIDBuilder
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
	colorTable        map[uint8]uint32
	defaultForeground uint32
	defaultBackground uint32
	bells             int
	lastPrinted       uint16
	printed           bool
	sgrStack          sgrStackState
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

// dispatchCursorState is AdaptDispatch::_savedCursorState. Coordinates are
// stored zero-based after converting the source's one-based VT coordinates;
// the saved origin, attributes, and TerminalOutput state remain separate from
// TextBuffer::Cursor, exactly as in the pinned adapter.
type dispatchCursorState struct {
	position   coordinate
	originMode bool
	attrs      textAttribute
	termOutput terminalOutput
}

func (p *vtParser) standardErase() textAttribute {
	erase := p.buffer.currentAttrs
	erase.setStandardErase()
	return erase
}

func newVTParser(width, height int) *vtParser {
	b := newTextBuffer(width, height)
	b.vtMode = true
	return &vtParser{buffer: b, mainBuffer: b, state: stateGround, termOutput: newTerminalOutput(), wideParser: newUTF8WideParser(), newLineAutoReturn: true, ansiMode: true, mouseModes: make(map[int]bool), colorTable: make(map[uint8]uint32), defaultForeground: 0xffffffff, defaultBackground: 0xffffffff}
}

func (p *vtParser) reset() {
	p.buffer = newTextBuffer(p.buffer.width, p.buffer.height)
	p.mainBuffer = p.buffer
	p.altBuffer = nil
	p.state = stateGround
	p.termOutput = newTerminalOutput()
	p.savedCursorState = [2]dispatchCursorState{}
	p.params = nil
	p.paramValue = 0
	p.paramStarted = false
	p.parameterLimit = false
	p.private = 0
	p.intermediate = nil
	p.identifier.clear()
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
	p.sgrStack = sgrStackState{}
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
	if len(data) != 0 {
		p.buffer.moveToBottom()
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
		p.consumeUnit(units[0])
		units = units[1:]
	}
}

func isActionableGroundUnit(unit uint16) bool {
	return unit <= 0x1f || unit == 0x7f || unit >= 0x80 && unit <= 0x9f
}

func (p *vtParser) printUnits(units []uint16) {
	var last uint16
	if len(units) != 0 {
		// OutputStateMachineEngine::ActionPrintString records the source
		// wchar before WriteBuffer performs any character-set translation.
		last = units[len(units)-1]
	}
	p.buffer.returnOnNewline = p.newLineAutoReturn
	if p.termOutput.needToTranslate() {
		translated := make([]uint16, len(units))
		for i, unit := range units {
			translated[i] = p.termOutput.translateKey(unit)
		}
		units = translated
	}
	if err := writeDefaultString(p.buffer, units); err != nil {
		p.failed = err
		return
	}
	if last >= unicodeSpace {
		p.lastPrinted = last
		p.printed = true
	}
}

func (p *vtParser) consumeUnit(unit uint16) {
	if unit == 0x18 || unit == 0x1a {
		// OutputStateMachineEngine::DispatchControlCharsFromEscape returns
		// false in the pinned source.  Consequently CAN/SUB in Escape is
		// handled by _EventEscape as an ordinary execute action and leaves
		// the parser in Escape; every other state takes the from-anywhere
		// interrupt/execute/ground path.
		if p.state == stateEscape {
			p.execute(byte(unit))
			return
		}
		p.interruptDCS()
		p.state = stateGround
		// StateMachine::_EnterGround only changes the state.  It does not call
		// ActionClear; the from-anywhere CAN/SUB path therefore leaves the
		// collected sequence data untouched until the next dispatch/reset path.
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
			p.dispatchVT52(unit)
		}
	case stateEscapeIntermediate:
		if p.ansiMode {
			p.dispatchEscape(unit)
		} else {
			p.dispatchVT52(unit)
		}
	case stateCSIEntry, stateCSIParam, stateCSIIntermediate:
		// A non-ASCII wchar is neither a C0/delete, intermediate, invalid
		// parameter, nor parameter delimiter in the pinned predicates. It is
		// therefore the final wchar passed to ActionCsiDispatch.
		p.dispatchCSI(unit)
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
	// StateMachine::_EventOscTermination re-enters Escape and reprocesses any
	// non-ST character through _EventEscape. This branch has to precede the
	// generic C0/DEL dispatch below: Escape deliberately keeps C0 execution in
	// Escape, while OSC termination itself is no longer active.
	if p.state == stateOSCTermination {
		if b == '\\' {
			p.dispatchOSC()
			return
		}
		p.state = stateEscape
		p.clearSequence()
		if b == 0x7f {
			return
		}
		if b < 0x20 {
			p.execute(b)
			return
		}
		p.consumeEscape(b)
		return
	}
	// DEL is state-dependent in StateMachine: Ground executes it (the output
	// adapter then filters it), OSCString stores it, DCSParam reaches the
	// source's fall-through dispatch after its Ignore action, and the other
	// states ignore it.
	if b == 0x7f {
		switch p.state {
		case stateGround:
			p.execute(b)
		case stateOSCString:
			p.consumeOSCString(b)
		case stateDCSParam:
			p.dispatchDCS(uint16(b))
		}
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
			// handler. AdaptDispatch::RequestSetting's handler does not add
			// C0 controls to the VTID, and remains active.
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
			p.identifier.addIntermediate(uint16(b))
		} else if b >= 0x30 && b <= 0x7e {
			if p.ansiMode {
				p.dispatchEscape(uint16(b))
			} else {
				p.dispatchVT52(uint16(b))
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
	// StateMachine::_EventEscape treats every 0x20..0x2f character as an
	// intermediate.  The parser must enter EscapeIntermediate before the final
	// character; handling only the currently exercised two values would change
	// the identifier for the other designated character sets.
	if b >= 0x20 && b <= 0x2f {
		p.intermediate = append(p.intermediate, uint16(b))
		p.identifier.addIntermediate(uint16(b))
		p.state = stateEscapeIntermediate
		return
	}
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
		p.dispatchEscape(uint16(b))
	case '>':
		p.dispatchEscape(uint16(b))
	case 'P':
		p.state = stateDCSEntry
	case 'X', '^', '_':
		p.state = stateSosPmApc
	case 'Y':
		if p.ansiMode {
			p.dispatchEscape(uint16(b))
		} else {
			p.vt52Params = nil
			p.state = stateVT52Param
		}
	case '7':
		p.saveCursorState()
		p.state = stateGround
		p.clearSequence()
		p.clearLastPrinted()
	case '8':
		p.restoreCursorState()
		p.state = stateGround
		p.clearSequence()
		p.clearLastPrinted()
	case 'c':
		if p.ansiMode {
			p.dispatchEscape(uint16(b))
		} else {
			p.dispatchVT52(uint16(b))
		}
	default:
		if p.ansiMode {
			p.dispatchEscape(uint16(b))
		} else {
			p.dispatchVT52(uint16(b))
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
		p.identifier.addIntermediate(uint16(b))
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
	add(";53", attr.isOverlined())
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
	p.paramValue = accumulateParameter(p.paramValue, int(b-'0'))
	p.params[len(p.params)-1] = p.paramValue
	p.paramStarted = true
}

const maxParameterValue = 32767

// StateMachine::_AccumulateTo clamps after every appended digit. Once the
// maximum is reached, subsequent digits therefore remain at that value.
func accumulateParameter(value, digit int) int {
	if value > (maxParameterValue-digit)/10 {
		return maxParameterValue
	}
	return value*10 + digit
}

func (p *vtParser) dispatchDCS(final uint16) {
	// OutputStateMachineEngine::ActionDcsDispatch recognizes DECRQSS (`$q`)
	// and returns a string handler. Other identifiers return nullptr and enter
	// DcsIgnore.
	if p.identifier.finalize(final) == vtIDFromString("$q") {
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
	id := p.identifier.finalize(final)
	switch id {
	case vtIDFromString("#8"):
		p.screenAlignmentPattern()
	case vtIDFromString("#3"):
		p.buffer.setCurrentLineRendition(lineRenditionDoubleHeightTop)
	case vtIDFromString("#4"):
		p.buffer.setCurrentLineRendition(lineRenditionDoubleHeightBottom)
	case vtIDFromString("#5"):
		p.buffer.setCurrentLineRendition(lineRenditionSingle)
	case vtIDFromString("#6"):
		p.buffer.setCurrentLineRendition(lineRenditionDoubleWidth)
	case vtIDFromString("D"):
		if err := p.buffer.lineFeed(false); err != nil {
			p.failed = err
		}
	case vtIDFromString("E"):
		p.buffer.carriageReturn()
		if err := p.buffer.lineFeed(true); err != nil {
			p.failed = err
		}
	case vtIDFromString("M"):
		p.reverseLineFeed()
	case vtIDFromString("H"):
		p.buffer.setTab(p.buffer.cursor.position.x)
	case vtIDFromString("N"):
		p.termOutput.singleShift(2)
	case vtIDFromString("O"):
		p.termOutput.singleShift(3)
	case vtIDFromString("="):
		p.keypadMode = true
	case vtIDFromString(">"):
		p.keypadMode = false
	case vtIDFromString("n"):
		p.termOutput.lockingShift(2)
	case vtIDFromString("o"):
		p.termOutput.lockingShift(3)
	case vtIDFromString("~"):
		p.termOutput.lockingShiftRight(1)
	case vtIDFromString("}"):
		p.termOutput.lockingShiftRight(2)
	case vtIDFromString("|"):
		p.termOutput.lockingShiftRight(3)
	case vtIDFromString("Z"):
		p.responses = append(p.responses, []byte("\x1b[?1;0c"))
	case vtIDFromString("\\"):
		// EscActionCodes::ST_StringTerminator is a successful no-op.
	case vtIDFromString("%@"):
		p.termOutput.enableGrTranslation(true)
	case vtIDFromString("%G"):
		p.termOutput.enableGrTranslation(false)
	case vtIDFromString("(0"):
		p.termOutput.designate94Charset(0, "0")
	case vtIDFromString("(B"):
		p.termOutput.designate94Charset(0, "B")
	case vtIDFromString(")0"):
		p.termOutput.designate94Charset(1, "0")
	case vtIDFromString(")B"):
		p.termOutput.designate94Charset(1, "B")
	case vtIDFromString("*0"):
		p.termOutput.designate94Charset(2, "0")
	case vtIDFromString("*B"):
		p.termOutput.designate94Charset(2, "B")
	case vtIDFromString("+0"):
		p.termOutput.designate94Charset(3, "0")
	case vtIDFromString("+B"):
		p.termOutput.designate94Charset(3, "B")
	case vtIDFromString("c"):
		p.hardReset()
	default:
		commandChar := byte(id)
		if id>>8 != 0 {
			switch commandChar {
			case '(', ')', '*', '+':
				p.termOutput.designate94CharsetValue(int(commandChar-'('), id>>8)
			case '-', '.', '/':
				p.termOutput.designate96CharsetValue(int(commandChar-'-'+1), id>>8)
			}
		}
	}
	p.state = stateGround
	p.clearSequence()
	p.clearLastPrinted()
}

// screenAlignmentPattern follows AdaptDispatch::ScreenAlignmentPattern and
// its pinned PrivateFillRegion/ResetLineRenditionRange path. DECALN writes
// default-attribute E cells over the visible viewport, clears only line
// rendition metadata, resets the current meta attributes, clears origin and
// margins, and homes the cursor.
func (p *vtParser) screenAlignmentPattern() {
	// AdaptDispatch::ScreenAlignmentPattern first calls MoveToBottom before
	// taking the viewport dimensions.
	p.buffer.moveToBottom()
	fillLength := (p.buffer.viewportHeight - 1) * p.buffer.width
	if fillLength > 0 {
		units := make([]uint16, fillLength)
		for i := range units {
			units[i] = 'E'
		}
		fillAttr := textAttribute{}
		cells := outputCellsFromUTF16WithAttr(units, fillAttr)
		wrap := false
		_, _ = p.buffer.write(cells, coordinate{y: p.buffer.viewportTop}, &wrap)
	}
	p.buffer.resetLineRenditionRange(p.buffer.viewportTop, p.buffer.viewportTop+p.buffer.viewportHeight)
	attrs := p.buffer.currentAttrs
	attrs.setStandardErase()
	p.buffer.currentAttrs = attrs
	p.buffer.originMode = false
	_ = p.buffer.setScrollingMarginsRaw(0, 0)
	_ = p.buffer.setCursorPosition(coordinate{}, true)
}

func (p *vtParser) dispatchVT52(final uint16) {
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
		p.identifier.addIntermediate(uint16(b))
		p.state = stateCSIParam
		return
	}
	if b >= 0x20 && b <= 0x2f {
		p.intermediate = append(p.intermediate, uint16(b))
		p.identifier.addIntermediate(uint16(b))
		p.state = stateCSIIntermediate
		return
	}
	if b >= 0x40 && b <= 0x7e {
		p.dispatchCSI(uint16(b))
		p.state = stateGround
		p.clearSequence()
		return
	}
	p.state = stateCSIIgnore
}

func (p *vtParser) consumeOSCParam(b byte) {
	if b >= '0' && b <= '9' {
		// StateMachine::_ActionOscParam accumulates through _AccumulateTo,
		// which clamps after each digit.
		p.oscParam = accumulateParameter(p.oscParam, int(b-'0'))
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
	p.oscString = append(p.oscString, uint16(b))
}

func (p *vtParser) dispatchOSC() {
	switch p.oscParam {
	case 0, 1, 2:
		// OutputStateMachineEngine::_GetOscTitle rejects an empty string
		// before calling SetWindowTitle.
		if len(p.oscString) != 0 {
			p.title = string(runesFromUTF16(p.oscString))
		}
	case 4:
		p.oscSetColorTable()
	case 10, 11, 12:
		p.oscSetDefaultColors(p.oscParam)
	case 52:
		p.oscSetClipboard()
	case 112:
		// AdaptDispatch::SetCursorColor returns false immediately for ConPTY,
		// including the INVALID_COLOR reset value.  The pinned dispatcher
		// consequently leaves the buffer cursor color untouched here.
	case 8:
		p.oscHyperlink()
	case 9:
		// AdaptDispatch::DoConEmuAction is the pinned no-op failure path.
	}
	p.state = stateGround
	p.clearSequence()
	p.clearLastPrinted()
}

func (p *vtParser) oscSetColorTable() {
	parts := splitPinnedString(p.oscString, ';')
	if len(parts) < 2 {
		return
	}
	for i, j := 0, 1; j < len(parts); i, j = i+2, j+2 {
		index, indexOK := stringToPinnedUint(parts[i])
		color, colorOK := colorFromXTermColor(string(runesFromUTF16(parts[j])))
		if !indexOK || !colorOK || index >= 256 {
			continue
		}
		// DoSrvPrivateSetColorTableEntry indexes the global Windows table
		// through Xterm256ToWindowsIndex before storing the RGB value.
		p.colorTable[xterm256ToWindowsIndex(int(index))] = color
	}
}

func (p *vtParser) oscSetDefaultColors(command int) {
	parts := splitPinnedString(p.oscString, ';')
	if len(parts) < 1 {
		return
	}
	// OutputStateMachineEngine::_GetOscSetColor retains one vector slot for
	// every field and stores INVALID_COLOR for a failed parse. The dispatch
	// loop advances over those slots but does not call the adapter for the
	// invalid entries.
	colors := make([]uint32, 0, len(parts))
	for _, part := range parts {
		if color, ok := colorFromXTermColor(string(runesFromUTF16(part))); ok {
			colors = append(colors, color)
		} else {
			colors = append(colors, 0xffffffff)
		}
	}
	colorIndex := 0
	if command == 10 && len(colors) > colorIndex {
		if colors[colorIndex] != 0xffffffff {
			p.defaultForeground = colors[colorIndex]
		}
		command++
		colorIndex++
	}
	if command == 11 && len(colors) > colorIndex {
		if colors[colorIndex] != 0xffffffff {
			p.defaultBackground = colors[colorIndex]
		}
		command++
		colorIndex++
	}
	if command == 12 && len(colors) > colorIndex {
		// AdaptDispatch::SetCursorColor is a ConPTY false-returning pass-through
		// boundary, so it must not mutate the text-buffer cursor state.
	}
}

func (p *vtParser) oscSetClipboard() {
	separator := -1
	for i, unit := range p.oscString {
		if unit == ';' {
			separator = i
			break
		}
	}
	if separator < 0 {
		return
	}
	content := p.oscString[separator+1:]
	if len(content) == 1 && content[0] == '?' {
		return
	}
	// AdaptDispatch::SetClipboard is a pinned false-returning no-op. Decode
	// first, exactly as ActionOscDispatch does, but do not invent clipboard
	// state in the standalone buffer.
	_, _ = pinnedBase64Decode(content)
}

func (p *vtParser) oscHyperlink() {
	params, uri, ok := parsePinnedHyperlink(p.oscString)
	if !ok {
		return
	}
	if uri == "" {
		p.buffer.currentAttrs.setHyperlinkID(0)
		return
	}
	id := p.buffer.getHyperlinkID(uri, params)
	p.buffer.currentAttrs.setHyperlinkID(id)
	p.buffer.hyperlinkMap[id] = uri
}

func parsePinnedHyperlink(value []uint16) (params, uri string, ok bool) {
	if len(value) == 1 && value[0] == ';' {
		return "", "", true
	}
	mid := -1
	for i, unit := range value {
		if unit == ';' {
			mid = i
			break
		}
	}
	if mid < 0 {
		return "", "", false
	}
	uri = string(runesFromUTF16(value[mid+1:]))
	for _, part := range splitPinnedString(value[:mid], ':') {
		const hyperlinkIDParameter = "id="
		partText := string(runesFromUTF16(part))
		if index := strings.Index(partText, hyperlinkIDParameter); index >= 0 {
			params = partText[index+len(hyperlinkIDParameter):]
		}
	}
	return params, uri, true
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

func (p *vtParser) dispatchCSI(final uint16) {
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
	// The pinned state machine places CSI private markers and intermediate
	// bytes in the VTID passed to ActionCsiDispatch. Keep the private marker
	// separate as well because AdaptDispatch's mode helper receives it as the
	// DEC-private discriminator.
	identifier := p.identifier.finalize(final)

	// These actions are selected by intermediate bytes in the pinned
	// OutputStateMachineEngine. They must be checked before the final-byte
	// switch: the same final character has different meaning under an
	// intermediate identifier.
	switch identifier {
	case vtIDFromString(" q"):
		p.setCursorStyle(n(0, 1))
		p.clearLastPrinted()
		return
	case vtIDFromString("!p"):
		if p.private == 0 {
			p.softReset()
		}
		p.clearLastPrinted()
		return
	case vtIDFromString("#{"), vtIDFromString("#p"):
		if p.private == 0 {
			p.sgrStack.push(p.buffer.currentAttrs, parameterValues())
		}
		p.clearLastPrinted()
		return
	case vtIDFromString("#}"), vtIDFromString("#q"):
		if p.private == 0 {
			p.buffer.currentAttrs = p.sgrStack.pop(p.buffer.currentAttrs)
		}
		p.clearLastPrinted()
		return
	case vtIDFromString(">c"):
		if raw(0) == 0 {
			p.responses = append(p.responses, []byte("\x1b[>0;10;1c"))
		}
		p.clearLastPrinted()
		return
	case vtIDFromString("=c"):
		if raw(0) == 0 {
			p.responses = append(p.responses, []byte("\x1bP!|00000000\x1b\\"))
		}
		p.clearLastPrinted()
		return
	case vtIDFromString("x"):
		if p.private == 0 {
			switch raw(0) {
			case 0:
				p.responses = append(p.responses, []byte("\x1b[2;1;1;128;128;1;0x"))
			case 1:
				p.responses = append(p.responses, []byte("\x1b[3;1;1;128;128;1;0x"))
			}
		}
		p.clearLastPrinted()
		return
	case vtIDFromString("$|"):
		// AdaptDispatch::SetColumns calls SetConsoleScreenBufferInfoEx, whose
		// width-only resize is TextBuffer::ResizeTraditional in the pinned
		// host path. It must not use the reflow path used by ConPTY resize.
		if p.private == 0 {
			_ = p.buffer.setColumns(n(0, 1))
		}
		p.clearLastPrinted()
		return
	}
	if len(p.intermediate) != 0 || (p.private != 0 && !(p.private == '?' && (final == 'h' || final == 'l'))) {
		p.clearLastPrinted()
		return
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
		p.eraseCharacters(n(0, 1))
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
			p.saveCursorState()
		}
	case 'u':
		if len(p.params) == 0 {
			p.restoreCursorState()
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
	case 'n':
		p.deviceStatusReport(raw(0))
	case 't':
		p.windowManipulation(raw(0), raw(1), raw(2))
	case 'p':
		// DECSTR is CSI ! p; bare CSI p is not an ActionCsiDispatch case.
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
			// AdaptDispatch::SetAnsiMode resets TerminalOutput on every
			// mode update, even when the mode value itself is unchanged.
			p.termOutput = newTerminalOutput()
		case 7:
			p.buffer.wrapAtEOL = enable
		case 3:
			if p.deccolmSupport {
				width := 80
				if enable {
					width = 132
				}
				if err := p.buffer.setColumns(width); err == nil {
					p.buffer.originMode = false
					if err := p.buffer.setCursorPosition(coordinate{}, true); err != nil {
						p.failed = err
						break
					}
					p.eraseDisplay(2)
					if !p.buffer.setScrollingMarginsRaw(0, 0) {
						p.failed = fmt.Errorf("DECCOLM could not reset scrolling margins")
					}
				} else {
					p.failed = err
				}
			}
		case 5:
			// AdaptDispatch::SetScreenMode is a NoOp failure for a ConPTY;
			// the pinned PTY path does not change screen state here.
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
			// AdaptDispatch::EnableXtermBracketedPasteMode is NoOp for the
			// pinned adapter and returns false without changing state.
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
			attr.setIndexedForeground(uint8([]int{0, 4, 2, 6, 1, 5, 3, 7}[option-30]))
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
					attr.setIndexedForeground256(xterm256ToWindowsIndex(index))
				}
				consumed = 2
			}
			i += consumed
		case option == 39:
			attr.setDefaultForeground()
		case option >= 40 && option <= 47:
			attr.setIndexedBackground(uint8([]int{0, 4, 2, 6, 1, 5, 3, 7}[option-40]))
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
					attr.setIndexedBackground256(xterm256ToWindowsIndex(index))
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
			attr.setIndexedForeground(uint8([]int{8, 12, 10, 14, 9, 13, 11, 15}[option-90]))
		case option >= 100 && option <= 107:
			attr.setIndexedBackground(uint8([]int{8, 12, 10, 14, 9, 13, 11, 15}[option-100]))
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
	var cursor cursorType
	var blinking bool
	switch style {
	case 0:
		cursor, blinking = cursorLegacy, true
	case 1:
		cursor, blinking = cursorFullBox, true
	case 2:
		cursor, blinking = cursorFullBox, false
	case 3:
		cursor, blinking = cursorUnderscore, true
	case 4:
		cursor, blinking = cursorUnderscore, false
	case 5:
		cursor, blinking = cursorVerticalBar, true
	case 6:
		cursor, blinking = cursorVerticalBar, false
	default:
		// AdaptDispatch::SetCursorStyle rejects invalid cursor-style values
		// before touching the host cursor.
		return
	}
	p.buffer.cursor.style = cursor
	p.buffer.cursor.blinkingAllowed = blinking
}

func (p *vtParser) deviceStatusReport(status int) {
	if p.private != 0 {
		return
	}
	switch status {
	case 5:
		p.responses = append(p.responses, []byte("\x1b[0n"))
	case 6:
		// AdaptDispatch::_CursorPositionReport moves the virtual viewport
		// before reading the cursor; _OperatingStatus does not.
		p.buffer.moveToBottom()
		position := p.buffer.cursor.position
		row := position.y - p.buffer.viewportTop + 1
		if p.buffer.originMode && p.buffer.marginsSet() {
			row -= p.buffer.scrollTop
		}
		p.responses = append(p.responses, []byte(fmt.Sprintf("\x1b[%d;%dR", row, position.x+1)))
	}
}

// eraseCharacters is AdaptDispatch::EraseCharacters. It fills only the
// remaining cells of the current line and uses standard erase attributes.
func (p *vtParser) eraseCharacters(count int) {
	if count <= 0 {
		return
	}
	position := p.buffer.cursor.position
	// AdaptDispatch::EraseCharacters reads csbiex.dwSize.X, not
	// PrivateGetLineWidth; the erase therefore uses the full backing row even
	// when the current line rendition has a narrower VT line width.
	remaining := p.buffer.width - position.x
	if remaining < 0 {
		remaining = 0
	}
	if count > remaining {
		count = remaining
	}
	if count == 0 {
		return
	}
	p.buffer.clearRangeWithAttr(position.y, position.x, position.x+count-1, p.standardErase())
}

// windowManipulation is AdaptDispatch::WindowManipulation. Function 7 only
// asks the pinned host to repaint; it has no buffer mutation. Function 8 is
// DispatchCommon::s_ResizeWindow with parameter order (width, height).
func (p *vtParser) windowManipulation(function, parameter1, parameter2 int) {
	switch function {
	case 7:
		// DispatchCommon::s_RefreshWindow calls only the host's private
		// repaint operation; it does not mutate the text buffer.
	case 8:
		_ = p.buffer.resizeWindow(parameter2, parameter1)
	}
}

func (p *vtParser) softReset() {
	p.buffer.wrapAtEOL = true
	p.buffer.originMode = false
	p.cursorKeysMode = false
	p.keypadMode = false
	p.buffer.moveToBottom()
	if !p.buffer.setScrollingMarginsRaw(0, 0) {
		p.failed = fmt.Errorf("SoftReset could not reset scrolling margins")
	}
	p.buffer.cursor.visible = true
	p.buffer.cursor.blinkingAllowed = true
	p.termOutput = newTerminalOutput()
	// AdaptDispatch::SoftReset calls SetGraphicsRendition with the default
	// VTParameters value. VTParameters::size() exposes one omitted parameter,
	// whose GraphicsOptions conversion is Off (SGR 0).
	p.setGraphicsRendition([]int{0})
	p.savedCursorState = [2]dispatchCursorState{}
}

// hardReset follows AdaptDispatch::HardReset in the pinned OpenConsole
// source. It deliberately is not parser reset: RIS resets the active
// screen/adapter state in a specific order, preserves process-wide color and
// title state, and in a ConPTY is ultimately reported as unhandled so the
// connected application can receive the RIS sequence.
func (p *vtParser) hardReset() {
	if p.buffer == p.altBuffer {
		p.useMain()
	}

	// The pinned implementation resets SGR before either erase operation so
	// erased cells use the default background color.
	p.softReset()
	p.eraseDisplay(2)
	p.eraseDisplay(3)

	// AdaptDispatch::SetScreenMode is a NoOp failure for a ConPTY, so there is
	// no state change here. Only the two mouse modes explicitly reset by the
	// pinned HardReset are touched.
	if p.mouseModes == nil {
		p.mouseModes = make(map[int]bool)
	}
	p.mouseModes[1006] = false
	p.mouseModes[1003] = false
	p.buffer.resetTabStops()

	// SoftReset makes addressing absolute; CursorPosition(1, 1) therefore
	// homes to the active viewport origin.
	p.buffer.cursorMove(0, 0, true, true, false)

	// PrivateUpdateSoftFont({}, {}, false) and its font-buffer ownership have
	// no text-buffer representation in this probe. This is an explicit source
	// boundary, not a substituted rendering implementation.
}

func (p *vtParser) moveCursor(dy, dx int) {
	p.buffer.cursorMove(dy, dx, false, false, true)
}

func (p *vtParser) eraseDisplay(mode int) {
	if mode == 0 || mode == 1 {
		// AdaptDispatch::EraseInDisplay first restores the virtual viewport
		// with MoveToBottom before reading cursor and window coordinates.
		p.buffer.moveToBottom()
	}
	pos := p.buffer.cursor.position
	eraseSingleLine := func(row, lineMode, cursorColumn int) {
		lineWidth := p.buffer.lineWidth(row)
		left, right := 0, lineWidth-1
		switch lineMode {
		case 0: // AdaptDispatch::EraseType::ToEnd
			left = cursorColumn
		case 1: // AdaptDispatch::EraseType::FromBeginning
			right = cursorColumn
		}
		p.buffer.clearRangeWithAttr(row, left, right, p.standardErase())
	}
	switch mode {
	case 0:
		// The pinned implementation resets line renditions only for complete
		// rows. If the cursor is in column zero, its row is complete too.
		startRow := pos.y + 1
		if pos.x == 0 {
			startRow = pos.y
		}
		p.buffer.resetLineRenditionRange(startRow, p.buffer.viewportBottom()+1)
		eraseSingleLine(pos.y, 0, pos.x)
		for y := pos.y + 1; y <= p.buffer.viewportBottom(); y++ {
			eraseSingleLine(y, 2, 0)
		}
	case 1:
		p.buffer.resetLineRenditionRange(p.buffer.viewportTop, pos.y)
		for y := p.buffer.viewportTop; y < pos.y; y++ {
			eraseSingleLine(y, 2, 0)
		}
		eraseSingleLine(pos.y, 1, pos.x)
	case 2:
		if err := p.buffer.vtEraseAll(); err != nil {
			p.failed = err
		}
	case 3:
		if err := p.buffer.eraseScrollback(); err != nil {
			p.failed = err
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

// shiftCells is the pinned AdaptDispatch::_InsertDeleteHelper path: it cuts
// the remainder of the current line and invokes host/output.cpp::ScrollRegion
// with the source rectangle as its clip rectangle.
func (p *vtParser) shiftCells(row *msRow, start, count int, insert bool) {
	if row == nil || count <= 0 || start < 0 || start >= len(row.charRow.data) {
		return
	}
	end := p.buffer.lineWidth(p.buffer.cursor.position.y)
	if end > len(row.charRow.data) {
		end = len(row.charRow.data)
	}
	if end <= start {
		return
	}
	destination := start - count
	if insert {
		destination = start + count
	}
	source := &cellRect{left: start, top: p.buffer.cursor.position.y, right: end, bottom: p.buffer.cursor.position.y + 1}
	fill := p.standardErase()
	p.buffer.scrollRectangle(source, source, coordinate{x: destination, y: p.buffer.cursor.position.y}, unicodeSpace, fill)
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

func (p *vtParser) saveCursorState() {
	p.buffer.moveToBottom()
	index := 0
	if p.buffer == p.altBuffer {
		index = 1
	}
	position := p.buffer.cursor.position
	position.y -= p.buffer.viewportTop
	p.savedCursorState[index] = dispatchCursorState{
		position: position, originMode: p.buffer.originMode,
		attrs: p.buffer.currentAttrs, termOutput: p.termOutput,
	}
}

func (p *vtParser) restoreCursorState() {
	p.buffer.moveToBottom()
	index := 0
	if p.buffer == p.altBuffer {
		index = 1
	}
	saved := p.savedCursorState[index]
	position := saved.position
	if saved.originMode && p.buffer.scrollBottom != 0 {
		if position.y < p.buffer.scrollTop {
			position.y = p.buffer.scrollTop
		}
		if position.y > p.buffer.scrollBottom {
			position.y = p.buffer.scrollBottom
		}
	}
	p.buffer.originMode = false
	position.y += p.buffer.viewportTop
	if err := p.buffer.setCursorPosition(position, true); err != nil {
		p.failed = err
	}
	p.buffer.originMode = saved.originMode
	p.buffer.currentAttrs = saved.attrs
	p.termOutput = saved.termOutput
}

func (p *vtParser) useAlternate() {
	// AdaptDispatch::UseAlternateScreenBuffer first saves the active cursor,
	// then SCREEN_INFORMATION::_CreateAltBuffer creates an initially erased
	// buffer with the main viewport dimensions and copies only the cursor style,
	// visibility, blinking policy, and viewport-relative position.
	p.saveCursorState()
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
	p.restoreCursorState()
	p.altBuffer = nil
}

func (p *vtParser) printRune(r rune) {
	units := utf16Units(string(r))
	p.buffer.returnOnNewline = p.newLineAutoReturn
	if p.termOutput.needToTranslate() {
		for i, unit := range units {
			units[i] = p.termOutput.translateKey(unit)
		}
	}
	if len(units) == 1 && units[0] == 0x7f {
		return
	}
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
	p.printUnits(units)
}

func (p *vtParser) execute(b byte) {
	switch b {
	case 0x00:
		// OutputStateMachineEngine::ActionExecute explicitly filters NUL.
	case 0x7f:
		// ActionExecute does not special-case DEL. It routes it through Print;
		// the pinned default WriteCharsLegacy path does not store it.
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
		if b == 0x0e {
			p.termOutput.lockingShift(1)
		} else {
			p.termOutput.lockingShift(0)
		}
	case 0x9b:
		p.state = stateCSIEntry
	case 0x9d:
		p.state = stateOSCParam
	default:
		// OutputStateMachineEngine::ActionExecute routes unhandled controls
		// to ITermDispatch::Print. Print enters WriteBuffer::_DefaultCase
		// directly; it does not pass through TerminalOutput::TranslateKey.
		if err := writeDefaultString(p.buffer, []uint16{uint16(b)}); err != nil {
			p.failed = err
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
	p.identifier.clear()
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
	// SCREEN_INFORMATION::ResizeScreenBuffer returns before selecting either
	// the traditional or reflow path when the requested dimensions already
	// match the active buffer.
	if width == p.buffer.width && height == p.buffer.height {
		return nil
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
	if err := reflow(old, newBuffer); err != nil {
		return nil, err
	}
	// SCREEN_INFORMATION::ResizeWithReflow adjusts the viewport by the
	// difference between the new and old cursor heights in that viewport.
	// The old viewport origin cancels algebraically; retaining it here avoids
	// the previous double subtraction of old.viewportTop.
	cursorHeightDiff := newBuffer.cursor.position.y - old.cursor.position.y
	newBuffer.currentAttrs = old.currentAttrs
	newBuffer.cursorSize = old.cursorSize
	newBuffer.viewportHeight = height
	if newBuffer.viewportHeight > newBuffer.height {
		newBuffer.viewportHeight = newBuffer.height
	}
	if newBuffer.viewportHeight < 1 {
		newBuffer.viewportHeight = 1
	}
	newBuffer.viewportTop = old.viewportTop + cursorHeightDiff
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
