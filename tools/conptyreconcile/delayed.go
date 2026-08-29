// Copyright (c) Microsoft Corporation.
// Licensed under the MIT license.
//
// The delay scheduler is probe harness code. It does not define terminal
// semantics; it only varies when the already audited mock receives live bytes
// and resize requests and records the resulting serialized event order.

package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"time"
)

type delayedMockSession struct {
	mu      sync.Mutex
	parser  *vtParser
	capture capture
}

func newDelayedMockSession(width, height int, seed int64) *delayedMockSession {
	return &delayedMockSession{
		parser:  newVTParser(width, height),
		capture: capture{Seed: seed},
	}
}

func (s *delayedMockSession) feedLive(chunk []byte, delay time.Duration) error {
	delayAtBoundary(delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.parser.feed(chunk); err != nil {
		return err
	}
	s.capture.append(streamLive, s.parser.buffer.width, s.parser.buffer.height, chunk, "delayed-live")
	return nil
}

func (s *delayedMockSession) resize(event resizeEvent, delay time.Duration) error {
	delayAtBoundary(delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.parser.resize(event.Width, event.Height); err != nil {
		return err
	}
	s.capture.append(streamResize, event.Width, event.Height, nil, "delayed-resize")
	return nil
}

func (s *delayedMockSession) result() (*vtParser, capture) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.parser, s.capture
}

func (s *delayedMockSession) emitFrame(delay time.Duration) {
	delayAtBoundary(delay)
	s.mu.Lock()
	defer s.mu.Unlock()
	data := frameBytesFromBuffer(s.parser.buffer)
	s.capture.append(streamFrame, s.parser.buffer.width, s.parser.buffer.height, data, "delayed-frame")
}

func (s *delayedMockSession) finishAndEmitFrame() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.parser.finish(); err != nil {
		return err
	}
	data := frameBytesFromBuffer(s.parser.buffer)
	s.capture.append(streamFrame, s.parser.buffer.width, s.parser.buffer.height, data, "delayed-final-frame")
	return nil
}

func delayAtBoundary(delay time.Duration) {
	if delay <= 0 {
		runtime.Gosched()
		return
	}
	time.Sleep(delay)
}

func runDelayedMockScenario(s scenario, delaySeed int64) error {
	_, err := runDelayedMockScenarioWithCapture(s, delaySeed)
	return err
}

func runDelayedMockScenarioWithCapture(s scenario, delaySeed int64) (capture, error) {
	return runDelayedMockScenarioWithCaptureOrdered(s, delaySeed)
}

func runDelayedMockScenarioWithCaptureOrdered(s scenario, delaySeed int64) (capture, error) {
	session := newDelayedMockSession(s.InitialWidth, s.InitialHeight, s.Seed)
	rng := rand.New(rand.NewSource(delaySeed))

	type delayedOperation struct {
		kind        streamKind
		chunk       []byte
		resize      resizeEvent
		beforeDelay time.Duration
		afterDelay  time.Duration
	}
	base := make([]delayedOperation, 0, len(s.Chunks)+len(s.Resizes))
	resizes := sortedResizeEvents(s.Resizes)
	liveIndex, resizeIndex := 0, 0
	for liveIndex < len(s.Chunks) || resizeIndex < len(resizes) {
		chooseResize := liveIndex == len(s.Chunks) || resizeIndex < len(resizes) && rng.Intn(2) == 0
		if chooseResize {
			base = append(base, delayedOperation{kind: streamResize, resize: resizes[resizeIndex]})
			resizeIndex++
			continue
		}
		// Split at arbitrary byte boundaries, including boundaries inside
		// UTF-8, CSI, and OSC sequences.
		for _, part := range splitDelayedChunk(s.Chunks[liveIndex], rng) {
			base = append(base, delayedOperation{kind: streamInput, chunk: part})
		}
		liveIndex++
	}
	operations := make([]delayedOperation, 0, len(base)+len(resizes)+1)
	operations = append(operations, delayedOperation{kind: streamFrame})
	for _, operation := range base {
		operation.beforeDelay = randomDelay(rng)
		operation.afterDelay = randomDelay(rng)
		operations = append(operations, operation)
		if operation.kind == streamResize || rng.Intn(5) == 0 {
			operations = append(operations, delayedOperation{
				kind:        streamFrame,
				beforeDelay: randomDelay(rng),
				afterDelay:  randomDelay(rng),
			})
		}
	}

	// Start all workers up front so the race detector observes the same
	// mutex/queue boundaries as a live producer. Turn gates preserve the
	// already-recorded source event order; random sleeps cannot reorder it.
	type operationResult struct {
		index int
		err   error
	}
	gates := make([]chan struct{}, len(operations))
	results := make(chan operationResult, len(operations))
	var wg sync.WaitGroup
	for index, operation := range operations {
		gate := make(chan struct{})
		gates[index] = gate
		wg.Add(1)
		go func(index int, operation delayedOperation, gate <-chan struct{}) {
			defer wg.Done()
			delayAtBoundary(operation.beforeDelay)
			<-gate
			var err error
			switch operation.kind {
			case streamInput:
				err = session.feedLive(operation.chunk, operation.afterDelay)
			case streamResize:
				err = session.resize(operation.resize, operation.afterDelay)
			case streamFrame:
				session.emitFrame(operation.afterDelay)
			}
			results <- operationResult{index: index, err: err}
		}(index, operation, gate)
	}
	for index := range operations {
		close(gates[index])
		result := <-results
		if result.err != nil {
			for _, gate := range gates[index+1:] {
				close(gate)
			}
			wg.Wait()
			parser, logged := session.result()
			_ = parser
			return logged, fmt.Errorf("seed %d delayed operation %d: %w", s.Seed, result.index, result.err)
		}
	}
	wg.Wait()

	delayAtBoundary(randomDelay(rng))
	if err := session.finishAndEmitFrame(); err != nil {
		_, logged := session.result()
		return logged, fmt.Errorf("seed %d delayed finish: %w", s.Seed, err)
	}
	parser, logged := session.result()
	var input bytes.Buffer
	for _, event := range logged.Events {
		if event.Kind == streamInput {
			input.Write(event.Bytes)
		}
	}
	if !bytes.Equal(input.Bytes(), s.Input) {
		return logged, fmt.Errorf("seed %d delayed scheduler reordered or lost input bytes", s.Seed)
	}
	marker := s.Marker
	if !bytes.Contains([]byte(parser.buffer.text()), []byte(marker)) {
		return logged, fmt.Errorf("seed %d delayed run lost terminal marker %q", s.Seed, marker)
	}
	if parser.buffer.text() != s.ExpectedText {
		return logged, fmt.Errorf("seed %d delayed source text mismatch: got %q want %q", s.Seed, parser.buffer.text(), s.ExpectedText)
	}
	whole, err := parseCapturedFrameEvents(s.InitialWidth, s.InitialHeight, logged.Events, false)
	if err != nil {
		return logged, fmt.Errorf("seed %d delayed whole-output parse: %w", s.Seed, err)
	}
	chunked, err := parseCapturedFrameEvents(s.InitialWidth, s.InitialHeight, logged.Events, true)
	if err != nil {
		return logged, fmt.Errorf("seed %d delayed byte-output parse: %w", s.Seed, err)
	}
	if whole.snapshot() != chunked.snapshot() {
		return logged, fmt.Errorf("seed %d delayed output chunking changed snapshot", s.Seed)
	}
	for i, event := range logged.Events {
		if event.Sequence != uint64(i) {
			return logged, fmt.Errorf("seed %d delayed event sequence gap at %d", s.Seed, i)
		}
	}
	return logged, nil
}

func randomDelay(rng *rand.Rand) time.Duration {
	// The delay is deliberately small: this is a scheduler perturbation, not
	// a timeout-based completion mechanism.
	return time.Duration(rng.Intn(100)) * time.Microsecond
}

func splitDelayedChunk(chunk []byte, rng *rand.Rand) [][]byte {
	if len(chunk) <= 1 {
		return [][]byte{bytes.Clone(chunk)}
	}
	parts := 1 + rng.Intn(minInt(len(chunk), 8))
	boundaries := make(map[int]bool, parts-1)
	for len(boundaries) < parts-1 {
		boundaries[1+rng.Intn(len(chunk)-1)] = true
	}
	result := make([][]byte, 0, parts)
	last := 0
	for boundary := 1; boundary < len(chunk); boundary++ {
		if boundaries[boundary] {
			result = append(result, bytes.Clone(chunk[last:boundary]))
			last = boundary
		}
	}
	result = append(result, bytes.Clone(chunk[last:]))
	return result
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
