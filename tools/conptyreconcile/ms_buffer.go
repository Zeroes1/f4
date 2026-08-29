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

// outputCell is the small input record consumed by ROW::WriteCells.  It is
// deliberately an iterator-like slice record rather than a text string: the
// pinned row code advances one cell at a time and may consume no input when it
// pads a leading/trailing DBCS boundary.
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

// eraseChars is CharRowCell::EraseChars: it removes extended storage and
// returns the visible character to the default space.
func (c *charRowCell) eraseChars(s unicodeStorage, key cellCoordinate) {
	if c.attr.glyphStored {
		c.attr.glyphStored = false
	}
	c.char = unicodeSpace
}

func (c *charRowCell) reset(s unicodeStorage, key cellCoordinate) {
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
		r.data[i].reset(r.store, r.key(i))
	}
}

func (r *charRow) resize(width int) {
	if width < 0 {
		panic("negative CharRow width")
	}
	for key := range r.store {
		if key.y == r.rowID && key.x >= width {
			delete(r.store, key)
		}
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
	r.data[column].reset(r.store, r.key(column))
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

type msRow struct {
	charRow          charRow
	attrs            []textAttribute
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
	return msRow{charRow: newCharRow(width, id, store), attrs: make([]textAttribute, width), lineRendition: lineRenditionSingle, id: id}
}

func (r *msRow) reset(fill textAttribute) {
	r.lineRendition = lineRenditionSingle
	r.wrapForced = false
	r.doubleBytePadded = false
	r.charRow.reset()
	for i := range r.attrs {
		r.attrs[i] = fill
	}
}

func (r *msRow) resize(width int) {
	r.charRow.resize(width)
	if width < len(r.attrs) {
		r.attrs = append([]textAttribute(nil), r.attrs[:width]...)
	} else {
		var fill textAttribute
		if len(r.attrs) != 0 {
			fill = r.attrs[len(r.attrs)-1]
		}
		for len(r.attrs) < width {
			r.attrs = append(r.attrs, fill)
		}
	}
}

func (r *msRow) clearColumn(column int) { r.charRow.clearColumn(column) }

func (r *msRow) replaceAttrs(start, end int, attr textAttribute) {
	if start < 0 {
		start = 0
	}
	if end > len(r.attrs) {
		end = len(r.attrs)
	}
	for i := start; i < end; i++ {
		r.attrs[i] = attr
	}
}

// setAttrToEnd is ATTR_ROW::SetAttrToEnd.  The pinned attr row is a run
// container; setting a column changes the run from that column through the
// row's end, rather than changing one cell only.
func (r *msRow) setAttrToEnd(begin int, attr textAttribute) bool {
	if begin < 0 || begin >= len(r.attrs) {
		return false
	}
	for i := begin; i < len(r.attrs); i++ {
		r.attrs[i] = attr
	}
	return true
}

// writeCells is a direct transcription of ROW::WriteCells' control flow.  A
// returned index is the first input cell not consumed, equivalent to the
// source OutputCellIterator return value.
func (r *msRow) writeCells(input []outputCell, index int, wrap *bool, limitRight *int) (int, int) {
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
	if len(input) == 0 {
		return 0, 0
	}
	currentColor := input[0].attr
	colorUses := 0
	colorStarts := index
	currentIndex := index
	inputIndex := 0
	distance := 0
	for inputIndex < len(input) && currentIndex <= finalColumn {
		cell := input[inputIndex]
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
					r.attrs[currentIndex] = cell.attr
				}
				r.charRow.data[currentIndex].attr = cell.dbcs
				r.charRow.setGlyph(currentIndex, cell.glyph)
				inputIndex++
			}
			if wrap != nil && fillingLastColumn {
				r.wrapForced = *wrap
			}
		} else {
			inputIndex++
		}
		currentIndex++
		distance++
	}
	if colorUses != 0 {
		r.replaceAttrs(colorStarts, currentIndex, currentColor)
	}
	return inputIndex, distance
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
	viewportTop        int
	viewportHeight     int
	virtualBottom      int
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
	return &textBuffer{width: width, height: height, rows: rows, store: store, cursor: cursor, viewportHeight: height, virtualBottom: height - 1, wrapAtEOL: true, processedOutput: true, returnOnNewline: true, currentAttrs: fill, cursorSize: 25, tabStops: tabStops, hyperlinkMap: make(map[uint16]string), hyperlinkCustomID: make(map[string]uint16), patterns: make(map[uint64]string)}
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
	top := origin.y
	if !absolute {
		top = b.viewportTop + origin.y
	}
	if top < 0 || top+b.viewportHeight > b.height {
		return fmt.Errorf("viewport origin %d is outside buffer height %d", top, b.height)
	}
	b.viewportTop = top
	if updateBottom {
		b.virtualBottom = b.viewportBottom()
	}
	return nil
}

func (b *textBuffer) makeCursorVisible(position coordinate, updateBottom bool) {
	delta := 0
	if position.y > b.viewportBottom() {
		delta = position.y - b.viewportBottom()
	} else if position.y < b.viewportTop {
		delta = position.y - b.viewportTop
	}
	if delta != 0 {
		_ = b.setViewportOrigin(false, coordinate{y: delta}, updateBottom)
	}
}

func (b *textBuffer) initializeCursorRowAttributes() {
	row := b.rowByOffset(b.cursor.position.y)
	fill := b.currentAttrs
	fill.setStandardErase()
	row.setAttrToEnd(0, fill)
	row.lineRendition = lineRenditionSingle
}

func (b *textBuffer) setCursor(p coordinate) {
	if p.y < 0 {
		p.y = 0
	}
	if p.y >= b.height {
		p.y = b.height - 1
	}
	if p.x < 0 {
		p.x = 0
	}
	if p.x >= b.width {
		p.x = b.width - 1
	}
	b.cursor.position = p
	b.cursor.delayed = false
	b.cursor.hasMoved = true
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
	if len(glyph) == 0 || !b.sizeInBounds(b.cursor.position) {
		return false
	}
	if !b.prepareForDoubleByteSequence(attr) {
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
func (b *textBuffer) write(input []outputCell, target coordinate, wrap *bool) (int, int) {
	inputIndex := 0
	distance := 0
	lineTarget := target
	for inputIndex < len(input) && b.sizeInBounds(lineTarget) {
		var lineDistance int
		inputIndex, lineDistance = b.writeLine(input, lineTarget, wrap, nil, inputIndex)
		distance += lineDistance
		lineTarget.x = 0
		lineTarget.y++
	}
	return inputIndex, distance
}

func (b *textBuffer) writeLine(input []outputCell, target coordinate, wrap *bool, limitRight *int, inputIndex int) (int, int) {
	if !b.sizeInBounds(target) {
		return inputIndex, 0
	}
	row := b.rowByOffset(target.y)
	consumed, distance := row.writeCells(input[inputIndex:], target.x, wrap, limitRight)
	return inputIndex + consumed, distance
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
	b.refreshRowIDs(&width)
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
		b.rowByOffset(row).attrs[col] = attr
	}
}

func (b *textBuffer) moveCursor(row, col int) {
	b.setCursor(coordinate{x: col, y: row})
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
	b.setCursor(coordinate{x: col, y: row})
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
	if top < 0 || bottom < 0 {
		return false
	}
	if top == 0 {
		top = 1
	}
	if bottom == 0 {
		bottom = b.height
	}
	if top >= bottom || bottom > b.height {
		return false
	}
	if top == 1 && bottom == b.height {
		b.scrollTop = 0
		b.scrollBottom = 0
	} else {
		b.scrollTop = top - 1
		b.scrollBottom = bottom - 1
	}
	b.cursorMove(0, 0, true, true, false)
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
			row.attrs[column] = fill
		}
		b.cursor.position = b.clampPositionWithinLine(b.cursor.position)
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
	b.setCursor(position)
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
	targetTop := top + delta
	targetBottom := bottom + delta
	clippedTop := targetTop
	if clippedTop < top {
		clippedTop = top
	}
	clippedBottom := targetBottom
	if clippedBottom > bottom {
		clippedBottom = bottom
	}
	if clippedTop <= clippedBottom {
		sourceTop := top + clippedTop - targetTop
		b.scrollRows(sourceTop, clippedBottom-clippedTop+1, delta)
	}
	fillTop, fillBottom := top, bottom
	if delta < 0 {
		fillTop = bottom + delta + 1
	} else {
		fillBottom = top + delta - 1
	}
	if fillTop < top {
		fillTop = top
	}
	if fillBottom > bottom {
		fillBottom = bottom
	}
	for row := fillTop; row <= fillBottom; row++ {
		b.rowByOffset(row).reset(fill)
	}
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

func (l logicalRow) text() string {
	return string(runesFromUTF16(l.units))
}

func (b *textBuffer) lastNonSpaceCharacter() coordinate {
	// TextBuffer::GetLastNonSpaceCharacter starts at the bottom of the
	// requested viewport and backs up over empty rows.  Returning the bottom
	// row merely because it exists would change Reflow's cOldRowsTotal for a
	// buffer whose last visible rows are blank.
	last := coordinate{}
	for row := b.height - 1; row >= 0; row-- {
		right := b.rowByOffset(row).charRow.measureRight()
		last.y = row
		last.x = right - 1
		if right >= 1 {
			break
		}
	}
	if last.x < 0 {
		last.x = 0
	}
	if last.y < 0 {
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
	oldCursor := oldBuffer.cursor.position
	oldLast := oldBuffer.lastNonSpaceCharacter()
	oldRowsTotal := oldLast.y + 1
	newCursor := coordinate{}
	foundCursor := false

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
			if !copyCell(newBuffer, oldRow.charRow.glyphAt(oldCol), cell.attr, oldRow.attrs[oldCol]) {
				return fmt.Errorf("reflow insertion failed at old row %d col %d", oldRowIndex, oldCol)
			}
		}
		// This is the pinned GH#32 attribute-row copy.  SetAttrToEnd is
		// intentionally called for every source column, preserving the source
		// run operation and its ordering.
		newRow := newBuffer.rowByOffset(newBuffer.cursor.position.y)
		newAttrColumn := newBuffer.cursor.position.x
		for copyAttrColumn := right; copyAttrColumn < oldWidth && newAttrColumn < newBuffer.lineWidth(newBuffer.cursor.position.y); copyAttrColumn++ {
			if !newRow.setAttrToEnd(newAttrColumn, oldRow.attrs[copyAttrColumn]) {
				break
			}
			newAttrColumn++
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
		newRow.attrs = append(newRow.attrs[:0:0], oldRow.attrs...)
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
