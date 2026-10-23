package panel

import (
	"path/filepath"
	"testing"
)

// The numbered bookmarks, which RCtrl/Ctrl+Alt and a digit reach.
func TestBookmarkSlotsSaveAndGo(t *testing.T) {
	pf, fsp := livePanels(t)
	saved := fsp.Vfs.GetPath()
	if !pf.SaveBookmarkSlot(4) {
		t.Fatal("slot 4 was not saved")
	}
	set, err := LoadBookmarks(BookmarksFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if set[4].Path != saved {
		t.Fatalf("slot 4 holds %q, want %q", set[4].Path, saved)
	}
	elsewhere, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pf.NavigateToPath(fsp, elsewhere)
	waitForLoad(t, fsp)
	if !pf.GoToBookmarkSlot(4) {
		t.Fatal("slot 4 was not followed")
	}
	waitForLoad(t, fsp)
	if got := fsp.Vfs.GetPath(); got != saved {
		t.Errorf("the panel is in %q after going to slot 4, want %q", got, saved)
	}
	if !pf.GoToBookmarkSlot(7) || fsp.Vfs.GetPath() != saved {
		t.Error("an empty slot moved the panel")
	}
	if pf.GoToBookmarkSlot(10) || pf.SaveBookmarkSlot(-1) {
		t.Error("a slot outside 0-9 was taken")
	}
}
