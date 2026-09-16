package editor

import "errors"

// wazero's amd64 compiler emits POPCNT for the scalar popcnt instructions in
// Colorer WASM. Keep the whole Colorer path out of that compiler on CPUs where
// the instruction is unavailable; callers already fall back to Chroma when a
// Colorer session cannot be created.
var errColorerUnsupportedCPU = errors.New("Colorer requires CPU POPCNT support")

func colorerRuntimeCheck() error {
	return colorerRuntimeCheckForCPU(colorerCPUSupportsPOPCNT())
}

func colorerRuntimeCheckForCPU(hasPOPCNT bool) error {
	if hasPOPCNT {
		return nil
	}
	return errColorerUnsupportedCPU
}
