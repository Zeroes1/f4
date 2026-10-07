package panel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// indexOfRow returns the cursor index for name, or -1 when the panel does not
// show it.
func indexOfRow(fp *FileSystemPanel, name string) int {
	for i, entry := range fp.Entries {
		if entry.Name == name {
			return i
		}
	}
	return -1
}

// TestReloadSameDirectoryKeepsRowsAndCursor is the regression test for the
// cursor jump seen on F8/Del and Shift+F5: with SyncPanelLoad on, re-reading
// the directory the panel already shows replaced the rows with an empty ".."
// skeleton and parked the cursor on it, then the completion task moved the
// cursor to the successor file. The reload must keep the live rows and the
// cursor until the fresh listing replaces them atomically.
func TestReloadSameDirectoryKeepsRowsAndCursor(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.SyncPanelLoad = true

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	dir := t.TempDir()
	v := vfs.NewOSVFS(dir)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		// #nosec G304 -- the path is inside the private test temp directory.
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	fp := NewFileSystemPanel(0, 0, 80, 25, v)
	t.Cleanup(func() {
		fp.cancelProviderOpen()
		if fp.Vfs != nil {
			_ = fp.Vfs.Close()
		}
		if fp.CancelLoad != nil {
			fp.CancelLoad()
		}
		fp.StopLoadingAnimation()
	})
	waitForLoad(t, fp)

	if len(fp.Entries) < 4 {
		t.Fatalf("panel rows = %v, want \"..\" and three files", panelNames(fp))
	}
	bRow := indexOfRow(fp, "b.txt")
	if bRow < 0 {
		t.Fatalf("panel rows = %v, no b.txt to put the cursor on", panelNames(fp))
	}
	fp.SetCursorIndex(bRow)

	// a.txt vanishes on disk; the refresh after a delete re-reads the same
	// directory with the successor as the pending target.
	// #nosec G304 -- the path is inside the private test temp directory.
	if err := os.Remove(filepath.Join(dir, "a.txt")); err != nil {
		t.Fatal(err)
	}
	fp.PendingSelection = "c.txt"
	fp.readDirectoryEx(false)

	if name := fp.GetRawSelectedName(); name != "b.txt" {
		t.Errorf("cursor right after the reload = %q, want b.txt; rows %v mean the panel was reset to a \"..\" skeleton",
			name, panelNames(fp))
	}
	waitForLoad(t, fp)

	if indexOfRow(fp, "a.txt") >= 0 {
		t.Errorf("deleted a.txt is still on screen after the reload finished: %v", panelNames(fp))
	}
	if name := fp.GetRawSelectedName(); name != "c.txt" {
		t.Errorf("cursor after the reload = %q, want successor c.txt; rows %v", name, panelNames(fp))
	}
}

// TestReloadOtherDirectoryShowsSkeleton is the other half of the fix: a panel
// that really navigates away must not keep the old directory's rows on screen
// while the new one loads.
func TestReloadOtherDirectoryShowsSkeleton(t *testing.T) {
	oldCfg := config.App
	defer func() { config.App = oldCfg }()
	config.App.SyncPanelLoad = true

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	first := t.TempDir()
	second := t.TempDir()
	// #nosec G304 -- the path is inside the private test temp directory.
	if err := os.WriteFile(filepath.Join(first, "stale.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	// #nosec G304 -- the path is inside the private test temp directory.
	if err := os.WriteFile(filepath.Join(second, "fresh.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	fp := NewFileSystemPanel(0, 0, 80, 25, vfs.NewOSVFS(first))
	t.Cleanup(func() {
		fp.cancelProviderOpen()
		if fp.CancelLoad != nil {
			fp.CancelLoad()
		}
		fp.StopLoadingAnimation()
	})
	waitForLoad(t, fp)
	if indexOfRow(fp, "stale.txt") < 0 {
		t.Fatalf("panel rows = %v, want the first directory loaded", panelNames(fp))
	}

	staleVFS := fp.Vfs
	fp.Vfs = vfs.NewOSVFS(second)
	fp.readDirectoryEx(false)
	if rows := panelNames(fp); len(rows) != 1 || rows[0] != ".." {
		t.Errorf("rows right after switching directory = %v, want only the \"..\" skeleton", rows)
	}

	waitForLoad(t, fp)
	_ = staleVFS.Close()

	if indexOfRow(fp, "fresh.txt") < 0 {
		t.Errorf("second directory did not load: %v", panelNames(fp))
	}
	if indexOfRow(fp, "stale.txt") >= 0 {
		t.Errorf("first directory's rows survived navigation: %v", panelNames(fp))
	}
}
