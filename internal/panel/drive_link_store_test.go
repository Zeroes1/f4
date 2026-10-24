package panel

import (
	"os"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// f4#2171: a command bound in the panels' areas to a link's global hot key
// never gets it there, as the key router asks the links first. A Ctrl link
// answers the right Ctrl too; an RCtrl link takes the right Ctrl from a
// command on plain Ctrl. The editor and the other areas never ask the links,
// and a link without a name or a path is no link.
func TestDriveLinkBeforeCommand(t *testing.T) {
	links := []DriveBookmark{
		{Name: "Docs", Path: "/docs", GlobalHotkey: "CtrlAltD"},
		{Name: "Right", Path: "/right", GlobalHotkey: "RCtrlAltR"},
		{Name: "", Path: "/nameless", GlobalHotkey: "CtrlAltX"},
	}
	cases := []struct {
		area, key string
		index     int
		ok        bool
	}{
		{"Shell", "CtrlAltD", 0, true},
		{"common", "RCtrlAltD", 0, true},
		{"Shell", "CtrlAltR", 1, true},
		{"Shell", "RCtrlAltR", 1, true},
		{"Editor", "CtrlAltD", 0, false},
		{"Shell", "CtrlAltX", 0, false},
		{"Shell", "CtrlAltQ", 0, false},
		{"Shell", "AltD", 0, false},
	}
	for _, tc := range cases {
		index, ok := DriveLinkBeforeCommand(links, tc.area, tc.key)
		if ok != tc.ok || (ok && index != tc.index) {
			t.Errorf("%s %s: link %d %v, want %d %v", tc.area, tc.key, index, ok, tc.index, tc.ok)
		}
	}
}

// The configurator's draft of the links file is written on Save, and only
// when something in it changed. A link given another link's key takes it
// from that link, as the link dialog said it would.
func TestDriveLinksDraft(t *testing.T) {
	useDriveLinksProfile(t)
	path := DriveBookmarksFilePath()
	if err := SaveDriveBookmarks(path, []DriveBookmark{
		{Name: "Docs", Path: "/docs", GlobalHotkey: "CtrlAltD"},
		{Name: "Logs", Path: "/var/log"},
	}); err != nil {
		t.Fatal(err)
	}
	draft, err := LoadDriveLinksDraft()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := draft.Save(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a draft with no changes wrote the file: %v", err)
	}

	logs := draft.DriveLinks()[1]
	logs.GlobalHotkey = "CtrlAltD"
	if err := PutDriveLink(draft, 1, logs); err != nil {
		t.Fatal(err)
	}
	if got := draft.DriveLinks(); got[0].GlobalHotkey != "" || got[1].GlobalHotkey != "CtrlAltD" {
		t.Fatalf("after Logs took Ctrl+Alt+D the draft holds %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("the draft wrote the file before Save")
	}
	if err := draft.Save(); err != nil {
		t.Fatal(err)
	}
	saved, err := LoadDriveBookmarks(path)
	if err != nil || len(saved) != 2 || saved[0].GlobalHotkey != "" || saved[1].GlobalHotkey != "CtrlAltD" {
		t.Fatalf("saved links = %+v, %v", saved, err)
	}
}

// The link dialog the configurator opens checks the key against the
// configurator's draft of the hotkey table, not the live one: a key the user
// unbound in the draft is free, one bound only in the draft is not. Taking a
// key from a command unbinds it in the draft and writes nothing; the
// configurator saves its draft itself.
func TestEditDriveLinkInChecksTheDraft(t *testing.T) {
	hm := useHotkeys(t, map[string]map[string]string{"Shell": {"F5": "Test.Copy"}})
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)

	draft := hm.CloneForEdit()
	draft.Bind("Shell", "F5", "None")
	draft.Bind("Shell", "F6", "Test.Draft")
	links := []DriveBookmark{
		{Name: "Logs", Path: "/var/log"},
		{Name: "Docs", Path: "/docs", GlobalHotkey: "CtrlAltD"},
	}
	var done []DriveBookmark
	EditDriveLinkIn(draft, links, 0, func(link DriveBookmark) { done = append(done, link) })
	d, ok := vtui.FrameManager.GetTopFrame().(*driveBookmarkEditDialog)
	if !ok {
		t.Fatalf("top frame = %T, want the link dialog", vtui.FrameManager.GetTopFrame())
	}
	if d.GetFocusedItem() != d.globalEdit {
		t.Fatal("the global hot key field does not have the focus")
	}

	d.ProcessKey(globalKeyEvent(vtinput.VK_F5, 0, 0))
	if msg := messageOnTop(t, d); msg != nil {
		t.Fatalf("F5, unbound in the draft, asked: %q", messageText(msg))
	}
	if d.globalEdit.Key() != "F5" {
		t.Fatalf("field = %q, want F5", d.globalEdit.Key())
	}

	d.ProcessKey(globalKeyEvent('D', 0, vtinput.LeftCtrlPressed|vtinput.LeftAltPressed))
	msg := messageOnTop(t, d)
	if msg == nil || !strings.Contains(messageText(msg), `link "Docs"`) {
		t.Fatal("Ctrl+Alt+D did not name the other link")
	}
	testutil.ClickDialogButton(t, msg, "Choose another")

	d.ProcessKey(globalKeyEvent(vtinput.VK_F6, 0, 0))
	msg = messageOnTop(t, d)
	if msg == nil || !strings.Contains(messageText(msg), `command "Test.Draft"`) {
		t.Fatal("F6, bound in the draft, did not name its command")
	}
	testutil.ClickDialogButton(t, msg, "Take")
	d.submit()

	if len(done) != 1 || done[0].Name != "Logs" || done[0].GlobalHotkey != "F6" {
		t.Fatalf("the dialog handed back %+v", done)
	}
	if got := draft.Bindings["Shell"]["F6"]; got != "None" {
		t.Errorf("draft Shell F6 = %q, want None", got)
	}
	if got := hm.Bindings["Shell"]["F6"]; got != "" {
		t.Errorf("the live table got Shell F6 = %q", got)
	}
	if _, err := os.Stat(hm.IniPath); !os.IsNotExist(err) {
		t.Errorf("the hotkeys file was written: %v", err)
	}
}
