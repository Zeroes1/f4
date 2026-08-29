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
	session := newDelayedMockSession(s.InitialWidth, s.InitialHeight, s.Seed)
	rng := rand.New(rand.NewSource(delaySeed))
	type liveJob struct {
		chunk []byte
		delay time.Duration
	}
	liveJobs := make([]liveJob, 0, len(s.Chunks))
	for _, chunk := range s.Chunks {
		// Splitting here deliberately places scheduler boundaries inside the
		// UTF-8 and CSI/OSC bytes already present in the recorded input.
		for _, part := range splitDelayedChunk(chunk, rng) {
			liveJobs = append(liveJobs, liveJob{chunk: part, delay: randomDelay(rng)})
		}
	}
	resizeJobs := make([]struct {
		event resizeEvent
		delay time.Duration
	}, 0, len(s.Resizes))
	for _, event := range sortedResizeEvents(s.Resizes) {
		resizeJobs = append(resizeJobs, struct {
			event resizeEvent
			delay time.Duration
		}{event: event, delay: randomDelay(rng)})
	}
	frameCount := 1 + len(resizeJobs)

	var wg sync.WaitGroup
	errors := make(chan error, 2)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i, job := range liveJobs {
			if err := session.feedLive(job.chunk, job.delay); err != nil {
				errors <- fmt.Errorf("seed %d delayed live chunk %d: %w", s.Seed, i, err)
				return
			}
		}
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i, job := range resizeJobs {
			if err := session.resize(job.event, job.delay); err != nil {
				errors <- fmt.Errorf("seed %d delayed resize %d: %w", s.Seed, i, err)
				return
			}
		}
	}()
	// Frame requests have an independent schedule, so live/resize/frame
	// lock-acquisition order is recorded rather than assumed.
	wg.Add(frameCount)
	for i := 0; i < frameCount; i++ {
		go func(delay time.Duration) {
			defer wg.Done()
			session.emitFrame(delay)
		}(randomDelay(rng))
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		return capture{}, err
	}

	if err := session.finishAndEmitFrame(); err != nil {
		_, logged := session.result()
		return logged, fmt.Errorf("seed %d delayed finish: %w", s.Seed, err)
	}
	parser, logged := session.result()
	var live bytes.Buffer
	for _, event := range logged.Events {
		if event.Kind == streamLive {
			live.Write(event.Bytes)
		}
	}
	if !bytes.Equal(live.Bytes(), s.Input) {
		return logged, fmt.Errorf("seed %d delayed scheduler reordered or lost live bytes", s.Seed)
	}
	marker := "__END_" + fmt.Sprint(s.Seed) + "__"
	if !bytes.Contains([]byte(parser.buffer.text()), []byte(marker)) {
		return logged, fmt.Errorf("seed %d delayed run lost terminal marker %q", s.Seed, marker)
	}
	whole, err := parseCapturedFrameEvents(s.InitialWidth, s.InitialHeight, logged.Events, false)
	if err != nil {
		return logged, fmt.Errorf("seed %d delayed whole-frame parse: %w", s.Seed, err)
	}
	chunked, err := parseCapturedFrameEvents(s.InitialWidth, s.InitialHeight, logged.Events, true)
	if err != nil {
		return logged, fmt.Errorf("seed %d delayed byte-frame parse: %w", s.Seed, err)
	}
	if whole.snapshot() != chunked.snapshot() {
		return logged, fmt.Errorf("seed %d delayed frame chunking changed snapshot", s.Seed)
	}
	if !bytes.Contains([]byte(chunked.buffer.text()), []byte(marker)) {
		return logged, fmt.Errorf("seed %d delayed final frame lost terminal marker %q", s.Seed, marker)
	}
	if err := reconcile(frameFromBuffer(chunked.buffer, "delayed-frame", uint64(s.Seed)), chunked.buffer.logicalRows()); err != nil {
		return logged, fmt.Errorf("seed %d delayed frame reconciliation: %w", s.Seed, err)
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
