package cmdline

import (
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Ctrl+Y clears the command line, as in Far (docs/FAR2L_COMPAT.md, K-72;
// unxed/f4#2139).
func TestCommandLine_CtrlYClearsTheLine(t *testing.T) {
	vtui.SetDefaultPalette()
	cl := NewCommandLine("> ")
	cl.SetPosition(0, 0, 40, 0)
	cl.Edit.SetText("abc def")
	cl.Edit.HistoryPos = 3

	handled := cl.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Y,
		ControlKeyState: vtinput.LeftCtrlPressed,
	})
	if !handled || cl.Edit.GetText() != "" {
		t.Fatalf("Ctrl+Y: handled=%v, text %q, want the line cleared", handled, cl.Edit.GetText())
	}
	if cl.Edit.HistoryPos != -1 {
		t.Fatalf("Ctrl+Y left the history position at %d", cl.Edit.HistoryPos)
	}
}

func TestCommandLine_PlainYAndCtrlShiftYAreText(t *testing.T) {
	vtui.SetDefaultPalette()
	cl := NewCommandLine("> ")
	cl.SetPosition(0, 0, 40, 0)
	cl.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: 'y', VirtualKeyCode: vtinput.VK_Y})
	if cl.Edit.GetText() != "y" {
		t.Fatalf("a plain y: %q", cl.Edit.GetText())
	}
	cl.ProcessKey(&vtinput.InputEvent{
		Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_Y,
		ControlKeyState: vtinput.LeftCtrlPressed | vtinput.ShiftPressed,
	})
	if cl.Edit.GetText() != "y" {
		t.Fatalf("Ctrl+Shift+Y changed the line: %q", cl.Edit.GetText())
	}
}
