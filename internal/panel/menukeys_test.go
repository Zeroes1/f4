package panel

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func keyEvent(vk uint16, state vtinput.ControlKeyState) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: state}
}

func TestIsAddItemKey(t *testing.T) {
	cases := []struct {
		name string
		e    *vtinput.InputEvent
		want bool
	}{
		{"Ins", keyEvent(vtinput.VK_INSERT, 0), true},
		{"Ctrl+N", keyEvent(vtinput.VK_N, vtinput.LeftCtrlPressed), true},
		{"RightCtrl+N", keyEvent(vtinput.VK_N, vtinput.RightCtrlPressed), true},
		{"N", keyEvent(vtinput.VK_N, 0), false},
		{"Ctrl+Ins", keyEvent(vtinput.VK_INSERT, vtinput.LeftCtrlPressed), false},
		{"Shift+Ins", keyEvent(vtinput.VK_INSERT, vtinput.ShiftPressed), false},
		{"Ctrl+Shift+N", keyEvent(vtinput.VK_N, vtinput.LeftCtrlPressed|vtinput.ShiftPressed), false},
		{"Ctrl+Alt+N", keyEvent(vtinput.VK_N, vtinput.LeftCtrlPressed|vtinput.LeftAltPressed), false},
		{"Ins key up", &vtinput.InputEvent{Type: vtinput.KeyEventType, VirtualKeyCode: vtinput.VK_INSERT}, false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := isAddItemKey(c.e); got != c.want {
			t.Errorf("%s: isAddItemKey = %v, want %v", c.name, got, c.want)
		}
	}
}

// Ctrl+N stores the active panel's directory in the slot under the cursor,
// exactly as Ins does, for keyboards without an Insert key (far2l#3645).
func TestBookmarksDialog_CtrlNSavesCurrentDir(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := &PanelsFrame{ActiveIdx: 0, LastW: 80, LastH: 25}
	pf.Panels[0] = &FileSystemPanel{Vfs: vfs.NewNullVFS(0)}
	wantPath := pf.Panels[0].(*FileSystemPanel).Vfs.GetPath()
	d := &BookmarksDialog{
		Pf:   pf,
		File: filepath.Join(t.TempDir(), "bookmarks.ini"),
		Set:  BookmarkSet{},
	}
	d.open(3, nil)
	defer vtui.FrameManager.Pop()

	d.menu.ProcessKey(keyEvent(vtinput.VK_N, vtinput.LeftCtrlPressed|vtinput.ShiftPressed))
	if !d.Set[3].IsEmpty() {
		t.Fatalf("Ctrl+Shift+N filled slot 3: %#v", d.Set[3])
	}
	d.menu.ProcessKey(keyEvent(vtinput.VK_N, vtinput.LeftCtrlPressed))
	if got := d.Set[3]; got != (Bookmark{Path: wantPath}) {
		t.Fatalf("Ctrl+N stored %#v in slot 3, want %q", got, wantPath)
	}
}

// Ctrl+N in the file associations list opens the "new association" dialog,
// exactly as Ins does.
func TestAssociationsEditor_CtrlNAddsAssociation(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	s := &AssocEditorState{
		Pf:         &PanelsFrame{},
		SourcePath: filepath.Join(t.TempDir(), "associations.ini"),
		Items:      []FileAssoc{{Mask: "*.txt", Description: "Text"}},
	}
	s.openList(0)
	list, ok := vtui.FrameManager.GetTopFrame().(*UserMenuFrame)
	if !ok {
		t.Fatalf("associations list not on top: %T", vtui.FrameManager.GetTopFrame())
	}
	list.ProcessKey(keyEvent(vtinput.VK_N, vtinput.LeftCtrlPressed))
	settleFrames(t)

	top, ok := vtui.FrameManager.GetTopFrame().(interface{ GetTitle() string })
	if !ok {
		t.Fatalf("Ctrl+N did not open a dialog: %T", vtui.FrameManager.GetTopFrame())
	}
	if want := " " + i18n.Msg("FileAssoc.NewTitle") + " "; top.GetTitle() != want {
		t.Fatalf("Ctrl+N opened %q, want %q", top.GetTitle(), want)
	}
}

// Ctrl+N in the drive menu opens the named-link editor for a new entry,
// exactly as Ins does.
func TestDriveMenu_CtrlNOpensBookmarkEditor(t *testing.T) {
	cfg := t.TempDir()
	oldUserConfigDir := config.UserConfigDir
	config.UserConfigDir = func() (string, error) { return cfg, nil }
	t.Cleanup(func() { config.UserConfigDir = oldUserConfigDir })
	if err := os.MkdirAll(filepath.Join(cfg, "f4", "settings"), 0o700); err != nil {
		t.Fatal(err)
	}

	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	pf.ShowDriveMenu(1)
	menu := findDriveMenu(t)
	menu.SetSelectPos(0)
	menu.ProcessKey(keyEvent(vtinput.VK_N, vtinput.LeftCtrlPressed))
	settleFrames(t)
	dlg, ok := vtui.FrameManager.GetTopFrame().(*driveBookmarkEditDialog)
	if !ok {
		t.Fatalf("Ctrl+N did not open drive bookmark editor: %T", vtui.FrameManager.GetTopFrame())
	}
	if dlg.pathEdit.GetText() == "" {
		t.Fatal("Ctrl+N did not pre-fill the panel path")
	}
	dlg.ProcessKey(keyEvent(vtinput.VK_ESCAPE, 0))
	settleFrames(t)
}
