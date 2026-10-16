//go:build !windows

package vfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Removing the last permission of a file -- -r-------- to ---------- -- is
// a mode of 000, which SetAttributes used to take for "no mode" and skip
// (f4#1981: the attributes dialog left the r in place).
func TestOSVFSSetAttributes_ModeZeroIsApplied(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, nil, 0o400); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	v := NewOSVFS(dir)
	item, err := v.Stat(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	item.UnixMode = 0
	if err := v.SetAttributes(context.Background(), path, item); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0 {
		t.Fatalf("mode after removing the last permission = %o, want 000", perm)
	}
}

// An item that does not carry its permissions still leaves the mode alone:
// a zero there means "not known", not "no permissions".
func TestOSVFSSetAttributes_UnknownModeIsKept(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, nil, 0o640); err != nil {
		t.Fatal(err)
	}
	v := NewOSVFS(dir)
	item := VFSItem{Name: "test.txt", Uid: -1, Gid: -1, KnownMetadata: MetadataExplicit}
	if err := v.SetAttributes(context.Background(), path, item); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o640 {
		t.Fatalf("mode = %o, want the file's own 640", perm)
	}
}
