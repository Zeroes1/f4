package viewer

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// countingApp counts the builds of the menu; the rest of Application is not
// reached by GetMenuBar.
type countingApp struct {
	Application
	builds int
}

func (a *countingApp) MenuBarItems(string) []vtui.MenuBarItem {
	a.builds++
	return []vtui.MenuBarItem{{Label: "&File"}}
}

// GetMenuBar is asked for several times for every key event; building the menu
// each time is the cost that made a paste take fifty seconds in the editor
// (unxed/f4#2157).
func TestViewerView_GetMenuBarDoesNotRebuildTheMenuForEveryCall(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "v.txt")
	if err := os.WriteFile(path, []byte("text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	vv, err := NewViewerView(context.Background(), vfs.NewOSVFS(root), path)
	if err != nil {
		t.Fatal(err)
	}
	defer vv.Close()

	oldApp := App
	app := &countingApp{}
	App = app
	t.Cleanup(func() { App = oldApp })

	for i := 0; i < 500; i++ {
		vv.GetMenuBar()
	}
	if app.builds > 2 {
		t.Fatalf("the menu was built %d times for 500 calls, want it built once and not again", app.builds)
	}

	vv.menuBar.Active = true
	before := app.builds
	vv.GetMenuBar()
	vv.GetMenuBar()
	if app.builds-before != 2 {
		t.Fatalf("with the bar up the menu was built %d times for 2 calls, want 2", app.builds-before)
	}

	vv.menuBar.Active = false
	vv.menuRefresh.at = time.Now().Add(-time.Second)
	before = app.builds
	vv.GetMenuBar()
	if app.builds-before != 1 {
		t.Fatalf("an old menu was built %d times, want 1", app.builds-before)
	}
}
