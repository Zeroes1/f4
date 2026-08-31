package main

import (
	"fmt"
	"os"
	"time"
)

// emitProbeWorkload is deliberately a dumb child: every byte below is
// authored by the probe, while repaint bytes, chunking, cursor movement and
// resize effects are produced by the pinned OpenConsole process.  Keeping the
// payload here makes the expected logical markers independent of any parser
// implementation that will eventually consume the captured stream.
func emitProbeWorkload() error {
	return emitProbeWorkloadWidth(80)
}

func emitProbeWorkloadWidth(width int) error {
	input := []byte(probeWorkloadForWidth(width))
	for offset := 0; offset < len(input); {
		end := offset + 1
		if end < len(input) {
			end += (offset * 17) % 31
			if end > len(input) {
				end = len(input)
			}
		}
		if _, err := os.Stdout.Write(input[offset:end]); err != nil {
			return fmt.Errorf("emit native probe workload: %w", err)
		}
		offset = end
		time.Sleep(time.Duration((offset*13)%400) * time.Microsecond)
	}
	return nil
}

func emitSeedWorkload(seed uint64, width int) error {
	input := seedWorkload(seed, width)
	for offset := 0; offset < len(input); {
		end := offset + 1 + int(seed%31)
		if end > len(input) {
			end = len(input)
		}
		if _, err := os.Stdout.Write(input[offset:end]); err != nil {
			return fmt.Errorf("emit seed %016x: %w", seed, err)
		}
		offset = end
		seed = seed*6364136223846793005 + 1
		time.Sleep(time.Duration(seed%200) * time.Microsecond)
	}
	return nil
}

func emitPartialWorkload(width int) error {
	input := partialProbeWorkload(width)
	cut := len(input) / 2
	if _, err := os.Stdout.Write(input[:cut]); err != nil {
		return fmt.Errorf("emit partial probe prefix: %w", err)
	}
	time.Sleep(100 * time.Millisecond)
	if _, err := os.Stdout.Write(input[cut:]); err != nil {
		return fmt.Errorf("emit partial probe suffix: %w", err)
	}
	return nil
}
