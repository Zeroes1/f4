package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtui"
)

// Every key a native key's action declares is written the way the hotkey
// table spells keys, so the configurator shows it and the action can press it.
func TestNativeKeyActionKeysAreHotkeySpellings(t *testing.T) {
	for _, spec := range nativeKeyActions() {
		for _, declared := range append(append([]string(nil), spec.keys...), spec.aliases...) {
			key, _, _ := strings.Cut(declared, ":")
			e := keymap.ParseFarKey(key)
			if e == nil || e.VirtualKeyCode == 0 || !strings.EqualFold(keymap.EventToHotkeyString(e), key) {
				t.Errorf("%s: %q is not a key in the hotkey table's spelling", spec.name, key)
			}
		}
	}
}

// The key inventory: the Hotkey Configurator lists the keys the panels, the
// drive menu and vtui handle themselves, aliases included, and a key the
// frame takes before the hotkey table keeps its row next to a binding on the
// same key.
func TestNativeKeysAreRowsOfTheHotkeyConfigurator(t *testing.T) {
	hm := keymap.NewHotkeyManager("")
	has := func(rows []hotkeyRow, actionName, key string) bool {
		for _, row := range rows {
			if row.Action == actionName && row.Key == key {
				return true
			}
		}
		return false
	}
	rows := buildHotkeyRows(hm, nil)
	for _, want := range []struct{ action, key string }{
		{"Panel.Bookmark.Go3", "RCtrl+3"},
		{"Panel.Bookmark.Go3", "Ctrl+Alt+3"},
		{"Panel.Bookmark.Save3", "Ctrl+Alt+Shift+3"},
		{"Panel.GoHome", "RCtrl+`"},
		{"Panel.Open", "Enter"},
		{"Panel.SwitchActive", "Shift+Tab"},
		{"CommandLine.Clear", "Ctrl+Y"},
		{"Panel.FastFind.Next", "Ctrl+Enter"},
		{"QuickView.Wrap", "F2"},
		{"QuickView.OpenViewer", "F3"},
		{"Panel.Player.MoveItemUp", "Ctrl+Up"},
		{"DriveMenu.AddLink", "Ins"},
		{"Workspace.New", "Ctrl+Alt+N"},
		{"Workspace.Go2", "Alt+2"},
		{"App.MainMenu", "Ctrl+F9"},
		{"Macro.Record", "Ctrl+."},
		{"Debug.ScreenDump", "Ctrl+Alt+Shift+P"},
	} {
		if !has(rows, want.action, want.key) {
			t.Errorf("the configurator has no %s row for %s", want.key, want.action)
		}
	}

	// A binding the user puts on a bookmark key replaces it
	// (ConfigurableHotkeyOwnsPanelBookmark), so its row goes.
	hm.Bind("Shell", "RCtrl3", "File.Copy")
	if has(buildHotkeyRows(hm, nil), "Panel.Bookmark.Go3", "RCtrl+3") {
		t.Error("RCtrl+3 is still shown as the bookmark's after it was bound to a command")
	}
}

// An action whose key only works in some state is offered and run only then.
func TestContextualNativeKeyActionsFollowTheirState(t *testing.T) {
	for _, name := range []string{"QuickView.Wrap", "Panel.FastFind.Next", "Panel.Player.Play", "TempPanel.List3", "Panel.ClosePluginPanel"} {
		act, ok := GetAction(name)
		if !ok {
			t.Fatalf("%s is not registered", name)
		}
		if act.Visible == nil || act.Visible() || act.Enabled == nil || act.Enabled() {
			t.Errorf("%s is offered with nothing it could act on", name)
		}
		if RunAction(name) {
			t.Errorf("%s ran with nothing it could act on", name)
		}
	}
	if crash, ok := GetAction("Debug.Crash"); !ok || crash.Visible == nil || crash.Visible() {
		t.Error("the test crash is offered in the command palette")
	}
}

func TestMacroRecordActionStartsRecording(t *testing.T) {
	previous := macro.MacroMgr
	macro.MacroMgr = macro.NewMacroManager("")
	t.Cleanup(func() { macro.MacroMgr = previous })
	if !RunAction("Macro.Record") || !macro.MacroMgr.Recording {
		t.Fatal("Macro.Record did not start a recording")
	}
	macro.MacroMgr.Recording = false
}

// The bookmark actions run the slots the bookmark keys reach.
func TestBookmarkActionsSaveAndGo(t *testing.T) {
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(paneltest.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ResizeConsole(100, 30)
	defer pf.Close()
	vtui.FrameManager.Push(pf)
	fsp := pf.GetActivePanel()
	saved := fsp.Vfs.GetPath()

	if !RunAction("Panel.Bookmark.Save6") {
		t.Fatal("Panel.Bookmark.Save6 did not run")
	}
	elsewhere, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pf.NavigateToPath(fsp, elsewhere)
	paneltest.WaitForLoad(t, fsp)
	if !RunAction("Panel.Bookmark.Go6") {
		t.Fatal("Panel.Bookmark.Go6 did not run")
	}
	paneltest.WaitForLoad(t, fsp)
	if got := fsp.Vfs.GetPath(); got != saved {
		t.Errorf("the panel is in %q after Panel.Bookmark.Go6, want %q", got, saved)
	}
	set, err := panel.LoadBookmarks(panel.BookmarksFilePath())
	if err != nil || set[6].Path != saved {
		t.Errorf("bookmark 6 holds %q (%v), want %q", set[6].Path, err, saved)
	}
}
