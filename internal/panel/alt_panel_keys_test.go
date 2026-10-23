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
// not see -- F5 and F6 there and in the info panel copied and moved it, and the
// player's Ctrl+arrows (docs/PLAYER.md) moved the split between the panels.
// The panels now veto them for the focused panel.
func TestFocusedAltPanelsKeepTheirKeys(t *testing.T) {
	pf, fsp := livePanels(t)
	key := func(vk uint16, mods vtinput.ControlKeyState) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: mods}
	}
	ctrl := vtinput.ControlKeyState(vtinput.LeftCtrlPressed)

	if pf.VetoActionKey(key(vtinput.VK_F3, 0)) || pf.VetoActionKey(key(vtinput.VK_UP, ctrl)) {
		t.Fatal("the file panel vetoes the hotkey table without an alternative panel")
	}

	shift := vtinput.ControlKeyState(vtinput.ShiftPressed)
	fileKeys := []*vtinput.InputEvent{
		key(vtinput.VK_F3, 0), key(vtinput.VK_F4, 0), key(vtinput.VK_F5, 0), key(vtinput.VK_F6, 0),
		key(vtinput.VK_F7, 0), key(vtinput.VK_F8, 0), key(vtinput.VK_DELETE, 0),
		key(vtinput.VK_F5, shift), key(vtinput.VK_F8, shift), key(vtinput.VK_DELETE, shift),
	}

	// The quick view and the info panel have no file under a cursor: every
	// file key stays away from the file panel hidden behind them.
	for _, alt := range []AltPanel{NewQuickViewPanel(fsp), NewInfoPanel(fsp)} {
		pf.AltPanels[pf.ActiveIdx] = alt
		alt.SetFocus(true)
		for _, e := range append([]*vtinput.InputEvent{key(vtinput.VK_F2, 0)}, fileKeys...) {
			if !pf.VetoActionKey(e) {
				t.Errorf("the focused %s gives %s (mods %#x) to the hotkey table", alt.Kind(), vtinput.VKString(e.VirtualKeyCode), e.ControlKeyState)
			}
		}
		if pf.VetoActionKey(key(vtinput.VK_F3, ctrl)) || pf.VetoActionKey(key(vtinput.VK_F5, vtinput.LeftAltPressed)) {
			t.Errorf("the focused %s takes Ctrl+F3 or Alt+F5, which are not file keys", alt.Kind())
		}
		alt.SetFocus(false)
		if pf.VetoActionKey(key(vtinput.VK_F8, 0)) {
			t.Errorf("an unfocused %s takes F8", alt.Kind())
		}
	}

	// The folder tree is a list of its own: F8 deletes the folder under its
	// cursor, so the file keys still reach the commands.
	tree := NewTreePanel(fsp)
	pf.AltPanels[pf.ActiveIdx] = tree
	tree.SetFocus(true)
	if pf.VetoActionKey(key(vtinput.VK_F8, 0)) || pf.VetoActionKey(key(vtinput.VK_F5, 0)) {
		t.Error("the focused tree keeps F8 or F5 from the hotkey table")
	}

	player := NewPlayerPanel(fsp)
	pf.AltPanels[pf.ActiveIdx] = player
	player.SetFocus(true)
	for _, e := range append([]*vtinput.InputEvent{
		key(vtinput.VK_UP, ctrl), key(vtinput.VK_DOWN, ctrl), key(vtinput.VK_LEFT, ctrl), key(vtinput.VK_RIGHT, ctrl), key(vtinput.VK_HOME, ctrl),
		key(vtinput.VK_RETURN, 0),
		{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: 'Z', Char: 'z'},
	}, fileKeys...) {
		if !pf.VetoActionKey(e) {
			t.Errorf("the focused player gives %s (mods %#x) to the hotkey table", vtinput.VKString(e.VirtualKeyCode), e.ControlKeyState)
		}
	}
	if pf.VetoActionKey(key(vtinput.VK_UP, ctrl|vtinput.ShiftPressed)) || pf.VetoActionKey(key(vtinput.VK_LEFT, ctrl|vtinput.LeftAltPressed)) {
		t.Error("the focused player takes a Ctrl+arrow chord it does not handle")
	}
}
