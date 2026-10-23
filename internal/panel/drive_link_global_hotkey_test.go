package panel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func globalKeyEvent(vk uint16, char rune, mods vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, Char: char, ControlKeyState: mods}
}

// useDriveLinksProfile points the links file at a fresh profile for one test.
func useDriveLinksProfile(t *testing.T) {
	t.Helper()
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() {
		config.UserConfigDir = oldUserConfigDir
		forgetDriveLinksCache()
	})
	forgetDriveLinksCache()
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
}

// useHotkeys installs a hotkey manager with the given bindings for one test.
func useHotkeys(t *testing.T, bindings map[string]map[string]string) *keymap.HotkeyManager {
	t.Helper()
	previous := keymap.GlobalHotkeysMgr
	hm := &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{},
		Bindings: bindings,
		IniPath:  filepath.Join(t.TempDir(), "hotkeys.ini"),
	}
	keymap.GlobalHotkeysMgr = hm
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previous })
	return hm
}

// f4#2171: a global hot key needs Ctrl or Alt, or is a function key; a key
// that types text or moves the cursor cannot be one.
func TestDriveLinkGlobalKeyAllowed(t *testing.T) {
	for key, want := range map[string]bool{
		"CtrlAltT":        true,
		"RCtrlY":          true,
		"AltD":            true,
		"CtrlShiftF11":    true,
		"F5":              true,
		"ShiftF24":        true,
		"CtrlAltShiftF11": true,
		"ctrlaltt":        true,
		"T":               false,
		"ShiftT":          false,
		"Space":           false,
		"Up":              false,
		"F25":             false,
		"F0":              false,
		"Ctrl":            false,
		"":                false,
	} {
		if got := DriveLinkGlobalKeyAllowed(key); got != want {
			t.Errorf("DriveLinkGlobalKeyAllowed(%q) = %v, want %v", key, got, want)
		}
	}
}

// A link written with Ctrl answers either Ctrl; one written with RCtrl only
// the right one, and it wins there over a Ctrl link with the same key.
func TestDriveLinkForKeyMatchesTheCtrlOfItsSpelling(t *testing.T) {
	links := []DriveBookmark{
		{Name: "Logs", Path: "/var/log", GlobalHotkey: "CtrlAltT"},
		{Name: "Home", Path: "/home", GlobalHotkey: "RCtrlY"},
		{Name: "Left", Path: "/left", GlobalHotkey: "CtrlAltL"},
		{Name: "Right", Path: "/right", GlobalHotkey: "RCtrlAltL"},
		{Name: "", Path: "/nameless", GlobalHotkey: "CtrlAltN"},
	}
	for key, want := range map[string]string{
		"CtrlAltT":  "/var/log",
		"RCtrlAltT": "/var/log",
		"RCtrlY":    "/home",
		"CtrlY":     "",
		"CtrlAltL":  "/left",
		"RCtrlAltL": "/right",
		"CtrlAltN":  "",
		"":          "",
	} {
		link, ok := driveLinkForKey(links, key)
		if ok != (want != "") || link.Path != want {
			t.Errorf("driveLinkForKey(%q) = %q (%v), want %q", key, link.Path, ok, want)
		}
	}
}

// A file of the "quick call" days (f4#1148) keeps working: QuickCall=true
// with the letter L becomes the global hot key RCtrl+L, and the next save
// writes the key and drops the flag. A digit was always the numbered
// bookmark's and is not carried over.
func TestDriveLinksLoadCarriesQuickCallOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "drive-bookmarks.ini")
	content := `[0]
Name=Logs
Path=/var/log
Hotkey=l
QuickCall=true

[1]
Name=Slot
Path=/slot
Hotkey=5
QuickCall=true

[2]
Name=Plain
Path=/plain
Hotkey=P
QuickCall=false

[3]
Name=Own
Path=/own
Hotkey=O
QuickCall=true
GlobalHotkey=CtrlAltO
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	links, err := LoadDriveBookmarks(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"RCtrlL", "", "", "CtrlAltO"}
	if len(links) != len(want) {
		t.Fatalf("loaded %d links, want %d", len(links), len(want))
	}
	for i, key := range want {
		if links[i].GlobalHotkey != key {
			t.Errorf("link %q: global hot key %q, want %q", links[i].Name, links[i].GlobalHotkey, key)
		}
	}
	if err := SaveDriveBookmarks(path, links); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "QuickCall") {
		t.Errorf("the saved file still says QuickCall:\n%s", data)
	}
	if !strings.Contains(string(data), "GlobalHotkey=RCtrlL\n") {
		t.Errorf("the saved file lacks the carried-over key:\n%s", data)
	}
	again, err := LoadDriveBookmarks(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range links {
		if again[i] != links[i] {
			t.Errorf("link %d after a round trip = %+v, want %+v", i, again[i], links[i])
		}
	}
}

func TestDriveLinkGlobalHotkeyMovesTheActivePanel(t *testing.T) {
	useDriveLinksProfile(t)
	target, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := SaveDriveBookmarks(DriveBookmarksFilePath(), []DriveBookmark{{Name: "Logs", Path: target, GlobalHotkey: "CtrlAltT"}}); err != nil {
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

	if pf.OpenDriveLinkForKey(globalKeyEvent('X', 0, vtinput.LeftCtrlPressed|vtinput.LeftAltPressed)) {
		t.Fatal("Ctrl+Alt+X was taken with no link on it")
	}
	if pf.OpenDriveLinkForKey(globalKeyEvent('T', 't', 0)) {
		t.Fatal("a plain T was taken")
	}
	if !pf.OpenDriveLinkForKey(globalKeyEvent('T', 0, vtinput.RightCtrlPressed|vtinput.LeftAltPressed)) {
		t.Fatal("RCtrl+Alt+T did not open the Ctrl+Alt+T link")
	}
	waitForLoad(t, fsp)
	if got := fsp.Vfs.GetPath(); got != target {
		t.Errorf("the active panel is in %q, want the link's %q", got, target)
	}

	// A saved change is seen at the next key, whatever the file's time says.
	if err := SaveDriveBookmarks(DriveBookmarksFilePath(), []DriveBookmark{{Name: "Other", Path: other, GlobalHotkey: "CtrlShiftF11"}}); err != nil {
		t.Fatal(err)
	}
	if pf.OpenDriveLinkForKey(globalKeyEvent('T', 0, vtinput.LeftCtrlPressed|vtinput.LeftAltPressed)) {
		t.Fatal("Ctrl+Alt+T still opens a link that no longer has it")
	}
	if !pf.OpenDriveLinkForKey(globalKeyEvent(vtinput.VK_F11, 0, vtinput.LeftCtrlPressed|vtinput.ShiftPressed)) {
		t.Fatal("Ctrl+Shift+F11 did not open the link saved with it")
	}
	waitForLoad(t, fsp)
	if got := fsp.Vfs.GetPath(); got != other {
		t.Errorf("the active panel is in %q, want %q", got, other)
	}

	// With the panels hidden the key belongs to the terminal.
	pf.ShowPanels = false
	if pf.OpenDriveLinkForKey(globalKeyEvent(vtinput.VK_F11, 0, vtinput.LeftCtrlPressed|vtinput.ShiftPressed)) {
		t.Error("the link was opened with the panels hidden")
	}
}

// The drive menu shows a link's global hot key at the right edge of its row,
// as menus show the key of a command, and the box is wide enough for the name
// and the key with two blanks between them.
func TestDriveMenuShowsTheGlobalHotkeyOfALink(t *testing.T) {
	useDriveLinksProfile(t)
	place := t.TempDir()
	if err := SaveDriveBookmarks(DriveBookmarksFilePath(), []DriveBookmark{
		{Name: "Logs", Path: place, Hotkey: "L", GlobalHotkey: "CtrlAltShiftF11"},
		{Name: "Plain", Path: place},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(120, 40)
	vtui.FrameManager.Init(scr)
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(120, 40)
	pf.ShowDriveMenu(0)
	menu, ok := driveMenuFromFrame(vtui.FrameManager.GetTopFrame())
	if !ok {
		t.Fatal("no drive menu on top")
	}
	defer menu.Close()

	row := -1
	for i, item := range menu.Items {
		switch {
		case strings.Contains(item.Text, "Logs"):
			row = i
			if item.Shortcut != "Ctrl+Alt+Shift+F11" {
				t.Errorf("Logs row shows %q at its edge, want Ctrl+Alt+Shift+F11", item.Shortcut)
			}
		case strings.Contains(item.Text, "Plain") && item.Shortcut != "":
			t.Errorf("a link without a global hot key shows %q", item.Shortcut)
		}
	}
	if row < 0 {
		t.Fatal("no Logs row")
	}
	x1, y1, x2, _ := menu.GetPosition()
	clean, _, _ := vtui.ParseAmpersandString(menu.Items[row].Text)
	if need := vtui.StringWidth(clean) + driveMenuShortcutGap + vtui.StringWidth("Ctrl+Alt+Shift+F11") + driveMenuBoxChrome; x2-x1+1 < need {
		t.Errorf("the menu is %d wide, the Logs row needs %d", x2-x1+1, need)
	}
	menu.SetSelectPos(row)
	menu.Show(scr)
	// Inside the borders: the key, then the one blank before the right one.
	line := testutil.ScreenRow(scr, y1+1+row-menu.TopPos, x1+1, x2-1)
	if !strings.HasSuffix(line, "Ctrl+Alt+Shift+F11 ") || !strings.Contains(line, "Logs  ") {
		t.Errorf("the Logs row reads %q, want the name, then the key at the right edge", line)
	}
}

func ownerNames(owners []DriveLinkKeyOwner) []string {
	var names []string
	for _, owner := range owners {
		names = append(names, owner.Name)
	}
	return names
}

// f4#2171: the link dialog names what a key would be taken from. A command
// loses the key in Shell only; a Ctrl link takes the RCtrl spelling of a
// command too, an RCtrl link leaves the Ctrl one alone; the numbered bookmarks
// cannot give their keys up.
func TestDriveLinkKeyOwners(t *testing.T) {
	hm := useHotkeys(t, map[string]map[string]string{
		"Shell":  {"F5": "Test.Copy", "RCtrlAltK": "Test.RightK", "CtrlAltN": "None"},
		"Common": {"CtrlAltT": "Test.Everywhere:SomeCondition", "CtrlAltN": "Test.Shadowed"},
	})
	oldExtra := DriveLinkExtraKeyOwners
	DriveLinkExtraKeyOwners = func(key string) []DriveLinkKeyOwner {
		if key == "CtrlAltM" {
			return []DriveLinkKeyOwner{{Name: "application owner"}}
		}
		return nil
	}
	t.Cleanup(func() { DriveLinkExtraKeyOwners = oldExtra })
	others := []DriveBookmark{{Name: "Docs", Path: "/docs", GlobalHotkey: "ctrlaltd"}}

	cases := []struct {
		key   string
		names []string
		fixed bool
	}{
		{"F5", []string{`command "Test.Copy"`}, false},
		{"CtrlAltT", []string{`command "Test.Everywhere"`}, false},
		{"RCtrlAltT", nil, false},
		{"CtrlAltK", []string{`command "Test.RightK"`}, false},
		{"CtrlAltN", nil, false},
		{"CtrlAltD", []string{`link "Docs"`}, false},
		{"CtrlAltM", []string{"application owner"}, false},
		{"AltD", []string{"quick search in the panel"}, false},
		{"RCtrl5", []string{"folder bookmark 5"}, true},
		{"RCtrlShift5", []string{"saving folder bookmark 5"}, true},
		{"CtrlAltVK_C0", []string{"going to the home folder"}, true},
		{"CtrlAltQ", nil, false},
	}
	for _, tc := range cases {
		owners := driveLinkKeyOwners(tc.key, others)
		if got := ownerNames(owners); strings.Join(got, "|") != strings.Join(tc.names, "|") {
			t.Errorf("owners of %s = %q, want %q", tc.key, got, tc.names)
			continue
		}
		if len(owners) > 0 && owners[0].Fixed != tc.fixed {
			t.Errorf("owner of %s fixed = %v, want %v", tc.key, owners[0].Fixed, tc.fixed)
		}
	}

	// Taking F5 unbinds it in Shell, where the links are asked, and writes
	// that down.
	owners := driveLinkKeyOwners("F5", nil)
	if len(owners) != 1 || owners[0].Release == nil {
		t.Fatalf("F5 owners = %+v", owners)
	}
	if err := owners[0].Release(); err != nil {
		t.Fatal(err)
	}
	if got := hm.Bindings["Shell"]["F5"]; got != "None" {
		t.Errorf("Shell F5 after the link took it = %q, want None", got)
	}
	if data, err := os.ReadFile(hm.IniPath); err != nil || !strings.Contains(string(data), "F5=None") {
		t.Errorf("hotkeys file after the link took F5: %q, %v", data, err)
	}
	if owners := driveLinkKeyOwners("F5", nil); len(owners) != 0 {
		t.Errorf("F5 still has owners after it was taken: %q", ownerNames(owners))
	}

	// Taking the RCtrl spelling of a command leaves its Common binding to the
	// editor and the viewer.
	for _, owner := range driveLinkKeyOwners("CtrlAltT", nil) {
		if owner.Release != nil {
			if err := owner.Release(); err != nil {
				t.Fatal(err)
			}
		}
	}
	if got := hm.Bindings["Common"]["CtrlAltT"]; got != "Test.Everywhere:SomeCondition" {
		t.Errorf("Common CtrlAltT = %q, want it kept", got)
	}
	if got := hm.Bindings["Shell"]["CtrlAltT"]; got != "None" {
		t.Errorf("Shell CtrlAltT = %q, want None", got)
	}
}

// Alt and a digit switches workspaces before the links are asked, when the
// frame manager does that.
func TestDriveLinkKeyOwnersNameTheWorkspaceSwitch(t *testing.T) {
	useHotkeys(t, map[string]map[string]string{})
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	old := vtui.FrameManager.WorkspaceAltNumberSwitch
	t.Cleanup(func() { vtui.FrameManager.WorkspaceAltNumberSwitch = old })
	vtui.FrameManager.WorkspaceAltNumberSwitch = true
	got := ownerNames(driveLinkKeyOwners("Alt3", nil))
	if strings.Join(got, "|") != "switching to workspace 3|quick search in the panel" {
		t.Errorf("owners of Alt+3 = %q", got)
	}
	vtui.FrameManager.WorkspaceAltNumberSwitch = false
	if got := ownerNames(driveLinkKeyOwners("Alt3", nil)); strings.Join(got, "|") != "quick search in the panel" {
		t.Errorf("owners of Alt+3 without the switch = %q", got)
	}
}
