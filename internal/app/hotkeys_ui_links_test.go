package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// useLinksFile points the drive-menu links file at a fresh profile holding
// links, for one test.
func useLinksFile(t *testing.T, links []panel.DriveBookmark) string {
	t.Helper()
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}
	path := panel.DriveBookmarksFilePath()
	if err := panel.SaveDriveBookmarks(path, links); err != nil {
		t.Fatal(err)
	}
	return path
}

// linkStore is a DriveLinkStore held in memory.
type linkStore struct{ links []panel.DriveBookmark }

func (s *linkStore) DriveLinks() []panel.DriveBookmark {
	return append([]panel.DriveBookmark(nil), s.links...)
}

func (s *linkStore) SetDriveLink(index int, link panel.DriveBookmark) error {
	s.links[index] = link
	return nil
}

func ctrlAltKey(vk uint16) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: vtinput.LeftCtrlPressed | vtinput.LeftAltPressed}
}

// topMessage is the text of the message box on top of the frames.
func topMessage(t *testing.T) (vtui.Container, string) {
	t.Helper()
	top, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want a message box", vtui.FrameManager.GetTopFrame())
	}
	var lines []string
	for _, child := range top.GetChildren() {
		if text, ok := child.(*vtui.Text); ok {
			lines = append(lines, text.GetText())
		}
	}
	return top, strings.Join(lines, "\n")
}

// f4#2171: every key f4 reacts to is a row of the configurator, the global
// hot key of a drive-menu link included. A link without a key has an empty
// row, as a command without one does, so a key can be given to it there.
func TestHotkeyConfiguratorListsTheDriveLinks(t *testing.T) {
	hm := keymap.NewHotkeyManager("")
	links := []panel.DriveBookmark{
		{Name: "Docs", Path: "/docs", GlobalHotkey: "CtrlAltD"},
		{Name: "Logs", Path: "/var/log"},
		{Name: "", Path: "/nameless", GlobalHotkey: "CtrlAltN"},
	}
	byLabel := map[string]hotkeyRow{}
	for _, row := range buildHotkeyRows(hm, links) {
		if row.IsLink {
			byLabel[row.Label] = row
		}
	}
	if len(byLabel) != 2 {
		t.Fatalf("link rows = %+v, want Docs and Logs", byLabel)
	}
	docs := byLabel[`Link "Docs"`]
	if docs.Key != "Ctrl+Alt+D" || docs.RawKey != "CtrlAltD" || docs.Area != "Shell" || !docs.Editable || docs.LinkIndex != 0 || !strings.Contains(docs.Desc, "/docs") {
		t.Errorf("Docs row = %+v", docs)
	}
	if docs.builtIn() || docs.GetCellText(hotkeyColMark) != "" {
		t.Error("a link's key is marked as a built-in key")
	}
	logs := byLabel[`Link "Logs"`]
	if logs.Key != "" || !logs.Editable || logs.LinkIndex != 1 {
		t.Errorf("Logs row = %+v", logs)
	}
}

// The configurator changes a link's key in its draft of the links file, as it
// changes the commands' keys in its draft of the hotkey table, and Save writes
// both. Assign opens the link dialog, which checks the key against the draft
// and takes it from a command there; Unbind clears the key.
func TestHotkeyConfiguratorChangesLinkKeysUntilSave(t *testing.T) {
	path := useLinksFile(t, []panel.DriveBookmark{
		{Name: "Docs", Path: "/docs", GlobalHotkey: "CtrlAltD"},
		{Name: "Logs", Path: "/var/log"},
	})
	previous := keymap.GlobalHotkeysMgr
	manager := keymap.NewHotkeyManager("")
	manager.Bind("Shell", "CtrlAltQ", "Test.Q")
	keymap.GlobalHotkeysMgr = manager
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previous })
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(160, 40)
	vtui.FrameManager.Init(screen)

	actionHotkeyConfig(nil)
	configurator, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame = %T, want the configurator", vtui.FrameManager.GetTopFrame())
	}
	var table *vtui.Table
	buttons := map[string]*vtui.Button{}
	for _, child := range configurator.GetChildren() {
		switch c := child.(type) {
		case *vtui.Table:
			table = c
		case *vtui.Button:
			buttons[testutil.GetCleanText(c)] = c
		}
	}
	if table == nil || buttons["Assign"] == nil || buttons["Unbind"] == nil || buttons["Save"] == nil {
		t.Fatal("configurator controls missing")
	}
	selectLink := func(label string) hotkeyRow {
		t.Helper()
		for i, row := range table.Rows {
			if r, ok := row.(hotkeyRow); ok && r.IsLink && r.Label == label {
				table.SelectPos = i
				return r
			}
		}
		t.Fatalf("no row %s", label)
		return hotkeyRow{}
	}

	selectLink(`Link "Logs"`)
	buttons["Assign"].OnClick()
	linkDialog, ok := vtui.FrameManager.GetTopFrame().(vtui.Container)
	if !ok || linkDialog == vtui.Container(configurator) {
		t.Fatalf("Assign on a link opened %T", vtui.FrameManager.GetTopFrame())
	}
	vtui.FrameManager.GetTopFrame().ProcessKey(ctrlAltKey('Q'))
	msg, text := topMessage(t)
	if !strings.Contains(text, `command "Test.Q"`) {
		t.Fatalf("Ctrl+Alt+Q did not name its command: %q", text)
	}
	testutil.ClickDialogButton(t, msg, "Take")
	testutil.ClickDialogButton(t, linkDialog, "Save")

	if row := selectLink(`Link "Logs"`); row.Key != "Ctrl+Alt+Q" {
		t.Fatalf("Logs row key = %q after the link dialog, want Ctrl+Alt+Q", row.Key)
	}
	if got := manager.GetAction("Shell", "CtrlAltQ"); got != "Test.Q" {
		t.Fatalf("the live table lost Ctrl+Alt+Q before Save: %q", got)
	}

	selectLink(`Link "Docs"`)
	buttons["Unbind"].OnClick()
	confirm, ok := vtui.FrameManager.GetTopFrame().(vtui.Container)
	if !ok {
		t.Fatalf("confirmation = %T", vtui.FrameManager.GetTopFrame())
	}
	testutil.ClickDialogButton(t, confirm, "Ok")
	if row := selectLink(`Link "Docs"`); row.Key != "" {
		t.Fatalf("Docs row key = %q after Unbind", row.Key)
	}
	if saved, _ := panel.LoadDriveBookmarks(path); saved[0].GlobalHotkey != "CtrlAltD" || saved[1].GlobalHotkey != "" {
		t.Fatalf("the links file changed before Save: %+v", saved)
	}

	buttons["Save"].OnClick()
	saved, err := panel.LoadDriveBookmarks(path)
	if err != nil || len(saved) != 2 || saved[0].GlobalHotkey != "" || saved[1].GlobalHotkey != "CtrlAltQ" {
		t.Fatalf("links after Save = %+v, %v", saved, err)
	}
	if got := manager.Bindings["Shell"]["CtrlAltQ"]; got != "None" {
		t.Fatalf("Shell Ctrl+Alt+Q after Save = %q, want None", got)
	}
}

// A command given a key a link has in the panels' areas would never get it
// there: the key router asks the links first. So the user is asked whether
// to take the key from the link or to choose another. In the editor the link
// is never asked, and the key is bound at once.
func TestCommandGivenALinkKeyAsksFirst(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(120, 40)
	vtui.FrameManager.Init(screen)
	hm := keymap.NewHotkeyManager("")
	store := &linkStore{links: []panel.DriveBookmark{{Name: "Docs", Path: "/docs", GlobalHotkey: "CtrlAltD"}}}
	completed := 0
	onComplete := func() { completed++ }

	pushHotkeyAssign(hm, store, "Test.Action", "Common", onComplete)
	vtui.FrameManager.GetTopFrame().ProcessKey(ctrlAltKey('D'))
	question := vtui.FrameManager.GetTopFrame()
	_, text := topMessage(t)
	if !strings.Contains(text, `link "Docs"`) || !strings.Contains(text, "Ctrl+Alt+D") {
		t.Fatalf("the question does not name the link: %q", text)
	}
	// Esc gives the key up, as Esc in the key window does.
	question.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE})
	testutil.DrainUITasks()
	if !question.IsDone() {
		t.Fatal("Esc did not close the question")
	}
	if _, ok := vtui.FrameManager.GetTopFrame().(*dialog.HotkeyAssignFrame); ok {
		t.Fatal("Esc on the question opened the key window again")
	}
	if hm.Bindings["Common"]["CtrlAltD"] != "" || completed != 0 || store.links[0].GlobalHotkey != "CtrlAltD" {
		t.Fatal("Esc on the question changed something")
	}

	pushHotkeyAssign(hm, store, "Test.Action", "Common", onComplete)
	vtui.FrameManager.GetTopFrame().ProcessKey(ctrlAltKey('D'))
	msg, _ := topMessage(t)
	testutil.ClickDialogButton(t, msg, "Choose another")
	testutil.DrainUITasks()
	if _, ok := vtui.FrameManager.GetTopFrame().(*dialog.HotkeyAssignFrame); !ok {
		t.Fatalf("Choose another left %T on top, want the key window", vtui.FrameManager.GetTopFrame())
	}
	if hm.Bindings["Common"]["CtrlAltD"] != "" || completed != 0 || store.links[0].GlobalHotkey != "CtrlAltD" {
		t.Fatal("Choose another changed something")
	}

	vtui.FrameManager.GetTopFrame().ProcessKey(ctrlAltKey('D'))
	msg, _ = topMessage(t)
	testutil.ClickDialogButton(t, msg, "Take")
	if got := hm.Bindings["Common"]["CtrlAltD"]; got != "Test.Action" || completed != 1 {
		t.Fatalf("after Take Common Ctrl+Alt+D = %q, completed %d", got, completed)
	}
	if store.links[0].GlobalHotkey != "" {
		t.Fatalf("the link kept %q", store.links[0].GlobalHotkey)
	}

	store.links[0].GlobalHotkey = "CtrlAltD"
	pushHotkeyAssign(hm, store, "Test.Action", "Editor", onComplete)
	vtui.FrameManager.GetTopFrame().ProcessKey(ctrlAltKey('D'))
	if got := hm.Bindings["Editor"]["CtrlAltD"]; got != "Test.Action" || completed != 2 {
		t.Fatalf("Editor Ctrl+Alt+D = %q, completed %d: the editor asked", got, completed)
	}
}
