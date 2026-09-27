//go:build !lite

package gui

// BackendBuilt reports whether this binary carries the named GUI backend.
// The regular build carries every backend vtui has; backends_lite.go lists
// the lite build's.
func BackendBuilt(backend string) bool {
	return true
}
