package editor

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtinput"
)

func persistentKey(ev *EditorView, vk uint16, state vtinput.ControlKeyState, ch rune) {
	ev.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk, ControlKeyState: state, Char: ch})
}

func selectedText(t *testing.T, ev *EditorView) string {
	t.Helper()
	start, end := ev.GetSelectionRange()
	if !ev.SelActive || end <= start {
		return ""
	}
	data, err := ev.Pt.GetRange(start, end-start)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// f4#1729 (gchet): with persistent blocks a selected block stays when the
// cursor moves away, typing works at the cursor and leaves it, edits before
// it move it along, and Ctrl+P copies it to the cursor.
func TestPersistentBlockStaysWhileTheCursorMoves_Issue1729(t *testing.T) {
	old := config.App.EditorPersistentBlocks
	t.Cleanup(func() { config.App.EditorPersistentBlocks = old })
	config.App.EditorPersistentBlocks = true

	ev := newFAR3Editor(t, "hello world\nsecond line\n")
	for i := 0; i < 5; i++ {
		persistentKey(ev, vtinput.VK_RIGHT, vtinput.ShiftPressed, 0)
	}
	persistentKey(ev, vtinput.VK_DOWN, 0, 0)
	if got := selectedText(t, ev); got != "hello" {
		t.Fatalf("after Down the block is %q, want %q", got, "hello")
	}

	persistentKey(ev, 0, 0, 'X')
	if got, want := ev.GetText(), "hello world\nseconXd line\n"; got != want {
		t.Fatalf("typing gave %q, want %q", got, want)
	}
	if got := selectedText(t, ev); got != "hello" {
		t.Fatalf("after typing below it the block is %q, want %q", got, "hello")
	}

	ev.CursorLine, ev.CursorPos = 0, 0
	persistentKey(ev, 0, 0, 'Y')
	if got := selectedText(t, ev); got != "hello" {
		t.Fatalf("after typing before it the block is %q, want it moved along", got)
	}

	ev.CursorLine, ev.CursorPos = 1, 0
	ev.CopySelectionToCursor()
	if got, want := ev.GetText(), "Yhello world\nhelloseconXd line\n"; got != want {
		t.Fatalf("Ctrl+P gave %q, want %q", got, want)
	}

	ev.ClearSelection()
	if ev.SelActive {
		t.Fatal("Ctrl+U kept the block")
	}
}

// Without the option the selection goes away as before.
func TestSelectionWithoutPersistentBlocksGoesWithTheCursor(t *testing.T) {
	old := config.App.EditorPersistentBlocks
	t.Cleanup(func() { config.App.EditorPersistentBlocks = old })
	config.App.EditorPersistentBlocks = false

	ev := newFAR3Editor(t, "hello world\nsecond line\n")
	for i := 0; i < 5; i++ {
		persistentKey(ev, vtinput.VK_RIGHT, vtinput.ShiftPressed, 0)
	}
	persistentKey(ev, vtinput.VK_DOWN, 0, 0)
	if ev.SelActive {
		t.Fatalf("the selection %q stayed after Down", selectedText(t, ev))
	}
}

// A click elsewhere moves the cursor and leaves a persistent block; a drag
// makes a new one.
func TestPersistentBlockSurvivesAClick_Issue1729(t *testing.T) {
	old := config.App.EditorPersistentBlocks
	t.Cleanup(func() { config.App.EditorPersistentBlocks = old })
	config.App.EditorPersistentBlocks = true

	ev := newFAR3Editor(t, "hello world\nsecond line\n")
	for i := 0; i < 5; i++ {
		persistentKey(ev, vtinput.VK_RIGHT, vtinput.ShiftPressed, 0)
	}
	persistentKey(ev, vtinput.VK_DOWN, 0, 0)

	mouse := func(x, y int, state uint32, flags uint32) {
		ev.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: state != 0,
			MouseX: int16(x), MouseY: int16(y), ButtonState: state, MouseEventFlags: flags})
	}
	_, y1, _, _ := ev.GetPosition()
	mouse(2, y1+2, vtinput.FromLeft1stButtonPressed, 0)
	mouse(2, y1+2, 0, 0)
	if ev.CursorLine != 1 || ev.CursorPos != 2 {
		t.Errorf("the click put the cursor at %d:%d, want 1:2", ev.CursorLine, ev.CursorPos)
	}
	if got := selectedText(t, ev); got != "hello" {
		t.Fatalf("after a click the block is %q, want %q", got, "hello")
	}
}
