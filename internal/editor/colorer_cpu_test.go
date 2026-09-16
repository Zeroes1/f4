package editor

import "testing"

func TestColorerRuntimeCheckForCPU(t *testing.T) {
	if err := colorerRuntimeCheckForCPU(true); err != nil {
		t.Fatalf("CPU with POPCNT was rejected: %v", err)
	}
	if err := colorerRuntimeCheckForCPU(false); err != errColorerUnsupportedCPU {
		t.Fatalf("CPU without POPCNT got %v, want %v", err, errColorerUnsupportedCPU)
	}
}
