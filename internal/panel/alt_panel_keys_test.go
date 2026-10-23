package panel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// livePanels is a panels frame on a fresh frame manager with a real file
// panel in a temporary folder on the active side, and a profile of its own
// for the bookmarks.
func livePanels(t *testing.T) (*PanelsFrame, *FileSystemPanel) {
	t.Helper()
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()
	pf := NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(100, 30)
	pf.ShowPanels = true
	fsp := NewFileSystemPanel(0, 0, 50, 25, vfs.NewOSVFS(t.TempDir()))
	waitForLoad(t, fsp)
	pf.Panels[pf.ActiveIdx] = fsp
	vtui.FrameManager.Push(pf)
	return pf, fsp
}

// The hotkey table took keys of the focused alternative panels first: the
// quick view's F3/F4/F7/F8 ran File.View, File.Edit, File.MakeDir and
// File.Delete on the file panel behind it -- F8 deleted a file the user could
// not see -- and the player's Ctrl+arrows (docs/PLAYER.md) moved the split
// between the panels. The panels now veto them for the focused panel.
func TestFocusedAltPanelsKeepTheirKeys(t *testing.T) {
	pf, fsp := livePanels(t)
	key := func(vk uint16, mods vtinput.ControlKeyState) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: mods}
	}
	ctrl := vtinput.ControlKeyState(vtinput.LeftCtrlPressed)

	if pf.VetoActionKey(key(vtinput.VK_F3, 0)) || pf.VetoActionKey(key(vtinput.VK_UP, ctrl)) {
		t.Fatal("the file panel vetoes the hotkey table without an alternative panel")
	}

	quick := NewQuickViewPanel(fsp)
	pf.AltPanels[pf.ActiveIdx] = quick
	quick.SetFocus(true)
	for _, vk := range []uint16{vtinput.VK_F2, vtinput.VK_F3, vtinput.VK_F4, vtinput.VK_F7, vtinput.VK_F8} {
		if !pf.VetoActionKey(key(vk, 0)) {
			t.Errorf("the focused quick view gives %s to the hotkey table", vtinput.VKString(vk))
		}
	}
	if pf.VetoActionKey(key(vtinput.VK_F3, ctrl)) {
		t.Error("the focused quick view takes Ctrl+F3, which it does not handle")
	}
	quick.SetFocus(false)
	if pf.VetoActionKey(key(vtinput.VK_F3, 0)) {
		t.Error("an unfocused quick view takes F3")
	}

	player := NewPlayerPanel(fsp)
	pf.AltPanels[pf.ActiveIdx] = player
	player.SetFocus(true)
	for _, e := range []*vtinput.InputEvent{
		key(vtinput.VK_UP, ctrl), key(vtinput.VK_DOWN, ctrl), key(vtinput.VK_LEFT, ctrl), key(vtinput.VK_RIGHT, ctrl), key(vtinput.VK_HOME, ctrl),
		key(vtinput.VK_F7, 0), key(vtinput.VK_DELETE, 0), key(vtinput.VK_RETURN, 0),
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: 'Z', Char: 'z'},
	} {
		if !pf.VetoActionKey(e) {
			t.Errorf("the focused player gives %s (mods %#x) to the hotkey table", vtinput.VKString(e.VirtualKeyCode), e.ControlKeyState)
		}
	}
	if pf.VetoActionKey(key(vtinput.VK_UP, ctrl|vtinput.ShiftPressed)) || pf.VetoActionKey(key(vtinput.VK_LEFT, ctrl|vtinput.LeftAltPressed)) {
		t.Error("the focused player takes a Ctrl+arrow chord it does not handle")
	}
}
