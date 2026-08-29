// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// The terminal-side checks in this file are probe logic.  They never supply
// host semantics: host text and frame order come from the pinned mock/host
// session, while the assertions preserve input order, multiplicity, and byte
// identity.

package main

import (
	"bytes"
	"fmt"
	"sort"
	"time"
)

type streamKind uint8

const (
	streamLive streamKind = iota
	streamFrame
	streamResize
	streamMarker
)

type streamEvent struct {
	Sequence uint64     `json:"sequence"`
	At       time.Time  `json:"at"`
	Kind     streamKind `json:"kind"`
	Width    int        `json:"width,omitempty"`
	Height   int        `json:"height,omitempty"`
	Bytes    []byte     `json:"bytes,omitempty"`
	Cause    string     `json:"cause,omitempty"`
}

type capture struct {
	Seed       int64
	HostPath   string
	HostSHA256 string
	Events     []streamEvent
}

func (c *capture) append(kind streamKind, width, height int, data []byte, cause string) {
	c.Events = append(c.Events, streamEvent{
		Sequence: uint64(len(c.Events)),
		At:       time.Now().UTC(),
		Kind:     kind,
		Width:    width,
		Height:   height,
		Bytes:    bytes.Clone(data),
		Cause:    cause,
	})
}

func (c capture) liveBytes() []byte {
	var result []byte
	for _, event := range c.Events {
		if event.Kind == streamLive || event.Kind == streamFrame {
			result = append(result, event.Bytes...)
		}
	}
	return result
}

type frameRow struct {
	Units       []uint16 `json:"units"`
	WrapForced  bool     `json:"wrap_forced"`
	DoublePad   bool     `json:"double_byte_padded"`
	SourceIndex int      `json:"source_index"`
}

type frame struct {
	Width       int        `json:"width"`
	Height      int        `json:"height"`
	Cursor      coordinate `json:"cursor"`
	Rows        []frameRow `json:"rows"`
	Cause       string     `json:"cause"`
	Sequence    uint64     `json:"sequence"`
	PartialTop  bool       `json:"partial_top"`
	EvictedRows int        `json:"evicted_rows"`
}

func frameFromBuffer(b *textBuffer, cause string, sequence uint64) frame {
	result := frame{Width: b.width, Height: b.height, Cursor: b.cursor.position, Cause: cause, Sequence: sequence}
	for i := 0; i < b.height; i++ {
		row := b.rowByOffset(i)
		limit := row.charRow.measureRight()
		if row.wrapForced {
			limit = b.lineWidth(i)
			if row.doubleBytePadded && limit > 0 {
				limit--
			}
		}
		units := make([]uint16, 0, limit)
		for col := 0; col < limit; col++ {
			if row.charRow.data[col].attr.isTrailing() {
				continue
			}
			units = append(units, row.charRow.glyphAt(col)...)
		}
		result.Rows = append(result.Rows, frameRow{Units: units, WrapForced: row.wrapForced, DoublePad: row.doubleBytePadded, SourceIndex: i})
	}
	if len(result.Rows) > 0 {
		result.PartialTop = result.Rows[0].WrapForced
	}
	return result
}

// frameBytesFromBuffer drives the pinned VtEngine/XtermEngine transcription
// over the current rows. The returned bytes are frame data; live input stays
// in the capture as its own streamLive events.
func frameBytesFromBuffer(b *textBuffer) []byte {
	emitter := newFrameEmitter(b.width, b.height)
	emitter.startPaint()
	for rowIndex := 0; rowIndex < b.height; rowIndex++ {
		row := b.rowByOffset(rowIndex)
		limit := row.charRow.measureRight()
		if row.wrapForced {
			limit = b.lineWidth(rowIndex)
			if row.doubleBytePadded && limit > 0 {
				limit--
			}
		}
		clusters := make([]frameCluster, 0, limit)
		for column := 0; column < limit; column++ {
			if row.charRow.data[column].attr.isTrailing() {
				continue
			}
			columns := 1
			if row.charRow.data[column].attr.isLeading() {
				columns = 2
			}
			clusters = append(clusters, frameCluster{Units: row.charRow.glyphAt(column), Columns: columns})
		}
		emitter.paint(clusters, coordinate{x: 0, y: rowIndex}, row.wrapForced)
	}
	emitter.endPaint()
	return bytes.Clone(emitter.output)
}

func (f frame) logicalLines() []logicalRow {
	result := make([]logicalRow, 0, len(f.Rows))
	for i, row := range f.Rows {
		if len(result) == 0 || !result[len(result)-1].continues {
			result = append(result, logicalRow{sourceStart: i})
		}
		line := &result[len(result)-1]
		line.rows = append(line.rows, i)
		line.units = append(line.units, row.Units...)
		line.continues = row.WrapForced
	}
	return result
}

type resizeEvent struct {
	Width  int
	Height int
	Order  int
}

type scenario struct {
	Seed          int64
	InitialWidth  int
	InitialHeight int
	Input         []byte
	ExpectedText  string
	Chunks        [][]byte
	Resizes       []resizeEvent
	Frame         frame
	Command       string
}

func splitAtBoundaries(data []byte, boundaries []int) [][]byte {
	result := make([][]byte, 0, len(boundaries)+1)
	last := 0
	for _, boundary := range boundaries {
		if boundary <= last || boundary >= len(data) {
			continue
		}
		result = append(result, bytes.Clone(data[last:boundary]))
		last = boundary
	}
	result = append(result, bytes.Clone(data[last:]))
	return result
}

func allByteChunks(data []byte) [][]byte {
	result := make([][]byte, len(data))
	for i, b := range data {
		result[i] = []byte{b}
	}
	return result
}

func parseWithChunks(width, height int, chunks [][]byte) (*vtParser, error) {
	parser := newVTParser(width, height)
	for _, chunk := range chunks {
		if err := parser.feed(chunk); err != nil {
			return nil, err
		}
	}
	if err := parser.finish(); err != nil {
		return nil, err
	}
	return parser, nil
}

// parseCapturedFrameEvents replays the host's serialized event order. A
// resize is applied at the exact point at which the recorder observed it;
// frame bytes are either handed to the parser as ReadFile chunks or split at
// every byte. This keeps resize ordering separate from parser chunking.
func parseCapturedFrameEvents(width, height int, events []streamEvent, byteAtATime bool) (*vtParser, error) {
	parser := newVTParser(width, height)
	for _, event := range events {
		switch event.Kind {
		case streamResize:
			if err := parser.resize(event.Width, event.Height); err != nil {
				return nil, err
			}
		case streamFrame:
			if byteAtATime {
				for _, value := range event.Bytes {
					if err := parser.feed([]byte{value}); err != nil {
						return nil, err
					}
				}
			} else if err := parser.feed(event.Bytes); err != nil {
				return nil, err
			}
		}
	}
	if err := parser.finish(); err != nil {
		return nil, err
	}
	return parser, nil
}

func expectedPrintableText(input []byte) string {
	// This is an input invariant, not a second terminal model.  It strips
	// control sequences only for the generated scenario's explicit printable
	// payload and is compared to the generator's recorded payload separately.
	var result []byte
	for len(input) > 0 {
		if input[0] == 0x1b {
			if len(input) >= 2 && input[1] == '[' {
				input = input[2:]
				for len(input) > 0 && (input[0] < 0x40 || input[0] > 0x7e) {
					input = input[1:]
				}
				if len(input) > 0 {
					input = input[1:]
				}
				continue
			}
			if len(input) >= 2 && input[1] == ']' {
				input = input[2:]
				for len(input) > 0 && input[0] != 0x07 {
					if input[0] == 0x1b && len(input) > 1 && input[1] == '\\' {
						input = input[2:]
						break
					}
					input = input[1:]
				}
				if len(input) > 0 && input[0] == 0x07 {
					input = input[1:]
				}
				continue
			}
			if len(input) > 1 {
				input = input[2:]
			} else {
				input = nil
			}
			continue
		}
		if input[0] >= 0x20 && input[0] != 0x7f {
			result = append(result, input[0])
		}
		input = input[1:]
	}
	return string(result)
}

func mirrorLines(parser *vtParser) []logicalRow { return parser.buffer.logicalRows() }

func rewrapLines(lines []logicalRow, width int) []string {
	if width <= 0 {
		return nil
	}
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		units := line.units
		if len(units) == 0 {
			result = append(result, "")
			continue
		}
		start := 0
		columns := 0
		for start < len(units) {
			glyph := nextUTF16Glyph(units[start:])
			glyphWidth := 1
			if newWidthDetector().IsWide(glyph) {
				glyphWidth = 2
			}
			if columns > 0 && columns+glyphWidth > width {
				result = append(result, string(runesFromUTF16(units[:start])))
				units = units[start:]
				start = 0
				columns = 0
				continue
			}
			columns += glyphWidth
			start += len(glyph)
			if columns >= width {
				result = append(result, string(runesFromUTF16(units[:start])))
				units = units[start:]
				start = 0
				columns = 0
			}
		}
		if len(units) > 0 || columns == 0 && len(result) == 0 {
			result = append(result, string(runesFromUTF16(units)))
		}
	}
	return result
}

func nextUTF16Glyph(units []uint16) []uint16 {
	if len(units) == 0 {
		return nil
	}
	return utf16ParseNext(units)
}

type viewport struct {
	Top    int
	Left   int
	Width  int
	Height int
}

func (v viewport) clampScroll(scroll, contentHeight int) int {
	max := contentHeight - v.Height
	if max < 0 {
		max = 0
	}
	if scroll < 0 {
		return 0
	}
	if scroll > max {
		return max
	}
	return scroll
}

func mapCoordinate(lines []string, width int, row, col int) (line, cell int, ok bool) {
	if width <= 0 || row < 0 || col < 0 || col >= width {
		return 0, 0, false
	}
	flatRow := 0
	for lineIndex, text := range lines {
		units := utf16Units(text)
		used := 0
		for len(units) > 0 {
			glyph := nextUTF16Glyph(units)
			glyphWidth := 1
			if newWidthDetector().IsWide(glyph) {
				glyphWidth = 2
			}
			if flatRow == row && col >= used && col < used+glyphWidth {
				return lineIndex, used, true
			}
			used += glyphWidth
			units = units[len(glyph):]
			if used == width {
				flatRow++
				used = 0
			}
		}
		if flatRow == row && col < used {
			return lineIndex, col, true
		}
		flatRow++
	}
	return 0, 0, false
}

func reconcile(frame frame, live []logicalRow) error {
	frameLines := frame.logicalLines()
	if len(frameLines) == 0 {
		return fmt.Errorf("frame has no rows")
	}
	if len(live) == 0 {
		return fmt.Errorf("live mirror has no rows")
	}
	frameIndex := 0
	liveIndex := 0
	if frame.PartialTop && len(live[0].units) > 0 {
		// A circular buffer may begin after the first physical row of a
		// logical line.  Compare only the visible suffix, retaining order.
		frameIndex++
	}
	for frameIndex < len(frameLines) && liveIndex < len(live) {
		if frameLines[frameIndex].text() != live[liveIndex].text() {
			return fmt.Errorf("ordered frame/live mismatch frame=%d live=%d frame=%q live=%q", frameIndex, liveIndex, frameLines[frameIndex].text(), live[liveIndex].text())
		}
		frameIndex++
		liveIndex++
	}
	if frameIndex != len(frameLines) {
		return fmt.Errorf("frame has %d unreconciled logical rows", len(frameLines)-frameIndex)
	}
	return nil
}

func sortedResizeEvents(events []resizeEvent) []resizeEvent {
	result := append([]resizeEvent(nil), events...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].Order < result[j].Order })
	return result
}
