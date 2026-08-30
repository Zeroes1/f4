// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// Scenario generation is test input only.  It supplies the recorded payload
// and seed independently of the parser/model; it does not encode a host
// behavior or a guessed frame grammar.

package main

import (
	"fmt"
	"math/rand"
	"strings"
	"unicode/utf8"
)

var scenarioCorpus = []string{
	"ASCII",
	"CJK漢字かなカナ",
	"ambiguous ДЖΩ·※→",
	"combining e\u0301 a\u0328",
	"variation ☕️ ✈︎",
	"surrogate 😀👾🧭",
	"cluster 👩‍💻 🏳️‍🌈 1️⃣",
	"bidi אבג العربية 123 abc",
	"identical ++++++++",
	"blank-adjacent",
}

func scenarioForSeed(seed int64) scenario {
	rng := rand.New(rand.NewSource(seed))
	widthChoices := []int{1, 2, 7, 8, 15, 16, 31, 32, 39, 40, 79, 80, 119, 120, 121}
	width := widthChoices[rng.Intn(len(widthChoices))]
	height := 256
	lineCount := 8 + rng.Intn(16)
	lines := make([]string, 0, lineCount)
	for i := 0; i < lineCount; i++ {
		var line strings.Builder
		target := width - 1 + rng.Intn(width*2+3)
		if i%5 == 1 {
			target = width
		} else if i%5 == 2 {
			target = width + 1
		} else if i%5 == 3 {
			target = width * (1 + rng.Intn(3))
		}
		for utf8.RuneCountInString(line.String()) < target {
			line.WriteString(scenarioCorpus[rng.Intn(len(scenarioCorpus))])
		}
		text := line.String()
		if utf8.RuneCountInString(text) > target+8 {
			runes := []rune(text)
			text = string(runes[:target])
		}
		lines = append(lines, text)
	}

	var input strings.Builder
	input.WriteString("\x1b[2J\x1b[H")
	input.WriteString("\x1b]0;conptyreconcile-")
	input.WriteString(fmt.Sprint(seed))
	input.WriteString("\x07")
	input.WriteString("\x1b[31m")
	for i, line := range lines {
		input.WriteString(line)
		if i != len(lines)-1 {
			input.WriteString("\r\n")
		}
	}
	input.WriteString("\x1b[0m\r\n__END_")
	input.WriteString(fmt.Sprint(seed))
	input.WriteString("__\r\n")

	data := []byte(input.String())
	cutCount := 1 + rng.Intn(32)
	boundaries := make([]int, 0, cutCount)
	for i := 0; i < cutCount; i++ {
		boundaries = append(boundaries, rng.Intn(len(data)))
	}
	resizes := []resizeEvent{
		{Width: maxInt(1, width-1), Height: height, Order: 0},
		{Width: width, Height: height, Order: 1},
		{Width: width + 1, Height: height, Order: 2},
		{Width: 1, Height: height, Order: 3},
		{Width: width * 2, Height: height, Order: 4},
		{Width: width, Height: height, Order: 5},
	}
	for i := range resizes {
		if i > 0 && rng.Intn(2) == 0 {
			resizes[i].Order = i
		}
	}

	return scenario{
		Seed:          seed,
		InitialWidth:  width,
		InitialHeight: height,
		Input:         data,
		ExpectedText:  strings.Join(append(lines, "__END_"+fmt.Sprint(seed)+"__"), "\n"),
		Marker:        "__END_" + fmt.Sprint(seed) + "__",
		Chunks:        splitAtBoundaries(data, boundaries),
		Resizes:       resizes,
		Command:       "cmd.exe /d /c dir /s C:\\Windows\\System32",
	}
}

func edgeScenario(width int) scenario {
	if width < 1 {
		panic("edge scenario width must be positive")
	}
	lines := []string{
		"",
		strings.Repeat("A", maxInt(0, width-1)),
		strings.Repeat("B", width),
		strings.Repeat("C", width+1),
		"漢字 e\u0301 ☕️ 😀 👩‍💻 אבג 123",
		"same same same",
		"",
	}
	marker := "__EDGE_END_" + fmt.Sprint(width) + "__"
	var input strings.Builder
	input.WriteString("\x1b[2J\x1b[H\x1b]0;edge-")
	input.WriteString(fmt.Sprint(width))
	input.WriteString("\x07\x1b[31m")
	for _, line := range lines {
		input.WriteString(line)
		input.WriteString("\r\n")
	}
	input.WriteString("\x1b[0m\x1b[2;1Htab\x1b[3Gmove\x1b[K\r\n")
	input.WriteString(marker)
	input.WriteString("\r\n")
	data := []byte(input.String())
	return scenario{
		Seed:          int64(width),
		InitialWidth:  width,
		InitialHeight: 256,
		Input:         data,
		// ESC[3G addresses the third column (one-based), so "move"
		// overwrites the final character of "tab" and leaves "tamove".
		ExpectedText: strings.Join(append(lines, "tamove", marker), "\n"),
		Marker:       marker,
		Chunks:       allByteChunks(data),
		Resizes: []resizeEvent{
			{Width: maxInt(1, width-1), Height: 256, Order: 0},
			{Width: width, Height: 256, Order: 1},
			{Width: width + 1, Height: 256, Order: 2},
			{Width: 1, Height: 256, Order: 3},
			{Width: width * 2, Height: 256, Order: 4},
			{Width: width, Height: 256, Order: 5},
		},
		Command: "cmd.exe /d /c dir /s C:\\Windows\\System32",
	}
}

func edgeScenarioWidths() []int {
	return []int{1, 2, 7, 8, 15, 16, 31, 32, 39, 40, 79, 80, 119, 120, 121}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func recordedLogicalLines(text string) []logicalRow {
	parts := strings.Split(text, "\n")
	result := make([]logicalRow, len(parts))
	for i, part := range parts {
		result[i] = logicalRow{units: utf16Units(part), rows: []int{i}}
	}
	return result
}

func runMockScenario(s scenario) error {
	_, err := runMockScenarioWithCapture(s)
	return err
}

func runMockScenarioWithCapture(s scenario) (capture, error) {
	var empty capture
	whole, err := parseWithChunks(s.InitialWidth, s.InitialHeight, [][]byte{s.Input})
	if err != nil {
		return empty, fmt.Errorf("seed %d whole parse: %w", s.Seed, err)
	}
	chunked, err := parseWithChunks(s.InitialWidth, s.InitialHeight, s.Chunks)
	if err != nil {
		return empty, fmt.Errorf("seed %d chunked parse: %w", s.Seed, err)
	}
	if whole.snapshot() != chunked.snapshot() {
		return empty, fmt.Errorf("seed %d chunking changed snapshot", s.Seed)
	}

	for _, resize := range sortedResizeEvents(s.Resizes) {
		if err := chunked.resize(resize.Width, resize.Height); err != nil {
			return empty, fmt.Errorf("seed %d resize %dx%d: %w", s.Seed, resize.Width, resize.Height, err)
		}
	}
	if got, want := expectedPrintableText(s.Input), strings.ReplaceAll(s.ExpectedText, "\n", ""); got != want {
		return empty, fmt.Errorf("seed %d generated printable invariant changed: got %q want %q", s.Seed, got, want)
	}
	text := chunked.buffer.text()
	marker := s.Marker
	if !strings.Contains(text, marker) {
		return empty, fmt.Errorf("seed %d lost end marker %q after the recorded resize sequence", s.Seed, marker)
	}
	if text != s.ExpectedText {
		return empty, fmt.Errorf("seed %d source text mismatch after resize: got %q want %q", s.Seed, text, s.ExpectedText)
	}

	if count := strings.Count(chunked.buffer.text(), marker); count != 1 {
		return empty, fmt.Errorf("seed %d marker multiplicity is %d, want exactly one", s.Seed, count)
	}

	// Drive one additional session with live chunks and resize requests
	// interleaved. The event order is seeded and is recorded, while the frame
	// bytes are produced by frameEmitter from the same pinned buffer state.
	interleaved := newVTParser(s.InitialWidth, s.InitialHeight)
	type scheduledEvent struct {
		kind   streamKind
		chunk  []byte
		resize resizeEvent
	}
	rng := rand.New(rand.NewSource(s.Seed ^ 0x243f6a8885a308d3))
	events := make([]scheduledEvent, 0, len(s.Chunks)+len(s.Resizes))
	liveIndex, resizeIndex := 0, 0
	resizes := sortedResizeEvents(s.Resizes)
	for liveIndex < len(s.Chunks) || resizeIndex < len(resizes) {
		chooseResize := liveIndex == len(s.Chunks) || resizeIndex < len(resizes) && rng.Intn(2) == 0
		if chooseResize {
			events = append(events, scheduledEvent{kind: streamResize, resize: resizes[resizeIndex]})
			resizeIndex++
		} else {
			events = append(events, scheduledEvent{kind: streamLive, chunk: s.Chunks[liveIndex]})
			liveIndex++
		}
	}
	logged := capture{Seed: s.Seed}
	for _, event := range events {
		switch event.kind {
		case streamLive:
			if err := interleaved.feed(event.chunk); err != nil {
				return logged, fmt.Errorf("seed %d interleaved live feed: %w", s.Seed, err)
			}
			logged.append(streamLive, interleaved.buffer.width, interleaved.buffer.height, event.chunk, "mock-live")
		case streamResize:
			if err := interleaved.resize(event.resize.Width, event.resize.Height); err != nil {
				return logged, fmt.Errorf("seed %d interleaved resize %dx%d: %w", s.Seed, event.resize.Width, event.resize.Height, err)
			}
			logged.append(streamResize, event.resize.Width, event.resize.Height, nil, "mock-resize")
			frameBytes, frameErr := frameBytesFromBuffer(interleaved.buffer)
			if frameErr != nil {
				return logged, fmt.Errorf("seed %d frame emission: %w", s.Seed, frameErr)
			}
			logged.append(streamFrame, interleaved.buffer.width, interleaved.buffer.height, frameBytes, "mock-frame")
		}
	}
	if err := interleaved.finish(); err != nil {
		return logged, fmt.Errorf("seed %d interleaved finish: %w", s.Seed, err)
	}
	if count := strings.Count(interleaved.buffer.text(), marker); count != 1 {
		return logged, fmt.Errorf("seed %d interleaved marker multiplicity is %d, want exactly one", s.Seed, count)
	}
	rendered := frameFromBuffer(interleaved.buffer, "mock-frame-final", uint64(len(logged.Events)))
	if err := reconcileParserState(rendered, interleaved); err != nil {
		return logged, fmt.Errorf("seed %d frame/live reconciliation: %w", s.Seed, err)
	}
	s.Frame = rendered
	return logged, nil
}
