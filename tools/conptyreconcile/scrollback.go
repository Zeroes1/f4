package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
)

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
