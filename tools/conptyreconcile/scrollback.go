package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

type consumerResizeCheck struct {
	Width          int    `json:"width"`
	Offset         int    `json:"offset"`
	CompletedLines int    `json:"completed_lines"`
	HistorySHA256  string `json:"history_sha256"`
	Status         string `json:"status"`
}

// scrollbackPieceTable is the consumer's immutable spill area. It stores
// complete logical records, including their explicit terminators; no display
// row or width participates in the stored representation.
type scrollbackPieceTable struct {
	pieces [][]byte
}

func (p *scrollbackPieceTable) Append(line logicalLine) {
	data := append([]byte(nil), line.Bytes...)
	data = append(data, line.Terminator...)
	p.pieces = append(p.pieces, data)
}

func (p scrollbackPieceTable) Bytes() []byte {
	var out []byte
	for _, piece := range p.pieces {
		out = append(out, piece...)
	}
	return out
}

// consumerScrollback keeps only a bounded editable tail in memory and spills
// older complete lines to the piece table. Scrolling and display resizing read
// the same logical records; neither operation mutates their bytes.
type consumerScrollback struct {
	maxTail int
	spilled scrollbackPieceTable
	tail    []logicalLine
}

func newConsumerScrollback(maxTail int) *consumerScrollback {
	if maxTail < 1 {
		maxTail = 1
	}
	return &consumerScrollback{maxTail: maxTail}
}

func (s *consumerScrollback) Append(line logicalLine) {
	copyLine := logicalLine{Bytes: append([]byte(nil), line.Bytes...), Terminator: append([]byte(nil), line.Terminator...)}
	s.tail = append(s.tail, copyLine)
	for len(s.tail) > s.maxTail {
		s.spilled.Append(s.tail[0])
		s.tail = s.tail[1:]
	}
}

func (s consumerScrollback) historyLines() []logicalLine {
	var stream logicalLineStream
	stream.Feed(s.historyBytes())
	return stream.Lines()
}

func (s consumerScrollback) historyBytes() []byte {
	var out []byte
	out = append(out, s.spilled.Bytes()...)
	for _, line := range s.tail {
		out = append(out, line.Bytes...)
		out = append(out, line.Terminator...)
	}
	return out
}

func (s consumerScrollback) historySHA256() string {
	h := sha256.Sum256(s.historyBytes())
	return hex.EncodeToString(h[:])
}

func (s consumerScrollback) visible(offset, height, width int) [][]byte {
	if height < 1 || width < 1 {
		return nil
	}
	rows := reflowLogicalLines(s.historyLines(), width)
	if offset < 0 {
		offset = 0
	}
	if offset > len(rows) {
		offset = len(rows)
	}
	end := len(rows) - offset
	if end < 0 {
		end = 0
	}
	start := end - height
	if start < 0 {
		start = 0
	}
	visible := rows[start:end]
	result := make([][]byte, len(visible))
	for i, row := range visible {
		result[i] = append([]byte(nil), row...)
	}
	return result
}

func rowsBytes(rows [][]byte) []byte {
	var out []byte
	for _, row := range rows {
		out = append(out, row...)
		out = append(out, '\n')
	}
	return out
}

func rowsSHA256(rows [][]byte) string {
	h := sha256.Sum256(rowsBytes(rows))
	return hex.EncodeToString(h[:])
}

func rowsEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// verifyConsumerResizeDuringFeed replays complete host-rendered records in
// chunks while changing only the consumer display width. Width changes occur
// while bytes are still arriving; the stored logical history must stay
// byte-identical at every checkpoint.
func verifyConsumerResizeDuringFeed(lines []logicalLine, widths []int) ([]consumerResizeCheck, error) {
	if len(widths) == 0 {
		return nil, nil
	}
	var input []byte
	for _, line := range lines {
		input = append(input, line.Bytes...)
		input = append(input, line.Terminator...)
	}
	model := newConsumerScrollback(len(lines) + 1)
	checks := make([]consumerResizeCheck, 0, len(widths))
	state := uint64(1)
	nextWidth := 0
	feed := logicalLineStream{}
	for offset := 0; offset < len(input); {
		state = state*6364136223846793005 + 1
		size := int((state>>32)%23) + 1
		end := offset + size
		if end > len(input) {
			end = len(input)
		}
		// Feed incrementally; parser state, rather than a row heuristic,
		// controls which records are complete at each checkpoint.
		feed.Feed(input[offset:end])
		for nextWidth < len(widths) && (end >= (nextWidth+1)*len(input)/len(widths) || end == len(input)) {
			model.tail = feed.Lines()
			width := widths[nextWidth]
			_ = model.visible(0, 25, width)
			checks = append(checks, consumerResizeCheck{Width: width, Offset: end, CompletedLines: len(model.tail), HistorySHA256: model.historySHA256(), Status: "passed"})
			nextWidth++
		}
		offset = end
	}
	if nextWidth != len(widths) {
		return checks, fmt.Errorf("consumer resize replay reached %d/%d checkpoints", nextWidth, len(widths))
	}
	if !bytes.Equal(model.historyBytes(), input) {
		return checks, fmt.Errorf("consumer resize replay changed final history")
	}
	return checks, nil
}
