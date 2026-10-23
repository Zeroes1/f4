//go:build windows

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

func TestViewOnADirectoryDoesNotFollowJunction(t *testing.T) {
	pf, fsp, entries := dirSizeFixture(t)
	dir := filepath.Join(fsp.Vfs.GetPath(), "b")
	payload := make([]byte, 1<<20)
	if err := os.WriteFile(filepath.Join(dir, "target.bin"), payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fsp.Vfs.(*vfs.OSVFS).Junction(context.Background(), dir, filepath.Join(dir, "loop")); err != nil {
		t.Fatalf("create recursive junction: %v", err)
	}

	fsp.SetCursorIndex(2) // b
	actionViewFile(pf)
	pumpSizeTasksUntil(t, func() bool { return entries["b"].SizeCalculated })

	got := entries["b"].Size
	if got < int64(len(payload)) || got >= 2*int64(len(payload)) {
		t.Errorf("F3 size = %d, want the directory's file but not a second traversal through its junction", got)
	}
}
