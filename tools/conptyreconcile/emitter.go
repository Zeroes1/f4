package main

import (
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"time"
)

// emitScenario is the attached-client entry point used by the pinned host
// gate. It writes the recorded input itself; the host, not a fixture, must
// render and return that stream through ConPTY.
func emitScenario(seedText, widthText string) error {
	var input []byte
	var seed int64
	if widthText != "" {
		width, err := strconv.Atoi(widthText)
		if err != nil || width < 1 {
			return fmt.Errorf("invalid emit width %q", widthText)
		}
		s := edgeScenario(width)
		input = s.Input
		seed = s.Seed
	} else {
		value, err := strconv.ParseUint(seedText, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid emit seed %q: %w", seedText, err)
		}
		seed = int64(value)
		input = scenarioForSeed(seed).Input
	}

	rng := rand.New(rand.NewSource(seed ^ 0x6a09e667f3bcc909))
	for offset := 0; offset < len(input); {
		remaining := len(input) - offset
		chunkSize := 1 + rng.Intn(minInt(64, remaining))
		if _, err := os.Stdout.Write(input[offset : offset+chunkSize]); err != nil {
			return fmt.Errorf("emit seed %d: %w", seed, err)
		}
		offset += chunkSize
		// This delay is part of the child workload, and is independently
		// seeded so it cannot become a completion heuristic in the parent.
		if delay := rng.Intn(200); delay != 0 {
			time.Sleep(time.Duration(delay) * time.Microsecond)
		}
	}
	return nil
}
