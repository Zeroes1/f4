package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

// K-74 and K-75 of docs/FAR2L_COMPAT.md (unxed/f4#2141): Ctrl+J puts the name
// of the file under the cursor into the command line like Ctrl+Enter, and
// Ctrl+Shift+Enter puts the name from the passive panel -- it never runs the
// command line.

func namesFixture(t *testing.T) *issue2049Fixture {
	t.Helper()
	f := newIssue2049Fixture(t)
	f.fsp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "active one.txt"}}}
	f.fsp.CursorIdx = 0
	passive, ok := f.pf.Panels[1].(*FileSystemPanel)
	if !ok {
		t.Fatal("the second panel is not a file panel")
	}
	passive.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "passive.txt"}}}
	passive.CursorIdx = 0
	f.pf.ActiveIdx = 0
	return f
}

func ctrlKey(vk uint16, shift bool) *vtinput.InputEvent {
	e := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: vtinput.LeftCtrlPressed}
	if shift {
		e.ControlKeyState |= vtinput.ShiftPressed
	}
	return e
}

func TestCtrlJInsertsTheNameOfTheFileUnderTheCursor(t *testing.T) {
	f := namesFixture(t)
	f.pf.CmdLine.Edit.SetText("ls ")
	pressKey(f.pf, ctrlKey(vtinput.VK_J, false))
	if got := f.pf.CmdLine.Edit.GetText(); got != "ls 'active one.txt' " {
		t.Fatalf("command line after Ctrl+J: %q, want the quoted name of the active file and a space", got)
	}
}

func TestCtrlShiftEnterInsertsThePassiveNameAndRunsNothing(t *testing.T) {
	f := namesFixture(t)
	f.pf.CmdLine.Edit.SetText("true ")
	pressKey(f.pf, ctrlKey(vtinput.VK_RETURN, true))
	got := f.pf.CmdLine.Edit.GetText()
	if got == "" || !strings.HasPrefix(got, "true ") {
		t.Fatalf("Ctrl+Shift+Enter ran or cleared the command line: %q", got)
	}
	if got != "true passive.txt " {
		t.Fatalf("command line after Ctrl+Shift+Enter: %q, want the name of the passive file", got)
	}
}
