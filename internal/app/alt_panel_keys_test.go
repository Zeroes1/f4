package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// With the focus in an alternative panel that has no file under a cursor, F5,
// F6 and F8 opened Copy, Move and Delete for the file under the cursor of the
// file panel hidden behind it: the user saw a quick view or the info panel and
// was asked to delete a file that was not on screen. Pressed the way the key
// router delivers them, they now open nothing.
func TestFileKeysDoNotReachTheFilePanelBehindAnAltPanel(t *testing.T) {
	kinds := []struct {
		name string
		make func(src *panel.FileSystemPanel) panel.AltPanel
	}{
		{"info", func(src *panel.FileSystemPanel) panel.AltPanel { return panel.NewInfoPanel(src) }},
		{"quick view", func(src *panel.FileSystemPanel) panel.AltPanel { return panel.NewQuickViewPanel(src) }},
		{"player", func(src *panel.FileSystemPanel) panel.AltPanel { return panel.NewPlayerPanel(src) }},
		{"AI chat", func(src *panel.FileSystemPanel) panel.AltPanel { return NewAIChatPanel(src) }},
	}
	for _, kind := range kinds {
		for _, vk := range []uint16{vtinput.VK_F5, vtinput.VK_F6, vtinput.VK_F8} {
			t.Run(kind.name+" "+vtinput.VKString(vk), func(t *testing.T) {
				pf := panelsWithHiddenFile(t)
				alt := kind.make(pf.GetActivePanel())
				pf.AltPanels[pf.ActiveIdx] = alt
				alt.SetFocus(true)
				pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk})
				if top := vtui.FrameManager.GetTopFrame(); top != pf {
					t.Errorf("%s opened %T %q", vtinput.VKString(vk), top, dialogTexts(top))
				}
			})
		}
	}
}

// The folder tree is a list of its own, and F8 there still deletes the
// folder under its cursor, not the hidden file.
func TestFileKeysStillWorkInTheFolderTree(t *testing.T) {
	pf := panelsWithHiddenFile(t)
	tree := panel.NewTreePanel(pf.GetActivePanel())
	pf.AltPanels[pf.ActiveIdx] = tree
	tree.SetFocus(true)
	pressKey(pf, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F8})
	top := vtui.FrameManager.GetTopFrame()
	if top == pf {
		t.Fatal("F8 in the focused tree opened nothing")
	}
	if text := dialogTexts(top); strings.Contains(text, "hidden.txt") {
		t.Errorf("F8 in the tree offered to delete the hidden file:\n%s", text)
	}
}

// panelsWithHiddenFile is a panels frame whose active file panel has its
// cursor on hidden.txt, the file an alternative panel on that side hides.
func panelsWithHiddenFile(t *testing.T) *panel.PanelsFrame {
	t.Helper()
	initFrameworkActionTestScreen(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hidden.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ResizeConsole(100, 30)
	t.Cleanup(pf.Close)
	pf.ShowPanels = true
	vtui.FrameManager.Push(pf)
	fsp := pf.GetActivePanel()
	pf.SwitchToVFS(fsp, vfs.NewOSVFS(dir))
	paneltest.WaitForLoad(t, fsp)
	fsp.SetCursorIndex(1)
	return pf
}
