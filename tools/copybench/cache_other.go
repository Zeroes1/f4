//go:build !linux

package main

// dropCache does nothing here: the files the benchmark has just written may
// still be in memory, and the first runs may read them from there.
func dropCache([]string) error { return nil }

// cacheResidency has nothing to look at here: 0 of 0 pages means no check, and
// no warning (the cache is not dropped at all, which cacheNote says).
func cacheResidency([]string) (resident, total int, err error) { return 0, 0, nil }

// onFUSE is not looked at here: the cache is not dropped at all (cacheNote).
func onFUSE(string) bool { return false }

const cacheNote = "source cache: NOT dropped on this system; the source files may be read from memory, so -src numbers can be far above the disk"
