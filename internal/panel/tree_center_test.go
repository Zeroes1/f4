package panel

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
)

// centeredTop is where the view of a tree of n rows, vh of them on screen,
// starts when row sel is put in its middle.
// keepTreeExpandCache puts the remembered expanded branches back when the
// test ends: branches it expands under its TempDir would otherwise be
// re-expanded by every tree a later test opens.
func keepTreeExpandCache(t *testing.T) {
	treeExpandCacheMu.Lock()
	saved := make(map[string]struct{}, len(treeExpandCache))
	for k := range treeExpandCache {
		saved[k] = struct{}{}
	}
	treeExpandCacheMu.Unlock()
	t.Cleanup(func() {
		treeExpandCacheMu.Lock()
		treeExpandCache = saved
		treeExpandCacheMu.Unlock()
	})
}

func centeredTop(sel, vh, n int) int {
	return min(max(sel-vh/2, 0), max(n-vh, 0))
}

// mitek1977-coder, f4#1602: with "Tree like Explorer's" on, a tree that does
// not fit opens with the folder of the panel in the middle of the view, and
// Tab over to the tree puts the cursor on the folder the panel shows by then.
func TestExplorerTreeCentersThePanelFolder(t *testing.T) {
	keepTreeExpandCache(t)
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.TreeExplorerMode = true
	config.App.TreeRootWholeVolume = true

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("d%02d", i)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	start := filepath.Join(root, "d30")
	fsp := NewFileSystemPanel(0, 0, 40, 14, vfs.NewOSVFS(start))
	waitForLoad(t, fsp)
	tp := NewTreePanel(fsp)

	vh, n := tp.Table.ViewHeight, tp.Table.ItemCount
	if n <= vh {
		t.Fatalf("setup: %d rows fit in %d", n, vh)
	}
	if got := tp.SelectedPath(); got != start {
		t.Fatalf("the cursor is on %q, want the panel's %q", got, start)
	}
	if want := centeredTop(tp.Table.SelectPos, vh, n); tp.Table.TopPos != want {
		t.Errorf("opened with the view at %d (cursor %d, %d rows on screen), want %d", tp.Table.TopPos, tp.Table.SelectPos, vh, want)
	}

	// The user goes elsewhere in the panel, then Tabs over to the tree.
	other := filepath.Join(root, "d05")
	if err := fsp.Vfs.SetPath(other); err != nil {
		t.Fatal(err)
	}
	tp.SetFocus(true)
	if got := tp.SelectedPath(); got != other {
		t.Fatalf("Tab: the cursor is on %q, want the panel's %q", got, other)
	}
	if want := centeredTop(tp.Table.SelectPos, vh, n); tp.Table.TopPos != want {
		t.Errorf("Tab: the view is at %d (cursor %d), want %d", tp.Table.TopPos, tp.Table.SelectPos, want)
	}

	// Focus that is already there (SetFocus runs on every frame) leaves the
	// cursor where the user put it.
	tp.setCursor(tp.Table.SelectPos + 1)
	moved := tp.SelectedPath()
	tp.SetFocus(true)
	if got := tp.SelectedPath(); got != moved {
		t.Errorf("a second SetFocus(true) moved the cursor from %q to %q", moved, got)
	}
}

// Without "Tree like Explorer's" the tree keeps far2l's placement: no jump
// to the middle, and Tab does not move the cursor.
func TestTreeWithoutExplorerModeKeepsItsPlacement(t *testing.T) {
	keepTreeExpandCache(t)
	oldCfg := config.App
	t.Cleanup(func() { config.App = oldCfg })
	config.App.TreeExplorerMode = false
	config.App.TreeRootWholeVolume = true

	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 40; i++ {
		if err := os.Mkdir(filepath.Join(root, fmt.Sprintf("d%02d", i)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fsp := NewFileSystemPanel(0, 0, 40, 14, vfs.NewOSVFS(filepath.Join(root, "d30")))
	waitForLoad(t, fsp)
	tp := NewTreePanel(fsp)
	before := tp.SelectedPath()
	if err := fsp.Vfs.SetPath(filepath.Join(root, "d05")); err != nil {
		t.Fatal(err)
	}
	tp.SetFocus(true)
	if got := tp.SelectedPath(); got != before {
		t.Errorf("Tab without Explorer mode moved the cursor from %q to %q", before, got)
	}
}

// A branch remembered from an earlier tree that lies above the panel's folder
// is expanded again when the tree opens; the cursor still lands on the
// panel's folder, not on the row the expansion pushed into its place.
func TestTreeOpensOnThePanelFolderBelowARememberedBranch(t *testing.T) {
	keepTreeExpandCache(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{"a/x1", "a/x2", "a/x3", "b"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	rememberTreeExpanded(filepath.Join(root, "a"))
	want := filepath.Join(root, "b")
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(want))
	waitForLoad(t, fsp)
	tp := NewTreePanel(fsp)
	if got := tp.SelectedPath(); got != want {
		t.Errorf("the tree opened on %q, want the panel's %q", got, want)
	}
}
