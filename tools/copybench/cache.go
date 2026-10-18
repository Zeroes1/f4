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
