package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A file that has just been written and read back is in the page cache. How
// much of it dropCache takes out depends on the file system (f4#1894: ntfs
// ignores it), so only the part that cannot depend on it is checked here.
func TestResidentPagesOfAFileJustWritten(t *testing.T) {
	const size = 1 << 20
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(path); err != nil {
		t.Fatal(err)
	}
	resident, total, err := residentPages(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := size / os.Getpagesize(); total != want {
		t.Errorf("total pages = %d, want %d", total, want)
	}
	if resident == 0 || resident > total {
		t.Errorf("resident pages = %d of %d, want some of them: the file was just written and read", resident, total)
	}
}

func TestResidentPagesCountsAPartialLastPage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(path, make([]byte, 1), 0o600); err != nil {
		t.Fatal(err)
	}
	_, total, err := residentPages(path)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Errorf("total pages of a 1 byte file = %d, want 1", total)
	}
}

// mmap of length 0 fails with EINVAL, an empty file must not break the check.
func TestResidentPagesOfAnEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	resident, total, err := residentPages(path)
	if err != nil {
		t.Fatalf("an empty file: %v", err)
	}
	if resident != 0 || total != 0 {
		t.Errorf("an empty file has %d of %d pages, want 0 of 0", resident, total)
	}
}

func TestResidentPagesOfAMissingFile(t *testing.T) {
	if _, _, err := residentPages(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("a missing file: want an error")
	}
}

func TestCacheResidencySumsTheFiles(t *testing.T) {
	dir := t.TempDir()
	var files []string
	for i, size := range []int{0, 3 * os.Getpagesize(), os.Getpagesize() + 1} {
		path := filepath.Join(dir, string(rune('a'+i)))
		if err := os.WriteFile(path, make([]byte, size), 0o600); err != nil {
			t.Fatal(err)
		}
		files = append(files, path)
	}
	resident, total, err := cacheResidency(files)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Errorf("total pages = %d, want 5 (0 + 3 + 2)", total)
	}
	if resident == 0 || resident > total {
		t.Errorf("resident pages = %d of %d, want some of them", resident, total)
	}
	if _, _, err := cacheResidency(append(files, filepath.Join(dir, "missing"))); err == nil {
		t.Error("a missing file in the list: want an error")
	}
}
