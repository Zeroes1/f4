// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Transcription of the pinned Utf8ToWideCharParser byte classification and
// partial-sequence path from src/host/utf8ToWideCharParser.cpp.  The Windows
// MultiByteToWideChar(CP_UTF8) call is represented by the explicitly isolated
// decodeUTF8Sequence helper below; it is not a terminal-semantic substitute.

package main

import "fmt"

const (
	utf8ByteSequenceMax = 4
	codePageUTF8        = 65001
	nonasciiBytePrefix  = 0x80
	continuationMask    = 0xc0
	continuationPrefix  = 0x80
	mostSignificantMask = 0x80
)

type utf8ParserState uint8

const (
	utf8Ready utf8ParserState = iota
	utf8Error
	utf8BeginPartialParse
	utf8AwaitingMoreBytes
	utf8Finished
)

type utf8WideParser struct {
	utf8CodePointPieces []byte
	bytesStored         int
	currentCodePage     uint32
	currentState        utf8ParserState
}

func newUTF8WideParser() *utf8WideParser {
	return &utf8WideParser{utf8CodePointPieces: make([]byte, utf8ByteSequenceMax), currentCodePage: codePageUTF8, currentState: utf8Ready}
}

func (p *utf8WideParser) feed(input []byte) ([]uint16, error) {
	if len(input) == 0 {
		return nil, nil
	}
	if p.currentCodePage != codePageUTF8 {
		p.currentState = utf8Error
		p.bytesStored = 0
		p.currentState = utf8Ready
		return nil, fmt.Errorf("Utf8ToWideCharParser: code page %d is not UTF-8", p.currentCodePage)
	}
	if p.currentState == utf8AwaitingMoreBytes {
		p.currentState = utf8BeginPartialParse
	}
	if p.currentState == utf8Ready {
		if isValidUTF8Range(input) {
			units, err := decodeUTF8SequencesStrict(input)
			if err == nil {
				p.currentState = utf8Finished
				p.currentState = utf8Ready
				return units, nil
			}
		}
		p.currentState = utf8BeginPartialParse
	}
	if p.currentState != utf8BeginPartialParse {
		return nil, fmt.Errorf("Utf8ToWideCharParser: invalid parser state %d", p.currentState)
	}
	combined := make([]byte, 0, p.bytesStored+len(input))
	combined = append(combined, p.utf8CodePointPieces[:p.bytesStored]...)
	combined = append(combined, input...)
	p.bytesStored = 0
	valid, partial := p.removeInvalidSequences(combined)
	if partial != nil {
		p.storePartialSequence(partial)
	}
	if len(valid) == 0 && p.bytesStored > 0 {
		p.currentState = utf8AwaitingMoreBytes
		return nil, nil
	}
	units, err := decodeUTF8Sequences(valid)
	if err != nil {
		p.currentState = utf8Error
		p.bytesStored = 0
		p.currentState = utf8Ready
		return nil, err
	}
	if p.bytesStored > 0 {
		p.currentState = utf8AwaitingMoreBytes
	} else {
		p.currentState = utf8Finished
		p.currentState = utf8Ready
	}
	return units, nil
}

func (p *utf8WideParser) finish() ([]uint16, error) {
	// The pinned parser remains AwaitingMoreBytes when the final input ends in
	// a partial sequence.  It does not invent U+FFFD at the stream boundary.
	p.bytesStored = 0
	p.currentState = utf8Ready
	return nil, nil
}

func (p *utf8WideParser) setCodePage(codePage uint32) {
	if p.currentCodePage != codePage {
		p.currentCodePage = codePage
		p.bytesStored = 0
		p.currentState = utf8Ready
	}
}

func isUTF8LeadByte(ch byte) bool {
	sequenceSize := utf8SequenceSize(ch)
	return !isUTF8ContinuationByte(ch) && !isUTF8ASCIIByte(ch) && sequenceSize > 1 && sequenceSize <= utf8ByteSequenceMax
}

func isUTF8ContinuationByte(ch byte) bool {
	return ch&continuationMask == continuationPrefix
}

func isUTF8ASCIIByte(ch byte) bool {
	return ch&nonasciiBytePrefix == 0
}

func isValidUTF8MultiByteSequence(input []byte) bool {
	if len(input) == 0 || !isUTF8LeadByte(input[0]) {
		return false
	}
	sequenceSize := utf8SequenceSize(input[0])
	if sequenceSize > len(input) {
		return false
	}
	for i := 1; i < sequenceSize; i++ {
		if !isUTF8ContinuationByte(input[i]) {
			return false
		}
	}
	return true
}

func isValidUTF8Range(input []byte) bool {
	for i := 0; i < len(input); {
		if input[i] < 0x80 {
			i++
			continue
		}
		if !isValidUTF8MultiByteSequence(input[i:]) {
			return false
		}
		size := utf8SequenceSize(input[i])
		codepoint := rune(input[i] & (0x7f >> size))
		for j := 1; j < size; j++ {
			codepoint = codepoint<<6 | rune(input[i+j]&0x3f)
		}
		minimal := (size == 2 && codepoint >= 0x80) || (size == 3 && codepoint >= 0x800) || (size == 4 && codepoint >= 0x10000)
		if !minimal || codepoint > 0x10ffff || codepoint >= 0xd800 && codepoint <= 0xdfff {
			return false
		}
		i += size
	}
	return true
}

func isPartialUTF8MultiByteSequence(input []byte) bool {
	if len(input) == 0 || !isUTF8LeadByte(input[0]) {
		return false
	}
	sequenceSize := utf8SequenceSize(input[0])
	if sequenceSize <= len(input) {
		return false
	}
	for i := 1; i < len(input); i++ {
		if !isUTF8ContinuationByte(input[i]) {
			return false
		}
	}
	return true
}

func utf8SequenceSize(ch byte) int {
	msbOnes := 0
	for ch&mostSignificantMask != 0 {
		msbOnes++
		ch <<= 1
	}
	return msbOnes
}

// removeInvalidSequences is Utf8ToWideCharParser::_RemoveInvalidSequences.
// A partial sequence is returned separately because the source stores it for
// the next Parse call instead of passing it to MultiByteToWideChar.
func (p *utf8WideParser) removeInvalidSequences(input []byte) ([]byte, []byte) {
	valid := make([]byte, 0, len(input))
	for current := 0; current < len(input); {
		if isUTF8ASCIIByte(input[current]) {
			valid = append(valid, input[current])
			current++
		} else if isUTF8ContinuationByte(input[current]) {
			for current < len(input) && isUTF8ContinuationByte(input[current]) {
				current++
			}
		} else if isUTF8LeadByte(input[current]) {
			remaining := input[current:]
			if isValidUTF8MultiByteSequence(remaining) {
				sequenceSize := utf8SequenceSize(input[current])
				valid = append(valid, input[current:current+sequenceSize]...)
				current += sequenceSize
			} else if isPartialUTF8MultiByteSequence(remaining) {
				return valid, append([]byte(nil), remaining...)
			} else {
				current++
				for current < len(input) && isUTF8ContinuationByte(input[current]) {
					current++
				}
			}
		} else {
			current++
		}
	}
	return valid, nil
}

func (p *utf8WideParser) storePartialSequence(input []byte) {
	limit := len(input)
	if limit > utf8ByteSequenceMax {
		limit = utf8ByteSequenceMax
	}
	copy(p.utf8CodePointPieces, input[:limit])
	p.bytesStored = limit
}

// decodeUTF8Sequence is the isolated platform conversion boundary.  For
// complete well-formed UTF-8, its output is the same UTF-16 sequence required
// by MultiByteToWideChar(CP_UTF8).  Non-minimal or non-scalar sequences receive
// the documented replacement treatment of the source's flags=0 conversion.
func decodeUTF8Sequences(input []byte) ([]uint16, error) {
	result := make([]uint16, 0, len(input))
	for i := 0; i < len(input); {
		if input[i] < 0x80 {
			result = append(result, uint16(input[i]))
			i++
			continue
		}
		size := utf8SequenceSize(input[i])
		if size < 2 || size > utf8ByteSequenceMax || i+size > len(input) {
			return nil, fmt.Errorf("UTF-8 decoder received an incomplete source sequence")
		}
		codepoint := rune(input[i] & (0x7f >> size))
		for j := 1; j < size; j++ {
			codepoint = codepoint<<6 | rune(input[i+j]&0x3f)
		}
		minimal := (size == 2 && codepoint >= 0x80) ||
			(size == 3 && codepoint >= 0x800) ||
			(size == 4 && codepoint >= 0x10000)
		if !minimal || codepoint > 0x10ffff || codepoint >= 0xd800 && codepoint <= 0xdfff {
			result = append(result, 0xfffd)
			i += size
			continue
		}
		if codepoint <= 0xffff {
			result = append(result, uint16(codepoint))
		} else {
			codepoint -= 0x10000
			result = append(result, uint16(0xd800+(codepoint>>10)), uint16(0xdc00+(codepoint&0x3ff)))
		}
		i += size
	}
	return result, nil
}

func decodeUTF8SequencesStrict(input []byte) ([]uint16, error) {
	if !isValidUTF8Range(input) {
		return nil, fmt.Errorf("UTF-8 decoder rejected an invalid source sequence")
	}
	return decodeUTF8Sequences(input)
}
