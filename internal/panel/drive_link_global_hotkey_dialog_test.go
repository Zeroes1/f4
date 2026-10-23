package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// linkDialogResult is what the dialog handed to its caller.
type linkDialogResult struct {
	called, saved bool
	link          DriveBookmark
}

// openLinkDialog shows the link dialog on a fresh frame manager, as the drive
// menu does, with the global hot key field focused.
func openLinkDialog(t *testing.T, initial DriveBookmark, others []DriveBookmark) (*driveBookmarkEditDialog, *linkDialogResult) {
	t.Helper()
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(100, 30)
	vtui.FrameManager.Init(scr)
	result := &linkDialogResult{}
	d := NewDriveBookmarkEditDialog(initial, "/tmp", func(saved bool, link DriveBookmark) {
		result.called, result.saved, result.link = true, saved, link
	})
	d.others = others
	vtui.FrameManager.Push(d)
	d.SetFocusedItem(d.globalEdit)
	return d, result
}

// messageOnTop is the message box the dialog put over itself, or nil.
func messageOnTop(t *testing.T, d *driveBookmarkEditDialog) *vtui.Window {
	t.Helper()
	top := vtui.FrameManager.GetTopFrame()
	if top == vtui.Frame(d) {
		return nil
	}
	w, ok := top.(*vtui.Window)
	if !ok {
		t.Fatalf("top frame is %T", top)
	}
	return w
}

func messageText(w *vtui.Window) string {
	var lines []string
	for _, child := range w.GetChildren() {
		if text, ok := child.(*vtui.Text); ok {
			lines = append(lines, text.GetText())
		}
	}
	return strings.Join(lines, "\n")
}

// f4#2171: the key pressed in the field is the global hot key, shown as
// Ctrl+Alt+T. Alt and a letter is a hot key there, not the jump to a field;
// Delete clears it; Tab leaves the field; a letter alone is explained, not
// typed.
func TestDriveLinkDialogCapturesTheGlobalHotkey(t *testing.T) {
	useHotkeys(t, map[string]map[string]string{})
	d, _ := openLinkDialog(t, DriveBookmark{}, nil)

	if !d.ProcessKey(globalKeyEvent('T', 0, vtinput.LeftCtrlPressed|vtinput.LeftAltPressed)) {
		t.Fatal("Ctrl+Alt+T was not taken by the field")
	}
	if d.globalEdit.Key() != "CtrlAltT" || d.globalEdit.GetText() != "Ctrl+Alt+T" {
		t.Fatalf("field = %q shown as %q, want CtrlAltT shown as Ctrl+Alt+T", d.globalEdit.Key(), d.globalEdit.GetText())
	}
	if msg := messageOnTop(t, d); msg != nil {
		t.Fatalf("a free key asked: %q", messageText(msg))
	}

	// Alt+N would jump to Name anywhere else in the dialog. Here it is the
	// key, and quick search is what it takes it from.
	d.ProcessKey(globalKeyEvent('N', 'n', vtinput.LeftAltPressed))
	if d.GetFocusedItem() != d.globalEdit {
		t.Fatal("Alt+N moved the focus out of the field")
	}
	msg := messageOnTop(t, d)
	if msg == nil || !strings.Contains(messageText(msg), "quick search") {
		t.Fatal("Alt+N did not say it takes the key from the quick search")
	}
	testutil.ClickDialogButton(t, msg, "Take")
	if d.globalEdit.Key() != "AltN" {
		t.Fatalf("field = %q after Take, want AltN", d.globalEdit.Key())
	}

	d.ProcessKey(globalKeyEvent('X', 'x', 0))
	msg = messageOnTop(t, d)
	if msg == nil || !strings.Contains(messageText(msg), "cannot be a global hot key") {
		t.Fatal("a plain letter was not explained")
	}
	testutil.ClickDialogButton(t, msg, "Ok")
	if d.globalEdit.Key() != "AltN" {
		t.Fatalf("a plain letter changed the field to %q", d.globalEdit.Key())
	}

	d.ProcessKey(globalKeyEvent(vtinput.VK_DELETE, 0, 0))
	if d.globalEdit.Key() != "" || d.globalEdit.GetText() != "" {
		t.Fatalf("Delete left %q", d.globalEdit.Key())
	}

	d.ProcessKey(globalKeyEvent(vtinput.VK_TAB, '\t', 0))
	if d.GetFocusedItem() == d.globalEdit {
		t.Fatal("Tab did not leave the field")
	}
}

// A key bound to a command is taken only when the user says so, and only on
// Save: Cancel leaves the command its key.
func TestDriveLinkDialogTakesTheKeyFromACommand(t *testing.T) {
	hm := useHotkeys(t, map[string]map[string]string{"Shell": {"F5": "Test.Copy"}})
	d, result := openLinkDialog(t, DriveBookmark{}, nil)
	d.nameEdit.SetText("Logs")
	d.pathEdit.SetText("/var/log")

	d.ProcessKey(globalKeyEvent(vtinput.VK_F5, 0, 0))
	msg := messageOnTop(t, d)
	if msg == nil || !strings.Contains(messageText(msg), `command "Test.Copy"`) {
		t.Fatal("F5 did not say which command has it")
	}
	testutil.ClickDialogButton(t, msg, "Choose another")
	if d.globalEdit.Key() != "" {
		t.Fatalf("Choose another left %q in the field", d.globalEdit.Key())
	}

	d.ProcessKey(globalKeyEvent(vtinput.VK_F5, 0, 0))
	testutil.ClickDialogButton(t, messageOnTop(t, d), "Take")
	if d.globalEdit.Key() != "F5" {
		t.Fatalf("Take left %q in the field", d.globalEdit.Key())
	}
	if got := hm.Bindings["Shell"]["F5"]; got != "Test.Copy" {
		t.Fatalf("F5 was taken from the command before Save: %q", got)
	}

	d.submit()
	if !result.called || !result.saved || result.link.GlobalHotkey != "F5" {
		t.Fatalf("Save handed back %+v", *result)
	}
	if got := hm.Bindings["Shell"]["F5"]; got != "None" {
		t.Errorf("after Save Shell F5 = %q, want None", got)
	}
}

// A key the link already had is checked on Save too: a command may have been
// bound to it since.
func TestDriveLinkDialogAsksAboutAKeptKeyOnSave(t *testing.T) {
	hm := useHotkeys(t, map[string]map[string]string{"Shell": {"CtrlAltT": "Test.Later"}})
	d, result := openLinkDialog(t, DriveBookmark{Name: "Logs", Path: "/var/log", GlobalHotkey: "CtrlAltT"}, nil)

	d.submit()
	msg := messageOnTop(t, d)
	if msg == nil || !strings.Contains(messageText(msg), `command "Test.Later"`) {
		t.Fatal("Save did not ask about the key a command has now")
	}
	if result.called {
		t.Fatal("Save finished before the answer")
	}
	testutil.ClickDialogButton(t, msg, "Take")
	if !result.saved || result.link.GlobalHotkey != "CtrlAltT" {
		t.Fatalf("Save handed back %+v", *result)
	}
	if got := hm.Bindings["Shell"]["CtrlAltT"]; got != "None" {
		t.Errorf("Shell CtrlAltT = %q, want None", got)
	}
}

// A numbered bookmark's key is answered before the links, so it is refused
// outright, with nothing to take.
func TestDriveLinkDialogRefusesABookmarkKey(t *testing.T) {
	useHotkeys(t, map[string]map[string]string{})
	d, _ := openLinkDialog(t, DriveBookmark{GlobalHotkey: "CtrlAltT"}, nil)
	d.ProcessKey(globalKeyEvent('5', 0, vtinput.RightCtrlPressed))
	msg := messageOnTop(t, d)
	if msg == nil || !strings.Contains(messageText(msg), "folder bookmark 5") {
		t.Fatal("RCtrl+5 was not refused as the bookmark's")
	}
	for _, child := range msg.GetChildren() {
		if b, ok := child.(*vtui.Button); ok && testutil.GetCleanText(b) == "Take" {
			t.Fatal("the bookmark's key was offered to be taken")
		}
	}
	testutil.ClickDialogButton(t, msg, "Ok")
	if d.globalEdit.Key() != "CtrlAltT" {
		t.Fatalf("the field changed to %q", d.globalEdit.Key())
	}
}

// The field takes chords, even with a Shift that some GUI hosts report only
// as its own key; a paste types nothing; the keys that move around a dialog
// are left to it, and the plain cursor keys do nothing.
func TestGlobalHotkeyEditKeys(t *testing.T) {
	e := NewGlobalHotkeyEdit("", 18)
	if e.ProcessKey(globalKeyEvent(vtinput.VK_SHIFT, 0, vtinput.ShiftPressed)) != true {
		t.Fatal("Shift was not taken")
	}
	e.ProcessKey(globalKeyEvent('T', 0, vtinput.LeftCtrlPressed|vtinput.LeftAltPressed))
	if e.Key() != "CtrlAltShiftT" {
		t.Errorf("with Shift held the field holds %q, want CtrlAltShiftT", e.Key())
	}
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, VirtualKeyCode: vtinput.VK_SHIFT})
	e.ProcessKey(globalKeyEvent('T', 0, vtinput.LeftCtrlPressed|vtinput.LeftAltPressed))
	if e.Key() != "CtrlAltT" {
		t.Errorf("after Shift was let go the field holds %q, want CtrlAltT", e.Key())
	}
	e.ProcessKey(globalKeyEvent(vtinput.VK_SHIFT, 0, vtinput.ShiftPressed))
	e.SetFocus(false)
	e.ProcessKey(globalKeyEvent(vtinput.VK_F11, 0, vtinput.LeftCtrlPressed))
	if e.Key() != "CtrlF11" {
		t.Errorf("a Shift held when the field lost the focus stuck: %q", e.Key())
	}

	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true})
	e.ProcessKey(globalKeyEvent('A', 'a', 0))
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.PasteEventType})
	if e.Key() != "CtrlF11" || e.GetText() != "Ctrl+F11" {
		t.Errorf("a paste changed the field to %q (%q)", e.Key(), e.GetText())
	}

	for _, vk := range []uint16{vtinput.VK_RETURN, vtinput.VK_ESCAPE, vtinput.VK_TAB, vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_PRIOR, vtinput.VK_NEXT} {
		if e.ProcessKey(globalKeyEvent(vk, 0, 0)) {
			t.Errorf("the field took %s, which the dialog needs", vtinput.VKString(vk))
		}
	}
	if e.ProcessKey(globalKeyEvent(vtinput.VK_TAB, '\t', vtinput.ShiftPressed)) {
		t.Error("the field took Shift+Tab")
	}
	for _, vk := range []uint16{vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_HOME, vtinput.VK_END, vtinput.VK_INSERT} {
		if !e.ProcessKey(globalKeyEvent(vk, 0, 0)) || e.Key() != "CtrlF11" {
			t.Errorf("%s changed the field to %q", vtinput.VKString(vk), e.Key())
		}
	}
	e.ProcessKey(globalKeyEvent(vtinput.VK_RETURN, '\r', vtinput.LeftCtrlPressed))
	if e.Key() != "CtrlEnter" {
		t.Errorf("Ctrl+Enter gave %q, want CtrlEnter", e.Key())
	}
	e.ProcessKey(globalKeyEvent(vtinput.VK_BACK, '\b', 0))
	if e.Key() != "" {
		t.Errorf("Backspace left %q", e.Key())
	}
}

// Another link with the key gives it up when the edited one is saved.
func TestTakeDriveLinkGlobalKeyClearsTheOtherLinks(t *testing.T) {
	links := []DriveBookmark{
		{Name: "A", Path: "/a", GlobalHotkey: "CtrlAltT"},
		{Name: "B", Path: "/b", GlobalHotkey: "ctrlaltt"},
		{Name: "C", Path: "/c", GlobalHotkey: "CtrlAltC"},
	}
	links = takeDriveLinkGlobalKey(links, 1)
	if links[0].GlobalHotkey != "" || links[1].GlobalHotkey != "ctrlaltt" || links[2].GlobalHotkey != "CtrlAltC" {
		t.Errorf("links after B took the key: %+v", links)
	}
}
