package main

import "fmt"

// warnShare is the share of the source pages that may stay in memory after the
// cache drop before the report says its numbers measure the memory. Some pages
// always stay (a file system keeps its own metadata around, a page can be in
// use), so a few percent is not a reason to doubt the numbers.
const warnShare = 0.10

// residentShare is the part of total that is resident, 0 for nothing to check.
func residentShare(resident, total int) float64 {
	if total <= 0 {
		return 0
	}
	return float64(resident) / float64(total)
}

// residencyWarning is the line printed after a run when too many pages of the
// source files are still in memory after the cache drop, and "" when the drop
// worked (or there was nothing to check).
//
// The reason is f4#1894: on an ntfs disk (nz256, /media/nz/data1) the numbers
// were 1-2.5 GB/s and grew from run to run, the speed of the memory, and the
// report said nothing, because posix_fadvise DONTNEED returns success on a file
// system that ignores it. Looking at what is actually in memory afterwards is
// the only way to tell.
func residencyWarning(resident, total int) string {
	share := residentShare(resident, total)
	if share <= warnShare {
		return ""
	}
	return fmt.Sprintf("  warning: %.0f%% of the source pages are still in memory after the cache drop "+
		"(this file system ignores posix_fadvise DONTNEED?); the MB/s above measure memory, not the disk", share*100)
}

// fuseWarning is the line printed under the table when the source is on a
// FUSE file system, and "" otherwise. ntfs-3g and the like read the partition
// through the cache of the block device, under the files: the drop of the
// file pages does not reach it and mincore sees nothing left, while the data
// comes from memory (nz256, f4#1894: /media/nz/data1, fuseblk on /dev/sdb3,
// 3500 small files/s and 1-2.4 GB/s from a hard disk).
func fuseWarning(onFUSE bool) string {
	if !onFUSE {
		return ""
	}
	return "warning: the source is on a FUSE file system (ntfs-3g and the like): it reads the disk through a cache " +
		"the drop above does not reach, so the MB/s above may measure memory, not the disk; to measure the disk, " +
		"mount it with a kernel driver (for ntfs: -t ntfs3)"
}
