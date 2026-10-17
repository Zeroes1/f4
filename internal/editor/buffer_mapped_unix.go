//go:build !windows

package editor

import "golang.org/x/sys/unix"

// mapFileRegion maps the whole file read-only and private: the editor never
// writes through the mapping, and a private mapping keeps the process from
// being able to modify the file by accident even if it tried.
func mapFileRegion(fd uintptr, size int64) ([]byte, error) {
	return unix.Mmap(int(fd), 0, int(size), unix.PROT_READ, unix.MAP_PRIVATE)
}

func unmapFileRegion(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	return unix.Munmap(data)
}

// releaseMappingBeforeReplace: a Unix mapping does not stop the file from
// being renamed over or rewritten, so a save can keep it until the reopened
// file replaces it. A variable so the tests can exercise the Windows path.
var releaseMappingBeforeReplace = false
