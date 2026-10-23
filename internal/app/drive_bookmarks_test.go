package app

import (
	"github.com/unxed/f4/internal/panel"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// f4#2171: in the panels a link's global hot key opens the link before the
// macros and the hotkey table get the key; a key no link has goes on to them.
func TestKeyRouterOpensADriveLinkBeforeMacrosAndCommands(t *testing.T) {
	preserveActionRegistry(t)
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)

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
	if err := panel.SaveDriveBookmarks(panel.DriveBookmarksFilePath(), []panel.DriveBookmark{{Name: "Logs", Path: target, GlobalHotkey: "CtrlAltT"}}); err != nil {
		t.Fatal(err)
	}

	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ResizeConsole(80, 25)
	defer pf.Close()
	vtui.FrameManager.Push(pf)

	ran := map[string]int{}
	for _, name := range []string{"Test.LinkKey", "Test.OtherKey"} {
		name := name
		action.RegisterAction(action.Action{Name: name, Area: "Shell", Handler: func() bool { ran[name]++; return true }})
	}
	shell := map[string]string{"CtrlAltT": "Test.LinkKey", "CtrlAltQ": "Test.OtherKey"}
	previousHotkeys, previousMacro := keymap.GlobalHotkeysMgr, macro.MacroMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{"Shell": shell},
		Bindings: map[string]map[string]string{"Shell": shell},
	}
	manager := &macro.MacroManager{Macros: map[string]map[string][]*vtinput.InputEvent{
		"Shell": {"CtrlAltT": {{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F1}}},
	}}
	macro.MacroMgr = manager
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = previousHotkeys
		macro.MacroMgr = previousMacro
	})

	ctrlAlt := vtinput.LeftCtrlPressed | vtinput.LeftAltPressed
	if !macroFilter(manager, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: 'T', ControlKeyState: ctrlAlt}) {
		t.Fatal("Ctrl+Alt+T was not taken")
	}
	if ran["Test.LinkKey"] != 0 {
		t.Error("the command bound to the link's key ran")
	}
	fsp := pf.GetActivePanel()
	paneltest.WaitForLoad(t, fsp)
	if got := fsp.Vfs.GetPath(); got != target {
		t.Errorf("the active panel is in %q, want the link's %q", got, target)
	}

	if !macroFilter(manager, &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: 'Q', ControlKeyState: ctrlAlt}) || ran["Test.OtherKey"] != 1 {
		t.Errorf("Ctrl+Alt+Q, which no link has, ran its command %d times, want 1", ran["Test.OtherKey"])
	}
}

// The key router's part of what a link's key would be taken from: macro
// recording cannot give Ctrl+. up, the palette's own Ctrl+Alt+P can be taken,
// and a macro is named, on the right Ctrl too: it plays on either Ctrl.
func TestDriveLinkRouterKeyOwners(t *testing.T) {
	previousHotkeys := keymap.GlobalHotkeysMgr
	hm := &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{},
		Bindings: map[string]map[string]string{},
		IniPath:  filepath.Join(t.TempDir(), "hotkeys.ini"),
	}
	keymap.GlobalHotkeysMgr = hm
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previousHotkeys })
	manager := &macro.MacroManager{Macros: map[string]map[string][]*vtinput.InputEvent{
		"Common": {"CtrlAlt;": {{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F1}}},
		"Editor": {"CtrlAltE": {{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F1}}},
	}}

	owners := driveLinkRouterKeyOwners(manager, "CtrlVK_BE")
	if len(owners) != 1 || !owners[0].Fixed {
		t.Errorf("owners of Ctrl+. = %+v, want macro recording, fixed", owners)
	}
	owners = driveLinkRouterKeyOwners(manager, "CtrlAltVK_BA")
	if len(owners) != 1 || owners[0].Fixed || owners[0].Release != nil {
		t.Errorf("owners of Ctrl+Alt+; = %+v, want the Common macro, kept", owners)
	}
	if owners := driveLinkRouterKeyOwners(manager, "RCtrlAltVK_BA"); len(owners) != 1 || owners[0].Fixed {
		t.Errorf("owners of Right Ctrl+Alt+; = %+v, want the macro, which plays on either Ctrl", owners)
	}
	if owners := driveLinkRouterKeyOwners(manager, "CtrlAltE"); len(owners) != 0 {
		t.Errorf("an Editor macro is named for the panels: %+v", owners)
	}

	owners = driveLinkRouterKeyOwners(manager, commandPaletteLegacyKey)
	if len(owners) != 1 || owners[0].Release == nil {
		t.Fatalf("owners of Ctrl+Alt+P = %+v, want the palette, releasable", owners)
	}
	if err := owners[0].Release(); err != nil {
		t.Fatal(err)
	}
	event := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: 'P', ControlKeyState: vtinput.LeftCtrlPressed | vtinput.LeftAltPressed}
	if commandPaletteLegacyShortcut("Shell", event) {
		t.Error("Ctrl+Alt+P still opens the palette in the panels after a link took it")
	}
	if owners := driveLinkRouterKeyOwners(manager, commandPaletteLegacyKey); len(owners) != 0 {
		t.Errorf("Ctrl+Alt+P still has owners after it was taken: %+v", owners)
	}
}

// f4#2171, the keys the user found offered as free: with the real action
// registry the link dialog names what Ctrl+N, Ctrl+Alt+N, Right Ctrl+Tab and
// F3 do in the panels. A key the framework keeps for the terminal is a plain
// key of the panels; one that works in some state says which.
func TestDriveLinkKeyOwnersKnowEveryKeyOfThePanels(t *testing.T) {
	previousHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previousHotkeys })
	previousExtra := panel.DriveLinkExtraKeyOwners
	panel.DriveLinkExtraKeyOwners = func(key string) []panel.DriveLinkKeyOwner {
		return driveLinkRouterKeyOwners(&macro.MacroManager{}, key)
	}
	t.Cleanup(func() { panel.DriveLinkExtraKeyOwners = previousExtra })
	names := func(key string) []string {
		var out []string
		for _, owner := range panel.DriveLinkGlobalKeyOwners(key) {
			out = append(out, owner.Name)
		}
		return out
	}
	for key, want := range map[string]string{
		"CtrlN":    `built-in key of "New workspace"`,
		"CtrlAltN": `built-in key of "New workspace"`,
		"RCtrlTab": `built-in key of "Next workspace"`,
		"F3":       `built-in key of "Quick view: open in viewer" when QuickViewFocused`,
	} {
		if got := names(key); !slices.Contains(got, want) {
			t.Errorf("owners of %s = %q, want %q among them", key, got, want)
		}
	}
	if got := strings.Join(names("F3"), "\n"); !strings.Contains(got, `command "View"`) {
		t.Errorf("owners of F3 = %q, want the View command among them", got)
	}
	fixed := false
	for _, owner := range panel.DriveLinkGlobalKeyOwners("CtrlVK_BE") {
		fixed = fixed || owner.Fixed
	}
	if !fixed {
		t.Error("Ctrl+., which starts a recording before the links are asked, is offered to a link")
	}
}

func TestDriveBookmarksRoundTripHasNoTenItemLimit(t *testing.T) {
	bookmarks := make([]panel.DriveBookmark, 12)
	for i := range bookmarks {
		bookmarks[i] = panel.DriveBookmark{
			Name:   "Folder " + string(rune('A'+i)),
			Path:   filepath.Join("$HOME", "folder", string(rune('a'+i))),
			Hotkey: "CtrlF" + string(rune('1'+i)),
		}
	}

	path := filepath.Join(t.TempDir(), "drive-bookmarks.ini")
	if err := panel.SaveDriveBookmarks(path, bookmarks); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := panel.LoadDriveBookmarks(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != len(bookmarks) {
		t.Fatalf("loaded %d bookmarks, want %d", len(got), len(bookmarks))
	}
	for i := range bookmarks {
		if got[i] != bookmarks[i] {
			t.Errorf("bookmark %d = %#v, want %#v", i, got[i], bookmarks[i])
		}
	}
}

func TestDriveBookmarksLoadSkipsInvalidAndCompactsSections(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drive-bookmarks.ini")
	content := `[4]
Name=Later
Path=/later
Hotkey=F8

[foreign]
Name=Ignored
Path=/ignored

[9]
Name=First
Path=/first
Hotkey=Ф

[12]
Path=/missing-name
`
	if err := config.WriteUserFileAtomically(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := panel.LoadDriveBookmarks(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []panel.DriveBookmark{{Name: "Later", Path: "/later", Hotkey: "F8"}, {Name: "First", Path: "/first", Hotkey: "Ф"}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("loaded bookmarks = %#v, want %#v", got, want)
	}
}

func TestDriveBookmarkMenuTextUsesNameAndKeepsHotkeyLeft(t *testing.T) {
	if got := panel.DriveBookmarkMenuText(panel.DriveBookmark{Name: "Long folder", Path: "/tmp/x", Hotkey: "Q"}); got != "&Q Long folder" {
		t.Fatalf("menu text = %q, want left-side letter shortcut", got)
	}
	if got := panel.DriveBookmarkMenuText(panel.DriveBookmark{Name: "Русская папка", Path: "/tmp/x", Hotkey: "Ф"}); got != "&Ф Русская папка" {
		t.Fatalf("Cyrillic menu text = %q, want left-side letter shortcut", got)
	}
	if got := panel.DriveBookmarkMenuText(panel.DriveBookmark{Name: "Long folder", Path: "/tmp/x", Hotkey: "CtrlF5"}); got != "Ctrl+F5 Long folder" {
		t.Fatalf("chord menu text = %q, want formatted left-side shortcut", got)
	}
	if got := panel.DriveBookmarkMenuText(panel.DriveBookmark{Name: "A & B", Path: "/tmp/x"}); strings.Contains(got, "/tmp/x") || !strings.Contains(got, "A && B") {
		t.Fatalf("menu text = %q, want escaped name without path", got)
	}
}

func TestDriveBookmarkKeyMatchesFarEventSpelling(t *testing.T) {
	if !panel.DriveBookmarkKeyMatches(panel.DriveBookmark{Name: "Русский", Path: "/tmp", Hotkey: "Ф"}, keymap.EventToHotkeyString(keymap.ParseFarKey("Ф"))) {
		t.Fatal("Cyrillic drive bookmark key did not match")
	}
	if !panel.DriveBookmarkKeyMatches(panel.DriveBookmark{Name: "Folder", Path: "/tmp", Hotkey: "CtrlF5"}, keymap.EventToHotkeyString(keymap.ParseFarKey("CtrlF5"))) {
		t.Fatal("chord drive bookmark key did not match")
	}
}

func TestDriveBookmarkEditorCapturesAnyKeyAndCanClearIt(t *testing.T) {
	dialog := panel.NewDriveBookmarkEditDialog(panel.DriveBookmark{}, "/tmp/default", nil)
	dialog.SetFocusedItem(dialog.HotkeyEdit)
	fieldX1, _, fieldX2, _ := dialog.HotkeyEdit.GetPosition()
	if fieldX2-fieldX1+1 != 1 {
		t.Fatalf("hotkey field width = %d, want one character", fieldX2-fieldX1+1)
	}
	labelX1, _, labelX2, _ := dialog.GetChildren()[4].GetPosition()
	if labelX1 >= fieldX1 || labelX2 >= fieldX1 {
		t.Fatalf("hotkey label position = %d..%d, field starts at %d", labelX1, labelX2, fieldX1)
	}

	if !dialog.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'ф'}) {
		t.Fatal("editor did not consume a Cyrillic hotkey")
	}
	if got := dialog.HotkeyEdit.GetText(); got != "Ф" {
		t.Fatalf("captured hotkey = %q, want Ф", got)
	}
	if !dialog.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_DELETE}) {
		t.Fatal("editor did not consume Delete while clearing hotkey")
	}
	if got := dialog.HotkeyEdit.GetText(); got != "" {
		t.Fatalf("cleared hotkey = %q, want empty", got)
	}
}
