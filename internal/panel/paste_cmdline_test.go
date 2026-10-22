package panel

import (
	"testing"

	"github.com/unxed/vtinput"
)

// A paste is text, not typing (docs/FAR2L_COMPAT.md, B-03, B-04, B-06): a line
// break inside a paste into the command line never acts as Enter, so nothing
// of the pasted text runs before the user presses Enter themselves
// (unxed/f4#2155).

func pasteKey(char rune, vk uint16, ctrl bool) *vtinput.InputEvent {
	e := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: char, VirtualKeyCode: vk}
	if ctrl {
		e.ControlKeyState = vtinput.LeftCtrlPressed
	}
	return e
}

// pasteInto sends a bracketed paste of the given key events to the frame and
// returns the text the command line holds afterwards.
func pasteInto(t *testing.T, f *issue2049Fixture, keys ...*vtinput.InputEvent) string {
	t.Helper()
	f.pf.CmdLine.Edit.SetText("")
	pressKey(f.pf, &vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true})
	for _, k := range keys {
		pressKey(f.pf, k)
	}
	pressKey(f.pf, &vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: false})
	return f.pf.CmdLine.Edit.GetText()
}

func TestPasteIntoCommandLineCRIsNotEnter(t *testing.T) {
	f := newIssue2049Fixture(t)
	got := pasteInto(t, f,
		pasteKey('a', vtinput.VK_A, false),
		pasteKey('\r', vtinput.VK_RETURN, false),
		pasteKey('b', vtinput.VK_B, false),
	)
	if got != "a b" {
		t.Fatalf("command line after a paste of a CR b: %q, want %q (the CR ran the first line?)", got, "a b")
	}
}

func TestPasteIntoCommandLineLFIsNotLost(t *testing.T) {
	f := newIssue2049Fixture(t)
	got := pasteInto(t, f,
		pasteKey('a', vtinput.VK_A, false),
		pasteKey(0, vtinput.VK_J, true), // the byte 0x0A arrives as Ctrl+J
		pasteKey('b', vtinput.VK_B, false),
	)
	if got != "a b" {
		t.Fatalf("command line after a paste of a LF b: %q, want %q", got, "a b")
	}
}

func TestPasteIntoCommandLineCRLFIsOneBreak(t *testing.T) {
	f := newIssue2049Fixture(t)
	got := pasteInto(t, f,
		pasteKey('a', vtinput.VK_A, false),
		pasteKey('\r', vtinput.VK_RETURN, false),
		pasteKey(0, vtinput.VK_J, true),
		pasteKey('b', vtinput.VK_B, false),
	)
	if got != "a b" {
		t.Fatalf("command line after a paste of a CR LF b: %q, want %q", got, "a b")
	}
}

func TestPasteIntoCommandLineEndsWithTheMarker(t *testing.T) {
	f := newIssue2049Fixture(t)
	pasteInto(t, f, pasteKey('x', vtinput.VK_X, false))
	// after the paste Enter is Enter again: a typed 'y' lands in the line
	pressKey(f.pf, pasteKey('y', vtinput.VK_Y, false))
	if got := f.pf.CmdLine.Edit.GetText(); got != "xy" {
		t.Fatalf("typing after a paste: %q, want %q", got, "xy")
	}
}
