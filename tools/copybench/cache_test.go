package main

import (
	"strings"
	"testing"
)

func TestResidentShare(t *testing.T) {
	for _, tc := range []struct {
		resident, total int
		want            float64
	}{
		{0, 0, 0},
		{5, 0, 0},
		{0, 100, 0},
		{25, 100, 0.25},
		{100, 100, 1},
	} {
		if got := residentShare(tc.resident, tc.total); got != tc.want {
			t.Errorf("residentShare(%d, %d) = %v, want %v", tc.resident, tc.total, got, tc.want)
		}
	}
}

func TestResidencyWarning(t *testing.T) {
	for _, tc := range []struct {
		name            string
		resident, total int
		want            string
	}{
		{"nothing to check", 0, 0, ""},
		{"dropped", 0, 2048, ""},
		{"a few pages stay", 5, 100, ""},
		{"exactly at the limit", 10, 100, ""},
		{"just over the limit", 11, 100, "  warning: 11% of the source pages are still in memory after the cache drop " +
			"(this file system ignores posix_fadvise DONTNEED?); the MB/s above measure memory, not the disk"},
		{"ignored", 87, 100, "  warning: 87% of the source pages are still in memory after the cache drop " +
			"(this file system ignores posix_fadvise DONTNEED?); the MB/s above measure memory, not the disk"},
		{"all of it", 2048, 2048, "  warning: 100% of the source pages are still in memory after the cache drop " +
			"(this file system ignores posix_fadvise DONTNEED?); the MB/s above measure memory, not the disk"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := residencyWarning(tc.resident, tc.total); got != tc.want {
				t.Errorf("residencyWarning(%d, %d) = %q, want %q", tc.resident, tc.total, got, tc.want)
			}
		})
	}
}

func TestResidencyWarningIsOneLine(t *testing.T) {
	if got := residencyWarning(1, 1); strings.Contains(got, "\n") {
		t.Errorf("the warning must be one line, got %q", got)
	}
}
