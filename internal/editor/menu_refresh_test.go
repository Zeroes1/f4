package editor

import (
	"testing"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/vtui"
)

// GetMenuBar is called several times for every key event; building the menu
// each time made a paste of 2000 lines take fifty seconds (unxed/f4#2157).
func TestEditorView_GetMenuBarDoesNotRebuildTheMenuForEveryCall(t *testing.T) {
	builds := 0
	oldItems := MenuBarItems
	MenuBarItems = func(string) []vtui.MenuBarItem {
		builds++
		return []vtui.MenuBarItem{{Label: "&File"}}
	}
	t.Cleanup(func() { MenuBarItems = oldItems })

	ev := NewEditorView(piecetable.New([]byte("x\n")), nil, "t.txt")
	t.Cleanup(ev.Close)
	builds = 0

	for i := 0; i < 500; i++ {
		if ev.GetMenuBar() == nil {
			t.Fatal("no menu bar")
		}
	}
	if builds > 2 {
		t.Fatalf("the menu was built %d times for 500 calls of GetMenuBar, want it built once and not again", builds)
	}
	if len(ev.GetMenuBar().Items) != 1 {
		t.Fatalf("the menu bar has %d items, want the built one", len(ev.GetMenuBar().Items))
	}
}

func TestEditorView_GetMenuBarRebuildsWhileTheBarIsUpAndWhenOld(t *testing.T) {
	builds := 0
	oldItems := MenuBarItems
	MenuBarItems = func(string) []vtui.MenuBarItem {
		builds++
		return []vtui.MenuBarItem{{Label: "&File"}}
	}
	t.Cleanup(func() { MenuBarItems = oldItems })

	ev := NewEditorView(piecetable.New([]byte("x\n")), nil, "t.txt")
	t.Cleanup(ev.Close)
	ev.GetMenuBar()

	ev.menuBar.Active = true
	before := builds
	ev.GetMenuBar()
	ev.GetMenuBar()
	if builds-before != 2 {
		t.Fatalf("with the bar up the menu was built %d times for 2 calls, want 2 (the checkmarks must be current)", builds-before)
	}

	ev.menuBar.Active = false
	ev.menuRefresh.at = time.Now().Add(-time.Second)
	before = builds
	ev.GetMenuBar()
	if builds-before != 1 {
		t.Fatalf("an old menu was built %d times, want 1", builds-before)
	}
}
