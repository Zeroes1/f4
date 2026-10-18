//go:build windows

package testutil

// openFiles is empty on Windows: the process-wide handle count mixes files
// with threads, events and mutexes the runtime creates as it goes, so a
// before/after comparison would say nothing about files.
func openFiles() []openFile { return nil }
