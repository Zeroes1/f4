// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// 1:1 port of microsoft/terminal src/buffer/out/Row.cpp + Row.hpp
// (commit 079d1cc423336c89c1e220701c94b320cecb603a), restricted to the
// text/wrap model: _chars, _charOffsets, WriteHelper, ReplaceText,
// _replaceTextUnicode, ReplaceCharacters, CopyTextFrom, Finish,
// _resizeChars, Reset/_init, GetText, MeasureRight, GetLastNonSpaceColumn,
// the clamp/adjust helpers, and the wrap flags.
//
// Mechanical transformations required by Go, and nothing else:
//   - wchar_t / uint16_t  -> uint16; wstring_view -> []uint16 + indices
//   - the SIMD variants of _init -> the scalar fallback that the same
//     function carries under `#else` (std::fill_n + std::iota)
//   - iterators           -> integer indices with identical arithmetic
//
// Recorded non-ports (docs/CONPTY_RESEARCH.md, "THE RULE" ledger):
//   - TextAttribute / til::small_rle color runs: represented by a zero-size
//     TextAttribute and no-op replace calls, kept at the original call
//     sites. Colors do not participate in the text/wrap model.
//   - LineRendition is carried but only SingleWidth is ever set, because
//     the DECDWL/DECDHL dispatch is not ported (listed in msengine.go).
//   - ImageSlice, hyperlinks, ScrollbarData/prompt marks: absent.

package main

const (
	// CharOffsetsTrailer, CharOffsetsMask (Row.hpp)
	CharOffsetsTrailer uint16 = 0x8000
	CharOffsetsMask    uint16 = 0x7fff
)

// TextAttribute: recorded non-port, see the file header.
type TextAttribute struct{}

type LineRendition int

const LineRenditionSingleWidth LineRendition = 0

type msPoint struct{ x, y int }

// ROW (Row.hpp), text/wrap members.
type msROW struct {
	_charsBuffer []uint16 // the fixed-size backing buffer, _columnCount wide
	_charsHeap   []uint16 // nil unless _resizeChars spilled to the heap
	_chars       []uint16 // the active view: _charsBuffer or _charsHeap
	_charOffsets []uint16 // _columnCount+1 entries
	_columnCount int

	_lineRendition    LineRendition
	_wrapForced       bool
	_doubleBytePadded bool

	// writeWidth is tool metadata, not part of Microsoft's ROW. It records
	// the width at which this row first received output so a later resize can
	// still tell which live lines were exact-width at write time. The ported
	// row semantics never consult it.
	writeWidth int
}

// ROW::ROW
func newMsROW(rowWidth int) *msROW {
	r := &msROW{
		_charsBuffer: make([]uint16, rowWidth),
		_charOffsets: make([]uint16, rowWidth+1),
		_columnCount: rowWidth,
	}
	r._chars = r._charsBuffer
	r._init()
	return r
}

// ROW::_init, scalar path: fills _charsBuffer with whitespace and
// _charOffsets with successive numbers from 0 to _columnCount+1.
func (r *msROW) _init() {
	for i := 0; i < r._columnCount; i++ {
		r._charsBuffer[i] = ' '
	}
	for i := 0; i <= r._columnCount; i++ {
		r._charOffsets[i] = uint16(i)
	}
}

// ROW::Reset
func (r *msROW) Reset(attr TextAttribute) {
	r._charsHeap = nil
	r._chars = r._charsBuffer[:r._columnCount]
	_ = attr // color runs: recorded non-port
	r._lineRendition = LineRenditionSingleWidth
	r._wrapForced = false
	r._doubleBytePadded = false
	r.writeWidth = 0
	r._init()
}

func (r *msROW) SetWrapForced(wrap bool)         { r._wrapForced = wrap }
func (r *msROW) WasWrapForced() bool             { return r._wrapForced }
func (r *msROW) SetDoubleBytePadded(p bool)      { r._doubleBytePadded = p }
func (r *msROW) WasDoubleBytePadded() bool       { return r._doubleBytePadded }
func (r *msROW) GetLineRendition() LineRendition { return r._lineRendition }

// ROW::GetReadableColumnCount
func (r *msROW) GetReadableColumnCount() int {
	if r._lineRendition == LineRenditionSingleWidth {
		if r._doubleBytePadded {
			return r._columnCount - 1
		}
		return r._columnCount
	}
	pad := 0
	if r._doubleBytePadded {
		pad = 2
	}
	return (r._columnCount - pad) >> 1
}

func msClamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// ROW::_clampedColumn / _clampedColumnInclusive
func (r *msROW) _clampedColumn(v int) int          { return msClamp(v, 0, r._columnCount-1) }
func (r *msROW) _clampedColumnInclusive(v int) int { return msClamp(v, 0, r._columnCount) }

// ROW::_charSize
func (r *msROW) _charSize() int { return int(r._charOffsets[r._columnCount]) }

// ROW::_uncheckedCharOffset / _uncheckedIsTrailer
func (r *msROW) _uncheckedCharOffset(col int) int {
	return int(r._charOffsets[col] & CharOffsetsMask)
}
func (r *msROW) _uncheckedIsTrailer(col int) bool {
	return r._charOffsets[col]&CharOffsetsTrailer != 0
}

// ROW::_adjustBackward / _adjustForward
func (r *msROW) _adjustBackward(column int) int {
	for ; r._uncheckedIsTrailer(column); column-- {
	}
	return column
}
func (r *msROW) _adjustForward(column int) int {
	for ; r._uncheckedIsTrailer(column); column++ {
	}
	return column
}

// ROW::AdjustToGlyphStart
func (r *msROW) AdjustToGlyphStart(column int) int {
	return r._adjustBackward(r._clampedColumn(column))
}

// ROW::GetText
func (r *msROW) GetText() []uint16 {
	width := int(r._charOffsets[r.GetReadableColumnCount()] & CharOffsetsMask)
	return r._chars[:width]
}

// ROW::GetText(columnBegin, columnEnd)
func (r *msROW) GetTextRange(columnBegin, columnEnd int) []uint16 {
	columns := r.GetReadableColumnCount()
	colBeg := msClamp(columnBegin, 0, columns)
	colEnd := msClamp(columnEnd, colBeg, columns)
	chBeg := r._uncheckedCharOffset(colBeg)
	chEnd := r._uncheckedCharOffset(colEnd)
	return r._chars[chBeg:chEnd]
}

// ROW::GetLastNonSpaceColumn
func (r *msROW) GetLastNonSpaceColumn() int {
	text := r.GetText()
	beg := 0
	end := len(text)
	it := end

	for ; it != beg; it-- {
		if text[it-1] != ' ' {
			break
		}
	}

	return r.GetReadableColumnCount() - (end - it)
}

// ROW::MeasureRight
func (r *msROW) MeasureRight() int {
	if r._wrapForced {
		width := r._columnCount
		if r._doubleBytePadded {
			width--
		}
		return width
	}
	return r.GetLastNonSpaceColumn()
}

// struct RowWriteState (Row.hpp)
type msRowWriteState struct {
	text        []uint16 // IN/OUT
	columnBegin int      // IN
	columnLimit int      // IN

	columnEnd        int // OUT
	columnBeginDirty int // OUT
	columnEndDirty   int // OUT
}

// struct RowCopyTextFromState (Row.hpp)
type msRowCopyTextFromState struct {
	source            *msROW // IN
	columnBegin       int    // IN
	columnLimit       int    // IN
	sourceColumnBegin int    // IN
	sourceColumnLimit int    // IN
	columnEnd         int    // OUT
	columnBeginDirty  int    // OUT
	columnEndDirty    int    // OUT
	sourceColumnEnd   int    // OUT
}

// ROW::WriteHelper
type msWriteHelper struct {
	row   *msROW
	chars []uint16

	colBeg        int
	colLimit      int
	chBegDirty    int
	colBegDirty   int
	leadingSpaces int
	chBeg         int
	colEnd        int
	colEndDirty   int
	charsConsumed int
}

// ROW::WriteHelper::WriteHelper
func newMsWriteHelper(row *msROW, columnBegin, columnLimit int, chars []uint16) msWriteHelper {
	var h msWriteHelper
	h.row = row
	h.chars = chars
	h.colBeg = row._clampedColumnInclusive(columnBegin)
	h.colLimit = row._clampedColumnInclusive(columnLimit)
	h.chBegDirty = row._uncheckedCharOffset(h.colBeg)
	h.colBegDirty = row._adjustBackward(h.colBeg)
	h.leadingSpaces = h.colBeg - h.colBegDirty
	h.chBeg = h.chBegDirty + h.leadingSpaces
	h.colEnd = h.colBeg
	h.colEndDirty = 0
	h.charsConsumed = 0
	return h
}

// ROW::WriteHelper::IsValid
func (h *msWriteHelper) IsValid() bool {
	return h.colBeg < h.colLimit && len(h.chars) != 0
}

// ROW::ClearCell
func (r *msROW) ClearCell(column int) {
	space := []uint16{' '}
	r.ReplaceCharacters(column, 1, space)
}

// ROW::ReplaceCharacters
func (r *msROW) ReplaceCharacters(columnBegin, width int, chars []uint16) {
	h := newMsWriteHelper(r, columnBegin, r._columnCount, chars)
	if !h.IsValid() {
		return
	}
	h.ReplaceCharactersW(width)
	h.Finish()
}

// ROW::WriteHelper::ReplaceCharacters
func (h *msWriteHelper) ReplaceCharactersW(width int) {
	colEndNew := h.colEnd + width
	if colEndNew > h.colLimit {
		h.colEndDirty = h.colLimit
	} else {
		h.row._charOffsets[h.colEnd] = uint16(h.chBeg)
		h.colEnd++
		for ; h.colEnd < colEndNew; h.colEnd++ {
			h.row._charOffsets[h.colEnd] = uint16(h.chBeg) | CharOffsetsTrailer
		}

		h.colEndDirty = h.colEnd
		h.charsConsumed = len(h.chars)
	}
}

// ROW::ReplaceText
func (r *msROW) ReplaceText(state *msRowWriteState) {
	h := newMsWriteHelper(r, state.columnBegin, state.columnLimit, state.text)
	if !h.IsValid() {
		state.columnEnd = h.colBeg
		state.columnBeginDirty = h.colBeg
		state.columnEndDirty = h.colBeg
		return
	}
	h.ReplaceText()
	h.Finish()

	state.text = state.text[h.charsConsumed:]
	// Here's why we set `state.columnEnd` to `colLimit` if there's remaining text:
	// Callers should be able to use `state.columnEnd` as the next cursor position, as well as the parameter for a
	// follow-up call to ReplaceAttributes(). But if we fail to insert a wide glyph into the last column of a row,
	// that last cell (which now contains padding whitespace) should get the same attributes as the rest of the
	// string so that the row looks consistent. This requires us to return `colLimit` instead of `colLimit - 1`.
	// Additionally, this has the benefit that callers can detect line wrapping by checking `columnEnd >= columnLimit`.
	if len(state.text) == 0 {
		state.columnEnd = h.colEnd
	} else {
		state.columnEnd = h.colLimit
	}
	state.columnBeginDirty = h.colBegDirty
	state.columnEndDirty = h.colEndDirty
}

// ROW::WriteHelper::ReplaceText
func (h *msWriteHelper) ReplaceText() {
	// This function starts with a fast-pass for ASCII. ASCII is still predominant in technical areas.
	//
	// We can infer the "end" from the amount of columns we're given (colLimit - colBeg),
	// because ASCII is always 1 column wide per character.
	it := 0
	end := it + len(h.chars)
	if lim := h.colLimit - h.colBeg; lim < end {
		end = lim
	}
	ch := h.chBeg

	for it != end {
		if h.chars[it] >= 0x80 {
			h._replaceTextUnicode(ch, it)
			return
		}

		h.row._charOffsets[h.colEnd] = uint16(ch)
		h.colEnd++
		ch++
		it++
	}

	h.colEndDirty = h.colEnd
	h.charsConsumed = ch - h.chBeg
}

// ROW::WriteHelper::_replaceTextUnicode
func (h *msWriteHelper) _replaceTextUnicode(ch int, it int) {
	// Check if the new text joins with the existing contents of the row to form a single grapheme cluster.
	if it == 0 {
		colPrev := h.colBeg
		for colPrev > 0 {
			colPrev--
			if !h.row._uncheckedIsTrailer(colPrev) {
				break
			}
		}

		chPrev := h.row._uncheckedCharOffset(colPrev)
		charsPrev := h.row._chars[chPrev:ch]

		var state msGraphemeState
		msGraphemeNext(&state, charsPrev)
		msGraphemeNext(&state, h.chars)

		if state.len > 0 {
			h.colBegDirty = colPrev
			h.colEnd = colPrev

			width := state.width
			if width < 1 {
				width = 1
			}
			colEndNew := h.colEnd + width
			if colEndNew > h.colLimit {
				h.colEndDirty = h.colLimit
				h.charsConsumed = ch - h.chBeg
				return
			}

			// Fill our char-offset buffer with 1 entry containing the mapping from the
			// current column (colEnd) to the start of the glyph in the string (ch)...
			h.row._charOffsets[h.colEnd] = uint16(chPrev)
			h.colEnd++
			// ...followed by 0-N entries containing an indication that the
			// columns are just a wide-glyph extension of the preceding one.
			for h.colEnd < colEndNew {
				h.row._charOffsets[h.colEnd] = uint16(chPrev) | CharOffsetsTrailer
				h.colEnd++
			}

			ch += state.len
			it += state.len
		}
	} else {
		// The non-ASCII character we have encountered may be a combining mark, like "a^" which is then displayed as "â".
		// In order to recognize both characters as a single grapheme, we need to back up by 1 ASCII character
		// and let MeasureNext() find the next proper grapheme boundary.
		h.colEnd--
		ch--
		it--
	}

	if end := len(h.chars); it != end {
		state := msGraphemeState{beg: it, len: 0}

		for {
			msGraphemeNext(&state, h.chars)

			width := state.width
			if width < 1 {
				width = 1
			}
			colEndNew := h.colEnd + width
			if colEndNew > h.colLimit {
				h.colEndDirty = h.colLimit
				h.charsConsumed = ch - h.chBeg
				return
			}

			// Fill our char-offset buffer with 1 entry containing the mapping from the
			// current column (colEnd) to the start of the glyph in the string (ch)...
			h.row._charOffsets[h.colEnd] = uint16(ch)
			h.colEnd++
			// ...followed by 0-N entries containing an indication that the
			// columns are just a wide-glyph extension of the preceding one.
			for h.colEnd < colEndNew {
				h.row._charOffsets[h.colEnd] = uint16(ch) | CharOffsetsTrailer
				h.colEnd++
			}

			ch += state.len
			it += state.len
			if it == end {
				break
			}
		}
	}

	h.colEndDirty = h.colEnd
	h.charsConsumed = ch - h.chBeg
}

// ROW::CopyTextFrom
func (r *msROW) CopyTextFrom(state *msRowCopyTextFromState) {
	source := state.source
	sourceColBeg := source._clampedColumnInclusive(state.sourceColumnBegin)
	sourceColLimit := source._clampedColumnInclusive(state.sourceColumnLimit)
	var charOffsets []uint16
	var chars []uint16

	if sourceColBeg < sourceColLimit {
		charOffsets = source._charOffsets[sourceColBeg : sourceColLimit+1]
		beg := int(charOffsets[0] & CharOffsetsMask)
		end := int(charOffsets[len(charOffsets)-1] & CharOffsetsMask)
		chars = source._chars[beg:end]
	}

	h := newMsWriteHelper(r, state.columnBegin, state.columnLimit, chars)

	if !h.IsValid() ||
		// If we were to copy text from ourselves, we'd overwrite
		// our _charOffsets and break Finish() which reads from it.
		r == state.source ||
		// Any valid charOffsets array is at least 2 elements long (the 1st element is the start offset and the 2nd
		// element is the length of the first glyph) and begins/ends with a non-trailer offset. We don't really
		// need to test for the end offset, since `WriteHelper::WriteWithOffsets` already takes care of that.
		len(charOffsets) < 2 || charOffsets[0]&CharOffsetsTrailer != 0 {
		state.columnEnd = h.colBeg
		state.columnBeginDirty = h.colBeg
		state.columnEndDirty = h.colBeg
		state.sourceColumnEnd = source._columnCount
		return
	}

	h.CopyTextFrom(charOffsets)
	h.Finish()

	// state.columnEnd is computed identical to ROW::ReplaceText. Check it out for more information.
	if h.charsConsumed == len(chars) {
		state.columnEnd = h.colEnd
	} else {
		state.columnEnd = h.colLimit
	}
	state.columnBeginDirty = h.colBegDirty
	state.columnEndDirty = h.colEndDirty
	state.sourceColumnEnd = sourceColBeg + h.colEnd - h.colBeg
}

// ROW::WriteHelper::CopyTextFrom
func (h *msWriteHelper) CopyTextFrom(charOffsets []uint16) {
	// Since our `charOffsets` input is already in columns (just like the `ROW::_charOffsets`),
	// we can directly look up the end char-offset, but...
	colEndDirtyInput := h.colLimit - h.colBeg
	if n := len(charOffsets) - 1; n < colEndDirtyInput {
		colEndDirtyInput = n
	}

	// ...since the colLimit might intersect with a wide glyph in `charOffset`, we need to adjust our input-colEnd.
	colEndInput := colEndDirtyInput
	for ; charOffsets[colEndInput]&CharOffsetsTrailer != 0; colEndInput-- {
	}

	baseOffset := int(charOffsets[0])
	endOffset := int(charOffsets[colEndInput])
	inToOutOffset := uint16(h.chBeg - baseOffset)

	msCopyOffsets(h.row._charOffsets[h.colEnd:], charOffsets, colEndInput, inToOutOffset)

	h.colEnd += colEndInput
	h.colEndDirty = h.colBeg + colEndDirtyInput
	h.charsConsumed = endOffset - baseOffset
}

// ROW::WriteHelper::_copyOffsets
func msCopyOffsets(dst, src []uint16, size int, offset uint16) {
	for i := 0; i < size; i++ {
		ch := src[i]
		off := ch & CharOffsetsMask
		trailer := ch & CharOffsetsTrailer
		newOff := off + offset
		dst[i] = newOff | trailer
	}
}

// ROW::WriteHelper::Finish
func (h *msWriteHelper) Finish() {
	h.colEndDirty = h.row._adjustForward(h.colEndDirty)

	trailingSpaces := h.colEndDirty - h.colEnd
	chEndDirtyOld := h.row._uncheckedCharOffset(h.colEndDirty)
	chEndDirty := h.chBegDirty + h.charsConsumed + h.leadingSpaces + trailingSpaces

	if chEndDirty != chEndDirtyOld {
		h.row._resizeChars(h.colEndDirty, h.chBegDirty, chEndDirty, chEndDirtyOld)
	}

	{
		copy(h.row._chars[h.chBeg:], h.chars[:h.charsConsumed])

		if h.leadingSpaces != 0 {
			for i := 0; i < h.leadingSpaces; i++ {
				h.row._chars[h.chBegDirty+i] = ' '
				h.row._charOffsets[h.colBegDirty+i] = uint16(h.chBegDirty + i)
			}
		}
		if trailingSpaces != 0 {
			for i := 0; i < trailingSpaces; i++ {
				h.row._chars[h.chBeg+h.charsConsumed+i] = ' '
				h.row._charOffsets[h.colEnd+i] = uint16(h.chBeg + h.charsConsumed + i)
			}
		}
	}

	// This updates `_doubleBytePadded` whenever we write the last column in the row. `_doubleBytePadded` tells our text
	// reflow algorithm whether it should ignore the last column. This is important when writing wide characters into
	// the terminal: If the last wide character in a row only fits partially, we should render whitespace, but
	// during text reflow pretend as if no whitespace exists. After all, the user didn't write any whitespace there.
	//
	// The way this is written, it'll set `_doubleBytePadded` to `true` no matter whether a wide character didn't fit,
	// or if the last 2 columns contain a wide character and a narrow character got written into the left half of it.
	// In both cases `trailingSpaces` is 1 and fills the last column and `_doubleBytePadded` will be `true`.
	if h.colEndDirty == h.row._columnCount {
		h.row.SetDoubleBytePadded(h.colEnd < h.row._columnCount)
	}
}

// ROW::_resizeChars
func (r *msROW) _resizeChars(colEndDirty, chBegDirty, chEndDirty, chEndDirtyOld int) {
	diff := chEndDirty - chEndDirtyOld
	currentLength := r._charSize()
	newLength := currentLength + diff

	if newLength <= len(r._chars) {
		copy(r._chars[chEndDirty:], r._chars[chEndDirtyOld:currentLength])
	} else {
		minCapacity := len(r._chars) + (len(r._chars) >> 1)
		if minCapacity > 0xFFFF {
			minCapacity = 0xFFFF
		}
		newCapacity := newLength
		if minCapacity > newCapacity {
			newCapacity = minCapacity
		}

		charsHeap := make([]uint16, newCapacity)
		copy(charsHeap, r._chars[:chBegDirty])
		copy(charsHeap[chEndDirty:], r._chars[chEndDirtyOld:currentLength])

		r._charsHeap = charsHeap
		r._chars = charsHeap
	}

	for it := colEndDirty; it < len(r._charOffsets); it++ {
		r._charOffsets[it] = uint16(int(r._charOffsets[it]) + diff)
	}
}

// ROW::CopyFrom
func (r *msROW) CopyFrom(source *msROW) {
	r._lineRendition = source._lineRendition
	r._wrapForced = source._wrapForced
	r.writeWidth = source.writeWidth

	state := msRowCopyTextFromState{
		source:            source,
		sourceColumnLimit: source.GetReadableColumnCount(),
		columnLimit:       0x7fffffff,
	}
	r.CopyTextFrom(&state)
}

// ROW::SetAttrToEnd / ReplaceAttributes: recorded non-port (colors),
// kept so call sites match the original line for line.
func (r *msROW) SetAttrToEnd(columnBegin int, attr TextAttribute)               {}
func (r *msROW) ReplaceAttributes(beginIndex, endIndex int, attr TextAttribute) {}
