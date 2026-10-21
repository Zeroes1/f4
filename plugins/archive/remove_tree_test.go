package archive

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/zip"
)

// newZipWith writes a zip holding the given entries ("name/" is a directory
// entry) and opens it as an ArchiveVFS.
func newZipWith(t *testing.T, entries ...string) (*ArchiveVFS, string) {
	t.Helper()
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "test.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, name := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(name, "/") {
			if _, err := io.WriteString(w, name); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	v, err := NewArchiveVFS(vfs.NewOSVFS(tmpDir), "test.zip")
	if err != nil {
		t.Fatalf("create ArchiveVFS: %v", err)
	}
	t.Cleanup(func() {
		if err := v.Close(); err != nil {
			t.Errorf("close archive VFS: %v", err)
		}
	})
	return v, zipPath
}

func zipEntryNames(t *testing.T, zipPath string) []string {
	t.Helper()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

// TestArchiveVFS_RemoveDirectoryWithNestedDirectory: a directory made with
// F7 inside another one made with F7 was left in the archive when the outer
// one was deleted, and every later delete said it did not exist (#2107).
func TestArchiveVFS_RemoveDirectoryWithNestedDirectory(t *testing.T) {
	v, zipPath := newZipWith(t, "keep.txt")
	ctx := context.Background()
	outer := v.Join(v.GetPath(), "2del")
	if err := v.MkDir(ctx, outer); err != nil {
		t.Fatalf("create 2del: %v", err)
	}
	if err := v.MkDir(ctx, v.Join(outer, "test")); err != nil {
		t.Fatalf("create 2del/test: %v", err)
	}
	if err := v.Remove(ctx, outer); err != nil {
		t.Fatalf("remove 2del: %v", err)
	}
	if got := strings.Join(zipEntryNames(t, zipPath), " "); got != "keep.txt" {
		t.Fatalf("entries after removing 2del: %q, want only keep.txt", got)
	}
	if err := v.Remove(ctx, outer); err == nil {
		t.Fatal("removing 2del a second time reported no error")
	}
}

// TestArchiveVFS_RemoveImpliedDirectory: a directory that only the names of
// other entries imply has no entry of its own; removing it removes them.
func TestArchiveVFS_RemoveImpliedDirectory(t *testing.T) {
	v, zipPath := newZipWith(t, "keep.txt", "2del/test/a.txt", "2del/b.txt", "2delx.txt")
	if err := v.Remove(context.Background(), v.Join(v.GetPath(), "2del")); err != nil {
		t.Fatalf("remove implied 2del: %v", err)
	}
	if got := strings.Join(zipEntryNames(t, zipPath), " "); got != "keep.txt 2delx.txt" {
		t.Fatalf("entries after removing 2del: %q, want keep.txt and 2delx.txt", got)
	}
}
