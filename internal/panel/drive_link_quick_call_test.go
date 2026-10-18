package panel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func quickCallKey(vk uint16, char rune, mods vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, Char: char, ControlKeyState: mods}
}

// mitek1977-coder, f4#1148: a link of the drive menu with "Use for quick
// call" opens with RCtrl or Ctrl+Alt and its hotkey, as the numbered
// bookmarks do; a link without it does not.
func TestDriveBookmarkQuickCallFindsTheLink(t *testing.T) {
	links := []DriveBookmark{
		{Name: "Tmp", Path: "/tmp", Hotkey: "T"},
		{Name: "Logs", Path: "/var/log", Hotkey: "L", QuickCall: true},
		{Name: "Logs again", Path: "/srv/log", Hotkey: "L", QuickCall: true},
	}
	for _, tc := range []struct {
		name string
		e    *vtinput.InputEvent
		want string
	}{
		{"RCtrl+L", quickCallKey('L', 0x0c, vtinput.RightCtrlPressed), "/var/log"},
		{"RCtrl+L with no character", quickCallKey('L', 0, vtinput.RightCtrlPressed), "/var/log"},
		{"Ctrl+Alt+L", quickCallKey('L', 0, vtinput.LeftCtrlPressed|vtinput.LeftAltPressed), "/var/log"},
		{"RCtrl+T, a link without quick call", quickCallKey('T', 0x14, vtinput.RightCtrlPressed), ""},
		{"RCtrl+X, no link", quickCallKey('X', 0x18, vtinput.RightCtrlPressed), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			link, ok := DriveBookmarkQuickCall(links, tc.e)
			if got := link.Path; ok != (tc.want != "") || got != tc.want {
				t.Errorf("link %q (%v), want %q", got, ok, tc.want)
			}
		})
	}
}

func TestQuickCallLinkMovesTheActivePanel(t *testing.T) {
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveDriveBookmarks(DriveBookmarksFilePath(), []DriveBookmark{{Name: "Logs", Path: target, Hotkey: "L", QuickCall: true}}); err != nil {
		t.Fatal(err)
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.ShowPanels = true
	start := t.TempDir()
	fsp := NewFileSystemPanel(0, 0, 40, 20, vfs.NewOSVFS(start))
	waitForLoad(t, fsp)
	pf.Panels[pf.ActiveIdx] = fsp

	if !pf.ProcessKey(quickCallKey('L', 0x0c, vtinput.RightCtrlPressed)) {
		t.Fatal("RCtrl+L was not taken")
	}
	waitForLoad(t, fsp)
	if got := fsp.Vfs.GetPath(); got != target {
		t.Errorf("the active panel is in %q, want the link's %q", got, target)
	}
}
