package main

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/unix"
)

// dropCache takes the files out of the page cache, so the next copy reads
// them from the disk. Files the benchmark has just written, or copied a run
// before, are in memory, and reading them there measured the memory (nz256,
// f4#1894: 1843 MB/s from an SD card in a USB reader). The data is flushed
// first: the kernel keeps pages that are not written back yet.
func dropCache(files []string) error {
	for _, path := range files {
		f, err := os.Open(path) // #nosec G304 -- a file the benchmark created.
		if err != nil {
			return err
		}
		err = f.Sync()
		if err == nil {
			err = unix.Fadvise(int(f.Fd()), 0, 0, unix.FADV_DONTNEED)
		}
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// cacheResidency counts the pages of the files that are in memory right now
// and all their pages. It is asked after dropCache: DONTNEED is only advice,
// a file system may ignore it and still answer success (nz256, f4#1894: an
// ntfs disk gave 1-2.5 GB/s, the memory), so the drop is checked by what is
// really resident, not by the error code.
func cacheResidency(files []string) (resident, total int, err error) {
	for _, path := range files {
		r, t, perr := residentPages(path)
		if perr != nil {
			return 0, 0, fmt.Errorf("%s: %w", path, perr)
		}
		resident += r
		total += t
	}
	return resident, total, nil
}

// residentPages maps the file and asks mincore(2) which of its pages are in
// the page cache. Mapping does not read anything, and mincore does not bring
// pages in. An empty file has no pages and cannot be mapped (mmap of length 0
// is EINVAL), so it counts as 0 of 0.
func residentPages(path string) (resident, total int, err error) {
	f, err := os.Open(path) // #nosec G304 -- a file the benchmark created.
	if err != nil {
		return 0, 0, err
	}
	defer func() { _ = f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return 0, 0, err
	}
	size := info.Size()
	if size == 0 {
		return 0, 0, nil
	}
	data, err := unix.Mmap(int(f.Fd()), 0, int(size), unix.PROT_READ, unix.MAP_SHARED) //nolint:gosec // G115: size of a file that fits in the address space, or mmap fails
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if uerr := unix.Munmap(data); err == nil {
			err = uerr
		}
	}()

	page := int64(os.Getpagesize())
	vec := make([]byte, (size+page-1)/page)
	// golang.org/x/sys/unix has no Mincore wrapper (v0.48.0), so the system call
	// is made directly: mincore(addr, length, vec), one byte per page, bit 0 set
	// when the page is resident.
	//nolint:gosec // G103: the addresses of the mapping and of vec, both alive during the call.
	_, _, errno := unix.Syscall(unix.SYS_MINCORE,
		uintptr(unsafe.Pointer(&data[0])), uintptr(len(data)), uintptr(unsafe.Pointer(&vec[0])))
	if errno != 0 {
		return 0, 0, errno
	}
	for _, b := range vec {
		if b&1 != 0 {
			resident++
		}
	}
	return resident, len(vec), nil
}

const cacheNote = "source cache: dropped before each run (posix_fadvise DONTNEED), then checked with mincore"
