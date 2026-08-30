// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Source-faithful Go transcription of the pinned Microsoft Terminal source
// e9b4e2e18fb1b9cee6839969d42cd0f95d228926.  The names and state transitions
// in this file correspond to CharRowCell, CharRow, ROW, UnicodeStorage, and
// the exercised TextBuffer methods in src/buffer/out.

package main

import (
	"fmt"
	"strings"
)

const unicodeSpace uint16 = 0x20

type dbcsKind uint8

const (
	dbcsSingle dbcsKind = iota
	dbcsLeading
	dbcsTrailing
)

type dbcsAttribute struct {
	kind        dbcsKind
	glyphStored bool
}

type textAttrBehavior uint8

const (
	attrStored textAttrBehavior = iota
	attrCurrent
	attrStoredOnly
)

// outputCell is OutputCellView's observable record. The iterator below keeps
// the source UTF-16 run and exposes this view; it is not a pre-expanded cell
// slice, because OutputCellIterator may expose a leading DBCS view twice.
type outputCell struct {
	glyph    []uint16
	dbcs     dbcsAttribute
	attr     textAttribute
	behavior textAttrBehavior
}

func (a dbcsAttribute) isSingle() bool   { return a.kind == dbcsSingle }
func (a dbcsAttribute) isLeading() bool  { return a.kind == dbcsLeading }
func (a dbcsAttribute) isTrailing() bool { return a.kind == dbcsTrailing }
func (a dbcsAttribute) isDBCS() bool     { return a.kind == dbcsLeading || a.kind == dbcsTrailing }

type cellCoordinate struct {
	x int
	y int
}

// unicodeStorage is the direct map analogue of Microsoft's UnicodeStorage.
// A cell stores only its first UTF-16 unit in the row; a multi-unit glyph is
// kept here and addressed by the row id and column.
type unicodeStorage map[cellCoordinate][]uint16

func (s unicodeStorage) get(key cellCoordinate) []uint16 {
	glyph, ok := s[key]
	if !ok {
		panic("missing UnicodeStorage glyph")
	}
	return glyph
}

func (s unicodeStorage) store(key cellCoordinate, glyph []uint16) {
	s[key] = append([]uint16(nil), glyph...)
}

func (s unicodeStorage) erase(key cellCoordinate) {
	delete(s, key)
}

func (s unicodeStorage) remap(rowMap map[int]int, width *int) {
	newMap := make(unicodeStorage)
	for oldKey, glyph := range s {
		if width != nil && oldKey.x >= *width {
			continue
		}
		newY, ok := rowMap[oldKey.y]
		if !ok {
			continue
		}
		newKey := cellCoordinate{x: oldKey.x, y: newY}
		if _, exists := newMap[newKey]; !exists {
			newMap[newKey] = append([]uint16(nil), glyph...)
		}
	}
	for key := range s {
		delete(s, key)
	}
	for key, glyph := range newMap {
		s[key] = glyph
	}
}

type charRowCell struct {
	char uint16
	attr dbcsAttribute
}

func newCharRowCell() charRowCell {
	return charRowCell{char: unicodeSpace}
}

// eraseChars is CharRowCell::EraseChars. The pinned method only changes the
// cell payload and glyph-stored flag; UnicodeStorage cleanup is deliberately
// not part of this method.
func (c *charRowCell) eraseChars() {
	if c.attr.glyphStored {
		c.attr.glyphStored = false
	}
	c.char = unicodeSpace
}

// reset is CharRowCell::Reset. As in the pinned source, this does not touch
// UnicodeStorage; callers only consult that storage while glyphStored is set.
func (c *charRowCell) reset() {
	c.attr = dbcsAttribute{}
	c.char = unicodeSpace
}

func (c charRowCell) isSpace() bool {
	return !c.attr.glyphStored && c.char == unicodeSpace
}

type charRow struct {
	data  []charRowCell
	rowID int
	store unicodeStorage
}

func newCharRow(width, rowID int, store unicodeStorage) charRow {
	data := make([]charRowCell, width)
	for i := range data {
		data[i] = newCharRowCell()
	}
	return charRow{data: data, rowID: rowID, store: store}
}

func (r *charRow) size() int { return len(r.data) }

func (r *charRow) key(column int) cellCoordinate {
	return cellCoordinate{x: column, y: r.rowID}
}

func (r *charRow) reset() {
	for i := range r.data {
		r.data[i].reset()
	}
}

func (r *charRow) resize(width int) {
	if width < 0 {
		panic("negative CharRow width")
	}
	r.data = append(r.data[:0:0], r.data...)
	if width < len(r.data) {
		r.data = r.data[:width]
	}
	for len(r.data) < width {
		r.data = append(r.data, newCharRowCell())
	}
}

func (r *charRow) measureLeft() int {
	for i, cell := range r.data {
		if !cell.isSpace() {
			return i
		}
	}
	return len(r.data)
}

func (r *charRow) measureRight() int {
	for i := len(r.data) - 1; i >= 0; i-- {
		if !r.data[i].isSpace() {
			return i + 1
		}
	}
	return 0
}

func (r *charRow) containsText() bool {
	return r.measureRight() != 0
}

func (r *charRow) clearCell(column int) {
	if column < 0 || column >= len(r.data) {
		panic("CharRow column out of range")
	}
	r.data[column].reset()
}

func (r *charRow) clearColumn(column int) { r.clearCell(column) }

func (r *charRow) glyphAt(column int) []uint16 {
	if column < 0 || column >= len(r.data) {
		panic("CharRow column out of range")
	}
	cell := r.data[column]
	if cell.attr.glyphStored {
		return r.store.get(r.key(column))
	}
	return []uint16{cell.char}
}

func (r *charRow) setGlyph(column int, glyph []uint16) {
	if column < 0 || column >= len(r.data) || len(glyph) == 0 {
		panic("invalid CharRow glyph write")
	}
	cell := &r.data[column]
	if len(glyph) == 1 {
		cell.char = glyph[0]
		cell.attr.glyphStored = false
		return
	}
	r.store.store(r.key(column), glyph)
	cell.attr.glyphStored = true
}

func (r *charRow) text() []uint16 {
	result := make([]uint16, 0, len(r.data))
	for i := range r.data {
		if !r.data[i].attr.isTrailing() {
			result = append(result, r.glyphAt(i)...)
		}
	}
	return result
}

// attrRun is the Go transcription of til::rle_pair<TextAttribute, uint16_t>.
// The length is stored on the run, exactly as ATTR_ROW's small_rle does; the
// row is not represented as one independent TextAttribute per column.
type attrRun struct {
	length uint16
	value  textAttribute
}

// attrRow is ATTR_ROW's til::small_rle<TextAttribute, uint16_t, 1> surface.
// Keeping the run boundaries matters: Replace and Resize are run operations
// in the pinned source, and GetHyperlinks walks runs rather than cells.
type attrRow struct {
	runs []attrRun
	size uint16
}

func newAttrRow(width int, attr textAttribute) attrRow {
	if width < 0 || width > int(^uint16(0)) {
		panic("invalid ATTR_ROW width")
	}
	row := attrRow{size: uint16(width)}
	if width != 0 {
		row.runs = []attrRun{{length: uint16(width), value: attr}}
	}
	return row
}

func (r *attrRow) reset(attr textAttribute) {
	r.runs = r.runs[:0]
	if r.size != 0 {
		r.runs = append(r.runs, attrRun{length: r.size, value: attr})
	}
}

func (r attrRow) at(column int) textAttribute {
	if column < 0 || column >= int(r.size) {
		panic("ATTR_ROW column out of range")
	}
	for _, run := range r.runs {
		if column < int(run.length) {
			return run.value
		}
		column -= int(run.length)
	}
	panic("ATTR_ROW run length mismatch")
}

func (r *attrRow) set(column int, attr textAttribute) {
	if column < 0 || column >= int(r.size) {
		panic("ATTR_ROW column out of range")
	}
	r.replace(column, column+1, attr)
}

func (r attrRow) expanded() []textAttribute {
	result := make([]textAttribute, 0, r.size)
	for _, run := range r.runs {
		for i := uint16(0); i < run.length; i++ {
			result = append(result, run.value)
		}
	}
	return result
}

func (r attrRow) clone() attrRow {
	return attrRow{runs: append([]attrRun(nil), r.runs...), size: r.size}
}

// resize is ATTR_ROW::Resize, backed by small_rle::resize_trailing_extent.
func (r *attrRow) resize(newSize int) {
	if newSize < 0 || newSize > int(^uint16(0)) {
		panic("invalid ATTR_ROW resize")
	}
	oldSize := int(r.size)
	if newSize == oldSize {
		return
	}
	if newSize < oldSize {
		r.truncate(newSize)
		return
	}
	if newSize == 0 {
		r.size = 0
		r.runs = nil
		return
	}
	if len(r.runs) == 0 {
		panic("ATTR_ROW resize of an empty run vector")
	}
	r.runs[len(r.runs)-1].length += uint16(newSize - oldSize)
	r.size = uint16(newSize)
}

func (r *attrRow) truncate(newSize int) {
	if newSize < 0 || newSize > int(r.size) {
		panic("invalid ATTR_ROW truncate")
	}
	remaining := newSize
	kept := r.runs[:0]
	for _, run := range r.runs {
		if remaining <= 0 {
			break
		}
		length := int(run.length)
		if length > remaining {
			length = remaining
		}
		kept = append(kept, attrRun{length: uint16(length), value: run.value})
		remaining -= length
	}
	r.runs = kept
	r.size = uint16(newSize)
}

// replace is til::small_rle::replace(begin,end,value), with an exclusive end.
// The sequence below follows basic_rle::_replace_unchecked's eight steps;
// retaining run-level mutation is important because expanding to cells would
// be an equivalent implementation rather than the pinned container logic.
func (r *attrRow) replace(begin, end int, attr textAttribute) {
	if begin < 0 || end < begin {
		panic("invalid ATTR_ROW replace range")
	}
	if end > int(r.size) {
		end = int(r.size)
	}
	if begin > end {
		panic("invalid ATTR_ROW replace range")
	}
	if begin == end {
		return
	}

	type scanResult struct {
		run int
		pos int
	}
	scan := func(column int) scanResult {
		position := 0
		for run, value := range r.runs {
			newTotal := position + int(value.length)
			if newTotal > column {
				return scanResult{run: run, pos: column - position}
			}
			position = newTotal
		}
		return scanResult{run: len(r.runs)}
	}

	beginScan := scan(begin)
	endScan := scan(end)
	beginRun, beginPos := beginScan.run, beginScan.pos
	endRun, endPos := endScan.run, endScan.pos
	replacement := attrRun{length: uint16(end - begin), value: attr}

	// [Step1] Detect future adjacent runs.
	beginAdditionalLength := 0
	endAdditionalLength := 0
	if begin != 0 {
		previous := beginRun
		if beginPos == 0 {
			previous--
		}
		if r.runs[previous].value == replacement.value {
			if beginPos != 0 {
				beginAdditionalLength = beginPos
			} else {
				beginAdditionalLength = int(r.runs[previous].length)
			}
			beginPos = 0
			beginRun = previous
		}
	}
	if end != int(r.size) {
		if r.runs[endRun].value == replacement.value {
			endAdditionalLength = int(r.runs[endRun].length) - endPos
			endPos = 0
			endRun++
		}
	}

	// [Step2] Detect a run that must be split to preserve its trailer.
	var midInsertionTrailer *attrRun
	if beginRun == endRun && beginPos != 0 {
		trailer := attrRun{length: r.runs[beginRun].length - uint16(endPos), value: r.runs[beginRun].value}
		midInsertionTrailer = &trailer
		endPos = 0
	}

	// [Step3] Adjust lengths of existing runs around the replaced range.
	if beginPos != 0 {
		r.runs[beginRun].length = uint16(beginPos)
		beginRun++
	}
	if endPos != 0 {
		r.runs[endRun].length -= uint16(endPos)
	}

	// [Step4] Copy as many replacement runs as the existing range can hold.
	availableSpace := 0
	if beginRun < endRun {
		availableSpace = endRun - beginRun
	}
	requiredSpace := 1
	if midInsertionTrailer != nil {
		requiredSpace++
	}
	beginIndex := beginRun
	if availableSpace > 0 {
		r.runs[beginRun] = replacement
		beginRun++
	}

	if availableSpace >= requiredSpace {
		// [Step6.1] Remove any existing runs left in the replaced range.
		r.runs = append(r.runs[:beginRun], r.runs[endRun:]...)
	} else if midInsertionTrailer != nil {
		// [Step6.2] Make room for the replacement and its split-run trailer.
		missing := requiredSpace - availableSpace
		insertAt := beginRun
		newRuns := make([]attrRun, 0, len(r.runs)+missing)
		newRuns = append(newRuns, r.runs[:insertAt]...)
		for i := 0; i < missing; i++ {
			newRuns = append(newRuns, attrRun{})
		}
		newRuns = append(newRuns, r.runs[insertAt:]...)
		r.runs = newRuns
		// [Step4 remainder] Copy replacement runs not copied in place.
		r.runs[beginIndex] = replacement
		// [Step5] Copy the trailer from the run that was split.
		r.runs[beginIndex+1] = *midInsertionTrailer
	} else {
		// [Step6.2] Insert replacement runs not copied in place.
		insertAt := beginRun
		newRuns := make([]attrRun, 0, len(r.runs)+requiredSpace-availableSpace)
		newRuns = append(newRuns, r.runs[:insertAt]...)
		newRuns = append(newRuns, replacement)
		newRuns = append(newRuns, r.runs[insertAt:]...)
		r.runs = newRuns
	}

	// [Step7] Apply the additional lengths for adjacent runs.
	if beginAdditionalLength != 0 {
		r.runs[beginIndex].length += uint16(beginAdditionalLength)
	}
	if endAdditionalLength != 0 {
		endIndex := beginIndex + requiredSpace - 1
		r.runs[endIndex].length += uint16(endAdditionalLength)
	}

	// [Step8] Recalculate the total length.
	r.size = uint16(int(r.size) - (end - begin) + int(replacement.length))
}

func (r *attrRow) setAttrToEnd(begin int, attr textAttribute) bool {
	if begin < 0 || begin > int(r.size) {
		return false
	}
	r.replace(begin, int(r.size), attr)
	return true
}

func (r *attrRow) replaceValues(oldAttr, newAttr textAttribute) {
	for index := range r.runs {
		if r.runs[index].value == oldAttr {
			r.runs[index].value = newAttr
		}
	}
	// replace_values merges adjacent equal runs in the pinned container.
	merged := r.runs[:0]
	for _, run := range r.runs {
		if len(merged) != 0 && merged[len(merged)-1].value == run.value {
			merged[len(merged)-1].length += run.length
		} else {
			merged = append(merged, run)
		}
	}
	r.runs = merged
}

func (r attrRow) hyperlinks() map[uint16]struct{} {
	ids := make(map[uint16]struct{})
	for _, run := range r.runs {
		if run.value.isHyperlink() {
			ids[run.value.hyperlinkID] = struct{}{}
		}
	}
	return ids
}

type msRow struct {
	charRow          charRow
	attrs            attrRow
	lineRendition    lineRendition
	wrapForced       bool
	doubleBytePadded bool
	id               int
}

type lineRendition uint8

const (
	lineRenditionSingle lineRendition = iota
	lineRenditionDoubleWidth
	lineRenditionDoubleHeightTop
	lineRenditionDoubleHeightBottom
)

func newMSRow(width, id int, store unicodeStorage) msRow {
	return msRow{charRow: newCharRow(width, id, store), attrs: newAttrRow(width, textAttribute{}), lineRendition: lineRenditionSingle, id: id}
}

func (r *msRow) reset(fill textAttribute) {
	r.lineRendition = lineRenditionSingle
	r.wrapForced = false
	r.doubleBytePadded = false
	r.charRow.reset()
	r.attrs.reset(fill)
}

func (r *msRow) resize(width int) {
	r.charRow.resize(width)
	r.attrs.resize(width)
}

func (r *msRow) clearColumn(column int) { r.charRow.clearColumn(column) }

func (r *msRow) replaceAttrs(start, end int, attr textAttribute) {
	r.attrs.replace(start, end, attr)
}

// setAttrToEnd is ATTR_ROW::SetAttrToEnd.  The pinned attr row is a run
// container; setting a column changes the run from that column through the
// row's end, rather than changing one cell only.
func (r *msRow) setAttrToEnd(begin int, attr textAttribute) bool {
	return r.attrs.setAttrToEnd(begin, attr)
}

// writeCells is a direct transcription of ROW::WriteCells' control flow. The
// returned iterator is the first source view not consumed.
func (r *msRow) writeCells(input outputCellIterator, index int, wrap *bool, limitRight *int) (outputCellIterator, int) {
	if index < 0 || index >= r.charRow.size() {
		panic("ROW::WriteCells index out of range")
	}
	finalColumn := r.charRow.size() - 1
	if limitRight != nil {
		if *limitRight < 0 || *limitRight >= r.charRow.size() {
			panic("ROW::WriteCells limit out of range")
		}
		finalColumn = *limitRight
	}
	it := input
	currentColor := it.current.attr
	colorUses := 0
	colorStarts := index
	currentIndex := index
	distance := 0
	for it.valid() && currentIndex <= finalColumn {
		cell := it.current
		if cell.behavior != attrCurrent {
			if currentColor == cell.attr {
				colorUses++
			} else {
				r.replaceAttrs(colorStarts, currentIndex, currentColor)
				currentColor = cell.attr
				colorUses = 1
				colorStarts = currentIndex
			}
		}
		fillingLastColumn := currentIndex == finalColumn
		if cell.behavior != attrStoredOnly {
			if currentIndex == 0 && cell.dbcs.isTrailing() {
				r.clearColumn(currentIndex)
			} else if fillingLastColumn && cell.dbcs.isLeading() {
				r.clearColumn(currentIndex)
				r.doubleBytePadded = true
			} else {
				if cell.behavior != attrCurrent {
					r.attrs.set(currentIndex, cell.attr)
				}
				r.charRow.data[currentIndex].attr = cell.dbcs
				r.charRow.setGlyph(currentIndex, cell.glyph)
				it.advance()
			}
			if wrap != nil && fillingLastColumn {
				r.wrapForced = *wrap
			}
		} else {
			it.advance()
		}
		currentIndex++
		distance++
	}
	if colorUses != 0 {
		r.replaceAttrs(colorStarts, currentIndex, currentColor)
	}
	return it, distance
}

type cursorState struct {
	position           coordinate
	hasMoved           bool
	visible            bool
	on                 bool
	double             bool
	blinkingAllowed    bool
	delay              bool
	conversionArea     bool
	popupShown         bool
	delayed            bool
	delayedAt          coordinate
	deferCursorRedraw  bool
	haveDeferredRedraw bool
	size               uint32
	style              cursorType
	useColor           bool
	color              uint32
}

type cursorType uint32

const (
	cursorLegacy cursorType = iota
	cursorVerticalBar
	cursorUnderscore
	cursorEmptyBox
	cursorFullBox
	cursorDoubleUnderscore
)

// copyProperties is Cursor::CopyProperties. Position, delayed EOL state, and
// size are deliberately excluded exactly as in the pinned source.
func (c *cursorState) copyProperties(other cursorState) {
	c.hasMoved = other.hasMoved
	c.visible = other.visible
	c.on = other.on
	c.double = other.double
	c.blinkingAllowed = other.blinkingAllowed
	c.delay = other.delay
	c.conversionArea = other.conversionArea
	c.deferCursorRedraw = other.deferCursorRedraw
	c.haveDeferredRedraw = other.haveDeferredRedraw
	c.style = other.style
	c.color = other.color
}

type textBuffer struct {
	width    int
	height   int
	firstRow int
	circled  bool
	rows     []msRow
	store    unicodeStorage
	cursor   cursorState
	// SCREEN_INFORMATION keeps these coordinates separately from TextBuffer.
	// They are carried here because the standalone port has one screen-info
	// owner for each active buffer and AdjustCursorPosition reads all of them.
	viewportLeft       int
	viewportWidth      int
	viewportTop        int
	viewportHeight     int
	virtualBottom      int
	terminalScrolling  bool
	wrapAtEOL          bool
	processedOutput    bool
	returnOnNewline    bool
	vtMode             bool
	currentAttrs       textAttribute
	cursorSize         uint32
	tabStops           map[int]bool
	savedCursor        coordinate
	savedCursorState   cursorState
	savedCursorAttrs   textAttribute
	savedOriginMode    bool
	originMode         bool
	scrollTop          int
	scrollBottom       int
	hyperlinkMap       map[uint16]string
	hyperlinkCustomID  map[string]uint16
	currentHyperlinkID uint16
	patterns           map[uint64]string
	currentPatternID   uint64
}

func newTextBuffer(width, height int) *textBuffer {
	return newTextBufferWithAttributes(width, height, textAttribute{})
}

// newTextBufferWithAttributes follows TextBuffer's constructor: every row and
// the current output attributes start with the supplied default attributes.
// _CreateAltBuffer passes the standard-erase form of the active attributes.
func newTextBufferWithAttributes(width, height int, fill textAttribute) *textBuffer {
	if width <= 0 || height <= 0 {
		panic("TextBuffer dimensions must be positive")
	}
	store := make(unicodeStorage)
	rows := make([]msRow, height)
	for i := range rows {
		rows[i] = newMSRow(width, i, store)
		rows[i].replaceAttrs(0, width, fill)
	}
	tabStops := make(map[int]bool)
	for i := 8; i < width; i += 8 {
		tabStops[i] = true
	}
	cursor := cursorState{visible: true, on: true, blinkingAllowed: true, size: 25, style: cursorLegacy, color: 0xffffffff}
	return &textBuffer{width: width, height: height, rows: rows, store: store, cursor: cursor, viewportWidth: width, viewportHeight: height, virtualBottom: height - 1, wrapAtEOL: true, processedOutput: true, returnOnNewline: true, currentAttrs: fill, cursorSize: 25, tabStops: tabStops, hyperlinkMap: make(map[uint16]string), hyperlinkCustomID: make(map[string]uint16), currentHyperlinkID: 1, patterns: make(map[uint64]string)}
}

func (b *textBuffer) sizeInBounds(p coordinate) bool {
	// TextBuffer::GetSize is the backing buffer size.  VT line rendition is
	// consulted by the callers that write VT text; it does not change the
	// bounds used by TextBuffer::Write itself.
	return p.x >= 0 && p.y >= 0 && p.x < b.width && p.y < b.height
}

// lineWidth is TextBuffer::GetLineWidth. Double-width and double-height
// renditions use half the backing row for VT coordinates.
func (b *textBuffer) lineWidth(row int) int {
	if row < 0 || row >= b.height {
		return b.width
	}
	if b.rowByOffset(row).lineRendition != lineRenditionSingle {
		return b.width >> 1
	}
	return b.width
}

func (b *textBuffer) rowByOffset(index int) *msRow {
	if index < 0 || index >= b.height {
		panic("TextBuffer row offset out of range")
	}
	return &b.rows[(b.firstRow+index)%b.height]
}

func (b *textBuffer) rowAt(index int) *msRow {
	return b.rowByOffset(index)
}

func (b *textBuffer) cursorPosition() coordinate { return b.cursor.position }

func (b *textBuffer) viewportBottom() int {
	return b.viewportTop + b.viewportHeight - 1
}

func (b *textBuffer) virtualViewportBottom() int { return b.virtualBottom }

func (b *textBuffer) virtualViewportTop() int {
	return b.virtualBottom - b.viewportHeight + 1
}

func (b *textBuffer) setViewportOrigin(absolute bool, origin coordinate, updateBottom bool) error {
	if !absolute && origin.x == 0 && origin.y == 0 {
		// SCREEN_INFORMATION::SetViewportOrigin returns before changing the
		// viewport or calling UpdateBottom for a zero relative delta.
		return nil
	}
	if absolute && origin.x == b.viewportLeft && origin.y == b.viewportTop {
		// The pinned absolute path has the same no-op early return when the
		// requested origin already is the current origin.
		return nil
	}
	left, top := origin.x, origin.y
	if !absolute {
		left = b.viewportLeft + origin.x
		top = b.viewportTop + origin.y
	}
	// SCREEN_INFORMATION::SetViewportOrigin keeps a terminal-scrolling
	// viewport from moving below the logical virtual bottom when the caller is
	// only moving the visible window. The source adjusts both inclusive Y
	// bounds by the same signed delta before validating the final rectangle.
	if b.terminalScrolling && !updateBottom && top+b.viewportHeight-1 > b.virtualBottom {
		top += b.virtualBottom - (top + b.viewportHeight - 1)
	}
	if left < 0 || top < 0 || left+b.viewportWidth > b.width || top+b.viewportHeight > b.height {
		return fmt.Errorf("viewport origin %d,%d is outside buffer %dx%d", left, top, b.width, b.height)
	}
	b.viewportLeft = left
	b.viewportTop = top
	if updateBottom {
		b.virtualBottom = b.viewportBottom()
	}
	return nil
}

func (b *textBuffer) makeCursorVisible(position coordinate, updateBottom bool) {
	delta := coordinate{}
	if position.x > b.viewportRight() {
		delta.x = position.x - b.viewportRight()
	} else if position.x < b.viewportLeft {
		delta.x = position.x - b.viewportLeft
	}
	if position.y > b.viewportBottom() {
		delta.y = position.y - b.viewportBottom()
	} else if position.y < b.viewportTop {
		delta.y = position.y - b.viewportTop
	}
	if delta.x != 0 || delta.y != 0 {
		_ = b.setViewportOrigin(false, delta, updateBottom)
	}
}

func (b *textBuffer) viewportRight() int {
	return b.viewportLeft + b.viewportWidth - 1
}

// moveToBottom follows SCREEN_INFORMATION::MoveToBottom. VT adapter calls
// operate on the virtual viewport rather than a user-scrolled viewport.
func (b *textBuffer) moveToBottom() {
	top := b.virtualViewportTop()
	if top < 0 {
		top = 0
	}
	maxTop := b.height - b.viewportHeight
	if top > maxTop {
		top = maxTop
	}
	if top < 0 {
		top = 0
	}
	_ = b.setViewportOrigin(true, coordinate{x: b.viewportLeft, y: top}, true)
}

func (b *textBuffer) initializeCursorRowAttributes() {
	row := b.rowByOffset(b.cursor.position.y)
	fill := b.currentAttrs
	fill.setStandardErase()
	row.setAttrToEnd(0, fill)
	row.lineRendition = lineRenditionSingle
}

// setCursor is Cursor::SetPosition. The Cursor method itself does not
// validate or clamp coordinates and does not set HasMoved; SCREEN_INFORMATION
// performs those responsibilities in SetCursorPosition.
func (b *textBuffer) setCursor(p coordinate) {
	b.cursor.position = p
	b.cursor.delayed = false
	b.cursor.delayedAt = coordinate{}
}

// setCursorPosition is SCREEN_INFORMATION::SetCursorPosition for the
// standalone screen-info state. The probe has no unfocused-console mode, so
// the pinned focus branch is the only observable branch here.
func (b *textBuffer) setCursorPosition(p coordinate, turnOn bool) error {
	if !b.sizeInBounds(p) {
		return fmt.Errorf("cursor position (%d,%d) is outside buffer", p.x, p.y)
	}
	b.setCursor(p)
	if turnOn {
		b.cursor.delay = false
		b.cursor.on = true
	} else {
		b.cursor.delay = true
	}
	b.cursor.hasMoved = true
	if p.y > b.virtualBottom {
		b.virtualBottom = p.y
	}
	return nil
}

func (b *textBuffer) clampPositionWithinLine(p coordinate) coordinate {
	if p.y < 0 {
		p.y = 0
	}
	if p.y >= b.height {
		p.y = b.height - 1
	}
	if p.x < 0 {
		p.x = 0
	}
	if width := b.lineWidth(p.y); p.x >= width {
		p.x = width - 1
	}
	return p
}

func (b *textBuffer) previousFromCursor() coordinate {
	p := b.cursor.position
	if p.x > 0 {
		p.x--
	} else if p.y > 0 {
		p.y--
		p.x = b.lineWidth(p.y) - 1
	}
	return p
}

// assertValidDoubleByteSequence follows TextBuffer::_AssertValidDoubleByteSequence.
func (b *textBuffer) assertValidDoubleByteSequence(attr dbcsAttribute) bool {
	prevPos := b.previousFromCursor()
	prev := b.rowByOffset(prevPos.y).charRow.data[prevPos.x].attr
	valid := true
	correctableByErase := false
	if prev.isSingle() && attr.isTrailing() {
		valid = false
	} else if prev.isLeading() {
		if attr.isSingle() || attr.isLeading() {
			valid = false
			correctableByErase = true
		}
	} else if prev.isTrailing() && attr.isTrailing() {
		valid = false
	}
	if correctableByErase {
		b.rowByOffset(prevPos.y).clearColumn(prevPos.x)
		valid = true
	}
	return valid
}

// prepareForDoubleByteSequence follows TextBuffer::_PrepareForDoubleByteSequence.
func (b *textBuffer) prepareForDoubleByteSequence(attr dbcsAttribute) bool {
	_ = b.assertValidDoubleByteSequence(attr)
	if attr.isLeading() && b.cursor.position.x == b.lineWidth(b.cursor.position.y)-1 {
		b.rowByOffset(b.cursor.position.y).doubleBytePadded = true
		return b.incrementCursor()
	}
	return true
}

// insertCharacter is TextBuffer::InsertCharacter.  Attributes other than
// DBCS are intentionally not part of the text oracle.
func (b *textBuffer) insertCharacter(glyph []uint16, attr dbcsAttribute) bool {
	return b.insertCharacterWithAttr(glyph, attr, textAttribute{})
}

func (b *textBuffer) insertCharacterWithAttr(glyph []uint16, attr dbcsAttribute, textAttr textAttribute) bool {
	if len(glyph) == 0 {
		return false
	}
	if !b.prepareForDoubleByteSequence(attr) {
		return false
	}
	if !b.sizeInBounds(b.cursor.position) {
		return false
	}
	p := b.cursor.position
	row := b.rowByOffset(p.y)
	row.charRow.setGlyph(p.x, glyph)
	row.charRow.data[p.x].attr = attr
	if !row.setAttrToEnd(p.x, textAttr) {
		return false
	}
	return b.incrementCursor()
}

// write and writeLine preserve TextBuffer::Write/WriteLine's row traversal.
func (b *textBuffer) write(input outputCellIterator, target coordinate, wrap *bool) (outputCellIterator, int) {
	it := input
	distance := 0
	lineTarget := target
	for it.valid() && b.sizeInBounds(lineTarget) {
		var lineDistance int
		it, lineDistance = b.writeLine(it, lineTarget, wrap, nil)
		distance += lineDistance
		lineTarget.x = 0
		lineTarget.y++
	}
	return it, distance
}

func (b *textBuffer) writeLine(input outputCellIterator, target coordinate, wrap *bool, limitRight *int) (outputCellIterator, int) {
	if !b.sizeInBounds(target) {
		return input, 0
	}
	row := b.rowByOffset(target.y)
	return row.writeCells(input, target.x, wrap, limitRight)
}

// incrementCursor follows TextBuffer::IncrementCursor.
func (b *textBuffer) incrementCursor() bool {
	b.cursor.position.x++
	if b.cursor.position.x >= b.lineWidth(b.cursor.position.y) {
		b.rowByOffset(b.cursor.position.y).wrapForced = true
		return b.newlineCursor()
	}
	return true
}

// newlineCursor follows TextBuffer::NewlineCursor.
func (b *textBuffer) newlineCursor() bool {
	b.cursor.position.x = 0
	b.cursor.position.y++
	if b.cursor.position.y >= b.height {
		b.cursor.position.y = b.height - 1
		return b.incrementCircularBuffer()
	}
	return true
}

// incrementCircularBuffer follows TextBuffer::IncrementCircularBuffer.
func (b *textBuffer) incrementCircularBuffer(vtMode ...bool) bool {
	fill := b.currentAttrs
	// TextBuffer::IncrementCircularBuffer's default is false.  The host
	// StreamScrollRegion call supplies the screen's VT-mode bit explicitly;
	// NewlineCursor and ResizeWithReflow use the default call.
	if len(vtMode) != 0 && vtMode[0] {
		fill.setStandardErase()
	}
	// TextBuffer::_PruneHyperlinks runs before the first physical row is
	// reset. It is observable when a wrapped hyperlink is evicted from the
	// circular buffer, even though ordinary text writes do not need it.
	b.pruneHyperlinks()
	b.rowByOffset(0).reset(fill)
	b.firstRow++
	if b.firstRow >= b.height {
		b.firstRow = 0
	}
	b.circled = true
	return true
}

// refreshRowIDs is the row-storage maintenance performed by the pinned
// TextBuffer bulk row movement path before UnicodeStorage::Remap. Rows are
// values in this transcription, so the parent/id refresh that C++ performs
// through ROW references is explicit here.
func (b *textBuffer) refreshRowIDs(newWidth *int) {
	rowMap := make(map[int]int, len(b.rows))
	for index := range b.rows {
		rowMap[b.rows[index].id] = index
	}
	for index := range b.rows {
		if newWidth != nil {
			b.rows[index].resize(*newWidth)
		}
		b.rows[index].id = index
		b.rows[index].charRow.rowID = index
	}
	b.store.remap(rowMap, newWidth)
}

// resizeTraditional is TextBuffer::ResizeTraditional. It intentionally does
// not reflow logical lines; it rotates the circular rows, changes the row
// count, refreshes row IDs, resizes rows, and remaps UnicodeStorage.
func (b *textBuffer) resizeTraditional(width, height int) error {
	if width < 0 || height < 0 {
		return fmt.Errorf("invalid traditional resize %dx%d", width, height)
	}
	if height == 0 {
		return fmt.Errorf("traditional resize requires a non-zero height")
	}
	currentHeight := b.height
	oldWidth := b.width
	oldViewportWidth := b.viewportWidth
	topRow := 0
	if height <= b.cursor.position.y {
		topRow = b.cursor.position.y - height + 1
	}
	topRowIndex := (b.firstRow + topRow) % currentHeight
	if topRowIndex < 0 {
		topRowIndex += currentHeight
	}
	if topRowIndex != 0 {
		rotated := append([]msRow(nil), b.rows[topRowIndex:]...)
		rotated = append(rotated, b.rows[:topRowIndex]...)
		b.rows = rotated
	}
	b.firstRow = 0
	for len(b.rows) > height {
		b.rows = b.rows[:len(b.rows)-1]
	}
	for len(b.rows) < height {
		b.rows = append(b.rows, newMSRow(width, len(b.rows), b.store))
		b.rows[len(b.rows)-1].replaceAttrs(0, width, b.currentAttrs)
	}
	b.width = width
	b.height = height
	if oldViewportWidth == oldWidth {
		b.viewportWidth = width
	} else if b.viewportWidth > width {
		b.viewportWidth = width
	}
	b.refreshRowIDs(&width)
	return nil
}

// setColumns is AdaptDispatch::SetColumns. The pinned operation changes only
// the screen-buffer width through the traditional TextBuffer resize path; it
// does not reflow logical rows.
func (b *textBuffer) setColumns(width int) error {
	if width <= 0 {
		return fmt.Errorf("invalid screen-buffer width %d", width)
	}
	if width == b.width {
		return nil
	}
	return b.resizeTraditional(width, b.height)
}

// resizeWindow is the state carried by DispatchCommon::s_ResizeWindow after
// the host has accepted the new viewport. The requested backing height grows
// when needed, while a smaller window leaves the backing rows intact.
func (b *textBuffer) resizeWindow(width, height int) error {
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid window size %dx%d", width, height)
	}
	backingHeight := b.height
	if height > backingHeight {
		backingHeight = height
	}
	if width != b.width || backingHeight != b.height {
		if err := b.resizeTraditional(width, backingHeight); err != nil {
			return err
		}
	}
	b.viewportWidth = width
	if b.viewportWidth > b.width {
		b.viewportWidth = b.width
	}
	b.viewportHeight = height
	if b.viewportHeight > b.height {
		b.viewportHeight = b.height
	}
	maxTop := b.height - b.viewportHeight
	if b.viewportTop > maxTop {
		b.viewportTop = maxTop
	}
	if b.viewportTop < 0 {
		b.viewportTop = 0
	}
	maxLeft := b.width - b.viewportWidth
	if b.viewportLeft > maxLeft {
		b.viewportLeft = maxLeft
	}
	if b.viewportLeft < 0 {
		b.viewportLeft = 0
	}
	return nil
}

func (b *textBuffer) clearRange(row, left, right int) {
	erase := b.currentAttrs
	erase.setStandardErase()
	b.clearRangeWithAttr(row, left, right, erase)
}

func (b *textBuffer) clearRangeWithAttr(row, left, right int, attr textAttribute) {
	if row < 0 || row >= b.height {
		return
	}
	if left < 0 {
		left = 0
	}
	if right >= b.width {
		right = b.width - 1
	}
	for col := left; col <= right; col++ {
		b.rowByOffset(row).clearColumn(col)
		b.rowByOffset(row).attrs.set(col, attr)
	}
}

// resetLineRenditionRange is TextBuffer::ResetLineRenditionRange. It changes
// only the rendition metadata; unlike ROW::Reset it leaves cell contents and
// attributes untouched.
func (b *textBuffer) resetLineRenditionRange(startRow, endRow int) {
	if startRow < 0 {
		startRow = 0
	}
	if endRow > b.height {
		endRow = b.height
	}
	for row := startRow; row < endRow; row++ {
		b.rowByOffset(row).lineRendition = lineRenditionSingle
	}
}

type cellRect struct {
	left, top, right, bottom int // right and bottom are exclusive
}

func (r cellRect) valid() bool { return r.left < r.right && r.top < r.bottom }

func intersectCellRect(left, right cellRect) cellRect {
	if left.left < right.left {
		left.left = right.left
	}
	if left.top < right.top {
		left.top = right.top
	}
	if left.right > right.right {
		left.right = right.right
	}
	if left.bottom > right.bottom {
		left.bottom = right.bottom
	}
	return left
}

// subtractCellRect is Viewport::Subtract for the two-dimensional regions used
// by output.cpp::ScrollRegion.  The returned rectangles are disjoint and cover
// original minus remove, in the same top/bottom/left/right decomposition used
// by the pinned helper.
func subtractCellRect(original, remove cellRect) []cellRect {
	if !original.valid() {
		return nil
	}
	intersection := intersectCellRect(original, remove)
	if !intersection.valid() {
		return []cellRect{original}
	}
	result := make([]cellRect, 0, 4)
	if original.top < intersection.top {
		result = append(result, cellRect{left: original.left, top: original.top, right: original.right, bottom: intersection.top})
	}
	if intersection.bottom < original.bottom {
		result = append(result, cellRect{left: original.left, top: intersection.bottom, right: original.right, bottom: original.bottom})
	}
	if original.left < intersection.left {
		result = append(result, cellRect{left: original.left, top: intersection.top, right: intersection.left, bottom: intersection.bottom})
	}
	if intersection.right < original.right {
		result = append(result, cellRect{left: intersection.right, top: intersection.top, right: original.right, bottom: intersection.bottom})
	}
	return result
}

// copyRectangle is output.cpp::_CopyRectangle. The full-row branch delegates
// to TextBuffer::ScrollRows; all other rectangles walk source and target in
// Viewport::DetermineWalkDirection order so an overlap never overwrites a
// source cell that has not yet been read.
func (b *textBuffer) copyRectangle(source, target cellRect) {
	if !source.valid() || !target.valid() || source.right-source.left != target.right-target.left || source.bottom-source.top != target.bottom-target.top {
		return
	}
	if source.left == target.left && source.top == target.top {
		return
	}
	if source.left == 0 && target.left == 0 && source.right-source.left == b.width {
		b.scrollRows(source.top, source.bottom-source.top, target.top-source.top)
		return
	}

	xStep := 1
	if target.left >= source.left {
		xStep = -1
	}
	yStep := 1
	if target.top >= source.top {
		yStep = -1
	}
	startX, startY := source.left, source.top
	targetX, targetY := target.left, target.top
	if xStep < 0 {
		startX = source.right - 1
		targetX = target.right - 1
	}
	if yStep < 0 {
		startY = source.bottom - 1
		targetY = target.bottom - 1
	}
	for sourceY, destinationY := startY, targetY; sourceY >= source.top && sourceY < source.bottom; sourceY, destinationY = sourceY+yStep, destinationY+yStep {
		for sourceX, destinationX := startX, targetX; sourceX >= source.left && sourceX < source.right; sourceX, destinationX = sourceX+xStep, destinationX+xStep {
			b.copyCell(coordinate{x: sourceX, y: sourceY}, coordinate{x: destinationX, y: destinationY})
		}
	}
}

// copyCell is the one-cell OutputCellIterator write used by
// output.cpp::_CopyRectangle.  In particular, a clipped leading/trailing
// DBCS cell is passed through ROW::WriteCells' boundary rules instead of being
// copied as an ordinary scalar.
func (b *textBuffer) copyCell(source, target coordinate) {
	if !b.sizeInBounds(source) || !b.sizeInBounds(target) {
		return
	}
	sourceRow := b.rowByOffset(source.y)
	cell := outputCell{
		glyph:    append([]uint16(nil), sourceRow.charRow.glyphAt(source.x)...),
		dbcs:     sourceRow.charRow.data[source.x].attr,
		attr:     sourceRow.attrs.at(source.x),
		behavior: attrStored,
	}
	// The pinned path constructs OutputCell from the source view and then
	// calls TextBuffer::Write with a one-element Cell-mode iterator. Let
	// ROW::WriteCells apply the target boundary rules, including a clipped
	// trailing or leading DBCS cell, instead of duplicating those branches here.
	_, _ = b.write(newOutputCellCellIterator(cell), target, nil)
}

// scrollRectangle is ScrollRegion from host/output.cpp.  The source and
// optional clip rectangles are clipped in the same order as Viewport::Intersect;
// the fill is applied to the source area minus the clipped target.
func (b *textBuffer) scrollRectangle(source, clip *cellRect, targetOrigin coordinate, fillChar uint16, fill textAttribute) {
	buffer := cellRect{left: 0, top: 0, right: b.width, bottom: b.height}
	originalSource := *source
	sourceView := intersectCellRect(originalSource, buffer)
	if !sourceView.valid() {
		return
	}
	clipView := buffer
	if clip != nil {
		clipView = intersectCellRect(*clip, buffer)
	}
	fillView := intersectCellRect(clipView, sourceView)
	if !fillView.valid() {
		return
	}
	currentSourceOrigin := coordinate{x: sourceView.left, y: sourceView.top}
	targetOrigin.x += currentSourceOrigin.x - originalSource.left
	targetOrigin.y += currentSourceOrigin.y - originalSource.top
	targetView := cellRect{left: targetOrigin.x, top: targetOrigin.y, right: targetOrigin.x + sourceView.right - sourceView.left, bottom: targetOrigin.y + sourceView.bottom - sourceView.top}
	originalTargetOrigin := targetView
	targetView = intersectCellRect(targetView, clipView)
	if targetView.valid() {
		sourceOrigin := coordinate{x: sourceView.left + targetView.left - originalTargetOrigin.left, y: sourceView.top + targetView.top - originalTargetOrigin.top}
		sourceView = cellRect{left: sourceOrigin.x, top: sourceOrigin.y, right: sourceOrigin.x + targetView.right - targetView.left, bottom: sourceOrigin.y + targetView.bottom - targetView.top}
		b.copyRectangle(sourceView, targetView)
	}
	if fillChar == 0 && fill == (textAttribute{}) {
		fillChar = unicodeSpace
		fill = b.currentAttrs
	}
	for _, remaining := range subtractCellRect(fillView, targetView) {
		for row := remaining.top; row < remaining.bottom; row++ {
			for column := remaining.left; column < remaining.right; column++ {
				cell := b.rowByOffset(row).charRow.data[column]
				cell.attr = dbcsAttribute{}
				b.rowByOffset(row).charRow.data[column] = cell
				b.rowByOffset(row).charRow.setGlyph(column, []uint16{fillChar})
				b.rowByOffset(row).attrs.set(column, fill)
			}
			if remaining.left == 0 && remaining.right == b.width && targetOrigin.x == 0 {
				b.rowByOffset(row).lineRendition = lineRenditionSingle
			}
		}
	}
}

// vtEraseAll follows SCREEN_INFORMATION::VtEraseAll. It moves the virtual
// viewport below the last non-space character, preserves the cursor's
// viewport-relative position, fills the visible rows with standard-erase
// attributes, and resets their line renditions.
func (b *textBuffer) vtEraseAll() error {
	last := b.lastNonSpaceCharacter()
	newTop := last.y + 1
	oldViewportTop := b.viewportTop
	relativeCursor := b.cursor.position
	relativeCursor.y -= oldViewportTop
	delta := newTop + b.viewportHeight - b.height
	for i := 0; i < delta; i++ {
		if !b.incrementCircularBuffer() {
			return fmt.Errorf("circular buffer increment failed")
		}
		newTop--
	}
	if err := b.setViewportOrigin(true, coordinate{y: newTop}, true); err != nil {
		return err
	}
	relativeCursor.y += b.viewportTop
	if err := b.setCursorPosition(relativeCursor, false); err != nil {
		return err
	}
	fill := b.currentAttrs
	fill.setStandardErase()
	units := make([]uint16, b.viewportHeight*b.width)
	for i := range units {
		units[i] = unicodeSpace
	}
	wrap := false
	_, _ = b.write(outputCellsFromUTF16WithAttr(units, fill), coordinate{y: b.viewportTop}, &wrap)
	b.resetLineRenditionRange(b.viewportTop, b.viewportTop+b.viewportHeight)
	return nil
}

// eraseScrollback follows AdaptDispatch::_EraseScrollback for the backing
// rows represented by this standalone screen buffer. It moves the old
// viewport to row zero, clears the rows below it with default attributes, and
// preserves the cursor's viewport-relative location.
func (b *textBuffer) eraseScrollback() error {
	oldTop := b.viewportTop
	height := b.viewportHeight
	oldCursor := b.cursor.position
	// AdaptDispatch::_EraseScrollback reads the old viewport, then calls
	// SCREEN_INFORMATION::MoveToBottom before scrolling that saved rectangle.
	b.moveToBottom()
	// The source rectangle has an intentionally oversized bottom
	// (SHORT_MAX), so after clipping it covers every backing row from the old
	// viewport top through the bottom, not merely one viewport height. The
	// target is unclipped; this is the distinction between the scrollback path
	// and an ordinary viewport-local scroll.
	source := &cellRect{left: 0, top: oldTop, right: b.width, bottom: b.height}
	b.scrollRectangle(source, nil, coordinate{}, unicodeSpace, textAttribute{})
	for row := height; row < b.height; row++ {
		b.rowByOffset(row).reset(textAttribute{})
	}
	b.resetLineRenditionRange(height, b.height)
	if err := b.setViewportOrigin(true, coordinate{}, true); err != nil {
		return err
	}
	newCursor := coordinate{x: oldCursor.x, y: oldCursor.y - oldTop}
	return b.setCursorPosition(newCursor, true)
}

func (b *textBuffer) moveCursor(row, col int) {
	_ = b.setCursorPosition(coordinate{x: col, y: row}, true)
}

// cursorMove is the buffer-side transcription of
// AdaptDispatch::_CursorMovePosition.
func (b *textBuffer) cursorMove(rowOffset, colOffset int, rowAbsolute, colAbsolute, clampInMargins bool) {
	current := b.cursor.position
	row := current.y
	col := current.x
	if rowAbsolute {
		row = b.viewportTop
		if b.originMode && b.marginsSet() {
			top, _ := b.absoluteScrollMargins()
			row = top
		}
	}
	if colAbsolute {
		col = 0
	}
	row += rowOffset
	col += colOffset
	if row < b.viewportTop {
		row = b.viewportTop
	}
	if row > b.viewportBottom() {
		row = b.viewportBottom()
	}
	if col < 0 {
		col = 0
	}
	if col >= b.width {
		col = b.width - 1
	}
	if b.marginsSet() && (clampInMargins || b.originMode) {
		top, bottom := b.absoluteScrollMargins()
		if current.y >= top && row < top {
			row = top
		}
		if current.y <= bottom && row > bottom {
			row = bottom
		}
	}
	_ = b.setCursorPosition(coordinate{x: col, y: row}, true)
}

func (b *textBuffer) marginsSet() bool { return b.scrollTop < b.scrollBottom }

func (b *textBuffer) absoluteScrollMargins() (top, bottom int) {
	if !b.marginsSet() {
		return b.viewportTop, b.viewportTop
	}
	return b.viewportTop + b.scrollTop, b.viewportTop + b.scrollBottom
}

// setScrollingMargins follows AdaptDispatch::_DoSetTopBottomScrollingMargins.
// Parameters are VT's one-based inclusive line numbers.
func (b *textBuffer) setScrollingMargins(top, bottom int) bool {
	if !b.setScrollingMarginsRaw(top, bottom) {
		return false
	}
	b.cursorMove(0, 0, true, true, false)
	return true
}

// setScrollingMarginsRaw is AdaptDispatch::_DoSetTopBottomScrollingMargins.
// The public adapter entry point homes the cursor after this helper returns;
// DECCOLM and DECALN use the helper directly, as in the pinned source.
func (b *textBuffer) setScrollingMarginsRaw(top, bottom int) bool {
	if top < 0 || bottom < 0 {
		return false
	}
	if top == 0 {
		top = 1
	}
	if bottom == 0 {
		// AdaptDispatch measures the default bottom from the exclusive
		// viewport height (srWindow.Bottom - srWindow.Top), not the backing
		// TextBuffer height.
		bottom = b.viewportHeight
	}
	if top >= bottom || bottom > b.viewportHeight {
		return false
	}
	if top == 1 && bottom == b.viewportHeight {
		b.scrollTop = 0
		b.scrollBottom = 0
	} else {
		b.scrollTop = top - 1
		b.scrollBottom = bottom - 1
	}
	return true
}

// setCurrentLineRendition follows TextBuffer::SetCurrentLineRendition. The
// right half is cleared when entering a non-single rendition and the cursor
// is clamped to the new logical width.
func (b *textBuffer) setCurrentLineRendition(rendition lineRendition) {
	rowIndex := b.cursor.position.y
	row := b.rowByOffset(rowIndex)
	if row.lineRendition == rendition {
		return
	}
	row.lineRendition = rendition
	row.wrapForced = false
	if rendition != lineRenditionSingle {
		fill := b.currentAttrs
		fill.setStandardErase()
		fillOffset := b.lineWidth(rowIndex)
		for column := fillOffset; column < b.width; column++ {
			row.charRow.clearCell(column)
			row.attrs.set(column, fill)
		}
		b.setCursor(b.clampPositionWithinLine(b.cursor.position))
	}
}

func (b *textBuffer) saveCursor() {
	b.savedCursorState = b.cursor
	b.savedCursorState.position.y -= b.viewportTop
	b.savedCursorAttrs = b.currentAttrs
	b.savedOriginMode = b.originMode
}

func (b *textBuffer) restoreCursor() {
	position := b.savedCursorState.position
	if b.savedOriginMode && b.marginsSet() {
		if position.y < b.scrollTop {
			position.y = b.scrollTop
		}
		if position.y > b.scrollBottom {
			position.y = b.scrollBottom
		}
	}
	position.y += b.viewportTop
	b.originMode = false
	_ = b.setCursorPosition(position, true)
	b.originMode = b.savedOriginMode
	b.currentAttrs = b.savedCursorAttrs
}

// copyHyperlinkMaps and copyPatterns are the two non-row copy operations at
// the end of TextBuffer::Reflow. They are kept even though the probe's
// current VT corpus does not create hyperlink or pattern records.
func (b *textBuffer) copyHyperlinkMaps(other *textBuffer) {
	b.hyperlinkMap = cloneUint16StringMap(other.hyperlinkMap)
	b.hyperlinkCustomID = cloneStringUint16Map(other.hyperlinkCustomID)
	b.currentHyperlinkID = other.currentHyperlinkID
}

func (b *textBuffer) copyPatterns(other *textBuffer) {
	b.patterns = cloneUint64StringMap(other.patterns)
	b.currentPatternID = other.currentPatternID
}

// hyperlinksInRow is ATTR_ROW::GetHyperlinks. ATTR_ROW returns hyperlink IDs
// from its runs; a set gives the same ID collection for the expanded Go row.
func (r *msRow) hyperlinksInRow() map[uint16]struct{} {
	return r.attrs.hyperlinks()
}

// removeHyperlinkFromMap follows TextBuffer::RemoveHyperlinkFromMap: remove
// the URI entry and the first custom-id entry referring to the same numeric
// ID. The custom-id key remains the isolated reconstruction documented above.
func (b *textBuffer) removeHyperlinkFromMap(id uint16) {
	delete(b.hyperlinkMap, id)
	for key, value := range b.hyperlinkCustomID {
		if value == id {
			delete(b.hyperlinkCustomID, key)
			break
		}
	}
}

// pruneHyperlinks is TextBuffer::_PruneHyperlinks. Only IDs present in the
// physical first row are candidates; each is retained if found in any later
// logical row and otherwise removed from the two maps.
func (b *textBuffer) pruneHyperlinks() {
	firstRowRefs := b.rowByOffset(0).hyperlinksInRow()
	if len(firstRowRefs) == 0 {
		return
	}
	for row := 1; row < b.height && len(firstRowRefs) != 0; row++ {
		for id := range b.rowByOffset(row).hyperlinksInRow() {
			delete(firstRowRefs, id)
		}
	}
	for id := range firstRowRefs {
		b.removeHyperlinkFromMap(id)
	}
}

// getHyperlinkID follows TextBuffer::GetHyperlinkId for the observable
// allocation, reuse, and zero-avoidance rules. The pinned source appends the
// result of std::hash<std::wstring_view> to the custom-id key; that library
// implementation is outside the pinned OpenConsole tree, so the isolated
// key below preserves only the documented equality relation and is recorded
// as a reconstruction in the audit ledger.
func (b *textBuffer) getHyperlinkID(uri, customID string) uint16 {
	var numericID uint16
	if customID == "" {
		numericID = b.currentHyperlinkID
		b.currentHyperlinkID++
	} else {
		key := customID + "\x00" + uri
		if existing, ok := b.hyperlinkCustomID[key]; ok {
			numericID = existing
		} else {
			b.hyperlinkCustomID[key] = b.currentHyperlinkID
			numericID = b.currentHyperlinkID
			b.currentHyperlinkID++
		}
	}
	if b.currentHyperlinkID == 0 {
		b.currentHyperlinkID++
	}
	return numericID
}

func cloneUint16StringMap(source map[uint16]string) map[uint16]string {
	result := make(map[uint16]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneStringUint16Map(source map[string]uint16) map[string]uint16 {
	result := make(map[string]uint16, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneUint64StringMap(source map[uint64]string) map[uint64]string {
	result := make(map[uint64]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (b *textBuffer) setTab(column int) { b.tabStops[column] = true }

func (b *textBuffer) clearTab(column int) { delete(b.tabStops, column) }

// resetTabStops follows AdaptDispatch::_ResetTabStops. The pinned adapter
// clears its lazily-sized table and marks default stops for reinitialization;
// this standalone buffer keeps the same observable result by materializing
// those defaults for its current width.
func (b *textBuffer) resetTabStops() {
	b.tabStops = make(map[int]bool)
	for column := 8; column < b.width; column += 8 {
		b.tabStops[column] = true
	}
}

func (b *textBuffer) tab() {
	b.tabForward(1)
}

func (b *textBuffer) tabForward(count int) {
	if count < 0 {
		return
	}
	start := b.cursor.position.x
	for step := 0; step < count; step++ {
		next := b.width - 1
		for i := start + 1; i < b.width; i++ {
			if b.tabStops[i] {
				next = i
				break
			}
		}
		start = next
	}
	// AdaptDispatch::ForwardTab moves the cursor; it does not write the
	// intervening cells.  Legacy WriteCharsLegacy expands a tab to spaces in
	// its own path, which is why that behavior is not shared here.
	b.cursor.position.x = start
}

func (b *textBuffer) tabBackward(count int) {
	if count < 0 {
		return
	}
	start := b.cursor.position.x
	for step := 0; step < count; step++ {
		previous := 0
		for i := start - 1; i >= 0; i-- {
			if b.tabStops[i] {
				previous = i
				break
			}
		}
		start = previous
	}
	b.cursor.position.x = start
}

func (b *textBuffer) clearTabs(mode int) {
	switch mode {
	case 0:
		b.clearTab(b.cursor.position.x)
	case 3:
		clear(b.tabStops)
	}
}

func (b *textBuffer) carriageReturn() { b.cursor.position.x = 0 }

func (b *textBuffer) lineFeed(withReturn bool) error {
	// DoSrvPrivateLineFeed first turns the cursor on, clears the wrap bit on
	// the row being left, advances Y, optionally returns X to zero, and then
	// delegates the boundary/margin handling to AdjustCursorPosition.
	row := b.cursor.position.y
	b.rowByOffset(row).wrapForced = false
	position := b.cursor.position
	position.y++
	if withReturn {
		position.x = 0
	} else {
		position = b.clampPositionWithinLine(position)
	}
	if err := adjustCursorPosition(b, position, false); err != nil {
		return err
	}
	return nil
}

func (b *textBuffer) backspace() {
	if b.cursor.position.x > 0 {
		b.cursor.position.x--
	}
}

func (b *textBuffer) scrollRegionUp(count int) {
	top, bottom := b.viewportTop, b.viewportBottom()
	if b.marginsSet() {
		top, bottom = b.absoluteScrollMargins()
	}
	b.scrollRegion(top, bottom, count, false)
}

func (b *textBuffer) scrollRegionDown(count int) {
	top, bottom := b.viewportTop, b.viewportBottom()
	if b.marginsSet() {
		top, bottom = b.absoluteScrollMargins()
	}
	b.scrollRegion(top, bottom, count, true)
}

// scrollRows is TextBuffer::ScrollRows(firstRow, size, delta). It only
// rearranges storage; the caller that implements ScrollRegion performs the
// revealed-row fill separately, just as the pinned host does.
func (b *textBuffer) scrollRows(first, size, delta int) {
	if first < 0 || size <= 0 || first+size > b.height || delta == 0 {
		return
	}
	// TextBuffer::ScrollRows first makes the circular storage linear, then
	// applies the same std::rotate calls to the requested subsection.
	if b.firstRow != 0 {
		rotated := append([]msRow(nil), b.rows[b.firstRow:]...)
		rotated = append(rotated, b.rows[:b.firstRow]...)
		b.rows = rotated
		b.firstRow = 0
	}
	if delta < 0 {
		rotateRows(b.rows, first+delta, first, first+size)
	} else {
		rotateRows(b.rows, first, first+size, first+size+delta)
	}
	b.refreshRowIDs(nil)
}

// scrollRegion is the pinned DoSrvPrivateScrollRegion/ScrollRegion shape for
// a full-width vertical move. The target is clipped to the source rectangle,
// the source is adjusted by the same displacement, then TextBuffer::ScrollRows
// performs the copy and the uncovered rows are filled with erase attributes.
func (b *textBuffer) scrollRegion(top, bottom, count int, down bool) {
	fill := b.currentAttrs
	if b.vtMode {
		fill.setStandardErase()
	}
	b.scrollRegionWithAttr(top, bottom, count, down, fill)
}

func (b *textBuffer) scrollRegionWithAttr(top, bottom, count int, down bool, fill textAttribute) {
	if top < 0 || bottom >= b.height || top > bottom || count <= 0 {
		return
	}
	delta := -count
	if down {
		delta = count
	}
	source := &cellRect{left: 0, top: top, right: b.width, bottom: bottom + 1}
	clip := *source
	b.scrollRectangle(source, &clip, coordinate{y: top + delta}, unicodeSpace, fill)
}

// rotateRows is std::rotate(first, middle, last) for a Go slice.  The caller
// supplies absolute indices into rows and is responsible for the same valid
// ranges guaranteed by the pinned caller.
func rotateRows(rows []msRow, first, middle, last int) {
	if first < 0 || middle < first || last < middle || last > len(rows) || first == middle || middle == last {
		return
	}
	tmp := append([]msRow(nil), rows[first:middle]...)
	copy(rows[first:last-(middle-first)], rows[middle:last])
	copy(rows[last-(middle-first):last], tmp)
}

func (b *textBuffer) logicalRows() []logicalRow {
	result := make([]logicalRow, 0, b.height)
	for i := 0; i < b.height; i++ {
		row := b.rowByOffset(i)
		if i == 0 || len(result) == 0 || !result[len(result)-1].continues {
			result = append(result, logicalRow{sourceStart: i})
		}
		line := &result[len(result)-1]
		line.rows = append(line.rows, i)
		line.continues = row.wrapForced
		limit := row.charRow.measureRight()
		if row.wrapForced {
			limit = b.lineWidth(i)
			if row.doubleBytePadded && limit > 0 {
				limit--
			}
		}
		for col := 0; col < limit; col++ {
			cell := row.charRow.data[col]
			if cell.attr.isTrailing() {
				continue
			}
			line.units = append(line.units, row.charRow.glyphAt(col)...)
		}
	}
	return result
}

type logicalRow struct {
	units       []uint16
	rows        []int
	sourceStart int
	continues   bool
}

// reflowPositionInformation is TextBuffer::PositionInformation. The
// standalone resize path currently passes nil, as pinned
// SCREEN_INFORMATION::ResizeWithReflow does; the optional branch remains
// represented for the TerminalCore caller in the pinned source.
type reflowPositionInformation struct {
	mutableViewportTop int
	visibleViewportTop int
}

func (l logicalRow) text() string {
	return string(runesFromUTF16(l.units))
}

func (b *textBuffer) lastNonSpaceCharacter() coordinate {
	return b.lastNonSpaceCharacterIn(nil)
}

func (b *textBuffer) lastNonSpaceCharacterIn(view *cellRect) coordinate {
	// TextBuffer::GetLastNonSpaceCharacter starts at the bottom of the
	// requested viewport and backs up over empty rows. Returning the bottom row
	// merely because it exists would change Reflow's cOldRowsTotal for a buffer
	// whose last visible rows are blank.
	top, bottom := 0, b.height
	if view != nil {
		top, bottom = view.top, view.bottom
		if top < 0 {
			top = 0
		}
		if bottom > b.height {
			bottom = b.height
		}
	}
	last := coordinate{y: bottom - 1}
	for row := bottom - 1; row >= top; row-- {
		right := b.rowByOffset(row).charRow.measureRight()
		last.y = row
		last.x = right - 1
		if last.x >= 0 || row == top {
			break
		}
	}
	if last.x < 0 {
		last.x = 0
	}
	if last.y < top {
		last.y = 0
	}
	return last
}

func (b *textBuffer) text() string {
	var out strings.Builder
	for _, row := range b.logicalRows() {
		out.WriteString(row.text())
		if !row.continues {
			out.WriteByte('\n')
		}
	}
	return strings.TrimSuffix(out.String(), "\n")
}

func copyCell(b *textBuffer, glyph []uint16, attr dbcsAttribute, textAttr textAttribute) bool {
	return b.insertCharacterWithAttr(glyph, attr, textAttr)
}

// reflow is the exercised TextBuffer::Reflow path.  It preserves the pinned
// row flags and copies cells in row order; no text comparison participates in
// the operation.
func reflow(oldBuffer, newBuffer *textBuffer) error {
	return reflowWithOptions(oldBuffer, newBuffer, nil, nil)
}

func reflowWithOptions(oldBuffer, newBuffer *textBuffer, lastCharacterViewport *cellRect, positionInfo *reflowPositionInformation) error {
	oldCursor := oldBuffer.cursor.position
	oldLast := oldBuffer.lastNonSpaceCharacterIn(lastCharacterViewport)
	oldRowsTotal := oldLast.y + 1
	newCursor := coordinate{}
	foundCursor := false
	foundOldMutable := false
	foundOldVisible := false

	for oldRowIndex := 0; oldRowIndex < oldRowsTotal; oldRowIndex++ {
		oldRow := oldBuffer.rowByOffset(oldRowIndex)
		oldWidth := oldBuffer.lineWidth(oldRowIndex)
		right := oldRow.charRow.measureRight()
		if oldRow.wrapForced {
			right = oldWidth
			if oldRow.doubleBytePadded && right > 0 {
				right--
			}
		}
		if newBuffer.cursor.position.x == 0 {
			newBuffer.rowByOffset(newBuffer.cursor.position.y).lineRendition = oldRow.lineRendition
		}
		for oldCol := 0; oldCol < right; oldCol++ {
			if oldCol == oldCursor.x && oldRowIndex == oldCursor.y && !foundCursor {
				newCursor = newBuffer.cursor.position
				foundCursor = true
			}
			cell := oldRow.charRow.data[oldCol]
			if !copyCell(newBuffer, oldRow.charRow.glyphAt(oldCol), cell.attr, oldRow.attrs.at(oldCol)) {
				return fmt.Errorf("reflow insertion failed at old row %d col %d", oldRowIndex, oldCol)
			}
		}
		// This is the pinned GH#32 attribute-row copy.  SetAttrToEnd is
		// intentionally called for every source column, preserving the source
		// run operation and its ordering.
		newRow := newBuffer.rowByOffset(newBuffer.cursor.position.y)
		newAttrColumn := newBuffer.cursor.position.x
		for copyAttrColumn := right; copyAttrColumn < oldWidth && newAttrColumn < newBuffer.lineWidth(newBuffer.cursor.position.y); copyAttrColumn++ {
			if !newRow.setAttrToEnd(newAttrColumn, oldRow.attrs.at(copyAttrColumn)) {
				break
			}
			newAttrColumn++
		}
		if positionInfo != nil {
			if !foundOldMutable && oldRowIndex >= positionInfo.mutableViewportTop {
				positionInfo.mutableViewportTop = newBuffer.cursor.position.y
				foundOldMutable = true
			}
			if !foundOldVisible && oldRowIndex >= positionInfo.visibleViewportTop {
				positionInfo.visibleViewportTop = newBuffer.cursor.position.y
				foundOldVisible = true
			}
		}
		if right < oldWidth && !oldRow.wrapForced && oldRowIndex < oldRowsTotal-1 {
			if !foundCursor && right == oldCursor.x && oldRowIndex == oldCursor.y {
				newCursor = newBuffer.cursor.position
				foundCursor = true
			}
			newBuffer.newlineCursor()
		} else if right < oldWidth && !oldRow.wrapForced && oldRowIndex == oldRowsTotal-1 {
			// TextBuffer::Reflow preserves a final hard newline if the
			// narrower buffer made the preceding row soft-wrap and thereby
			// already moved the cursor to column zero.
			newPosition := newBuffer.cursor.position
			if newPosition.x == 0 && newPosition.y > 0 && newBuffer.rowByOffset(newPosition.y-1).wrapForced {
				newBuffer.newlineCursor()
			}
		}
	}

	// TextBuffer::Reflow also copies the attribute rows below the last
	// printable row.  Those rows are not text, but their attributes remain
	// observable after a resize (the color-only buffer case).
	newRowY := newBuffer.cursor.position.y + 1
	for oldRowIndex := oldRowsTotal; oldRowIndex < oldBuffer.height && newRowY < newBuffer.height; oldRowIndex++ {
		oldRow := oldBuffer.rowByOffset(oldRowIndex)
		newRow := newBuffer.rowByOffset(newRowY)
		newRow.attrs = oldRow.attrs.clone()
		newRow.resize(newBuffer.lineWidth(newRowY))
		newRowY++
	}

	// TextBuffer::CopyProperties runs before the final position restoration.
	// Cursor::CopyProperties deliberately excludes position, size, and delayed
	// EOL state, so this ordering is observable for the remaining properties.
	oldCursorSize := oldBuffer.cursor.size
	newBuffer.cursor.copyProperties(oldBuffer.cursor)
	newBuffer.copyHyperlinkMaps(oldBuffer)
	newBuffer.copyPatterns(oldBuffer)
	if foundCursor {
		newBuffer.setCursor(newCursor)
	} else {
		// This is TextBuffer::Reflow's post-copy cursor advancement when the
		// old cursor was not encountered while copying printable cells.
		newlines := oldCursor.y - oldLast.y
		increments := oldCursor.x - oldLast.x
		newLast := newBuffer.lastNonSpaceCharacter()
		if newBuffer.rowByOffset(newLast.y).wrapForced {
			if newlines > 0 {
				newlines--
			}
		} else if oldBuffer.rowByOffset(oldLast.y).wrapForced {
			if newlines > 0 {
				newlines--
			}
		}
		for i := 0; i < newlines; i++ {
			newBuffer.newlineCursor()
		}
		for col := 0; col < increments-1; col++ {
			newBuffer.incrementCursor()
		}
	}
	// ResizeWithReflow restores the old cursor size after TextBuffer::Reflow.
	newBuffer.cursor.size = oldCursorSize
	return nil
}
