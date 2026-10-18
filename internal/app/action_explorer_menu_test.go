package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
)

// The Explorer context menu opens for the item under the cursor, and has no
// key of its own until one is chosen for it (f4#2041).
func TestAction_FileExplorerMenu_OpensForTheCursorItem(t *testing.T) {
	a, ok := action.Lookup("File.ExplorerMenu")
	if !ok {
		t.Fatal("File.ExplorerMenu is not registered")
	}
	if len(a.DefaultKeys) != 0 {
		t.Fatalf("File.ExplorerMenu keys = %q, want none yet", a.DefaultKeys)
	}

	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	pf := seedPanelForCopyName(t, tmp)
	opened := make(chan string, 1)
	old := explorerMenuShower
	explorerMenuShower = func(path string) error {
		opened <- path
		return nil
	}
	t.Cleanup(func() { explorerMenuShower = old })

	fsp := pf.GetActivePanel()
	fsp.SelectName("a.txt")
	waitForBothPanels(t, pf)
	actionFileExplorerMenu(pf)

	abs, err := fsp.Vfs.Abs(tmp)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-opened:
		if want := fsp.Vfs.Join(abs, "a.txt"); got != want {
			t.Fatalf("menu opened for %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the menu was not opened")
	}

	// Once the menu closes the action posts a task that reads both panels
	// again. Left in the queue it ran in the next test, and the read
	// recorded this TempDir in that test's folder history
	// (TestActionFoldersHistoryInsertPersistsLock). Nothing else reads the
	// panels now, so the active one loading means the task has run.
	deadline := time.After(5 * time.Second)
	for !fsp.IsLoading {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the panels were not read again after the menu")
		}
	}
	waitForBothPanels(t, pf)
}

func waitForBothPanels(t *testing.T, pf *panel.PanelsFrame) {
	t.Helper()
	for _, p := range pf.Panels {
		if fp, ok := p.(*panel.FileSystemPanel); ok {
			paneltest.WaitForLoad(t, fp)
		}
	}
}
