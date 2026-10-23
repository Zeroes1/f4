package panel

import (
	"strings"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// GlobalHotkeyEdit is the field of a link's global hot key (f4#2171): the key
// pressed in it becomes the hot key and is shown as Ctrl+Alt+T; nothing is
// typed into it. Delete and Backspace clear it. Tab, Enter, Esc and the plain
// Up, Down, PgUp and PgDn are left to the dialog, so the field is left as any
// other; the plain Left, Right, Home, End and Ins do nothing.
type GlobalHotkeyEdit struct {
	*vtui.Edit
	key string
	// shiftHeld: some GUI hosts leave ShiftPressed out of the key that
	// follows Shift, as the hotkey assignment dialog found.
	shiftHeld bool
	// pasting swallows the characters of a paste, which arrive as keys.
	pasting bool
	// OnKey gets each key pressed in the field, in the spelling of
	// keymap.EventToHotkeyString, or "" for Delete, and decides whether the
	// field takes it (SetKey). Nil takes every key.
	OnKey func(key string)
}

func NewGlobalHotkeyEdit(key string, width int) *GlobalHotkeyEdit {
	e := &GlobalHotkeyEdit{Edit: vtui.NewEdit(0, 0, max(1, width), "")}
	e.SetKey(key)
	return e
}

// CapturesKeys asks the application's key router to hand the focused field
// every key before it acts on any (keymap.KeyCapturer): Ctrl+., Ctrl+Shift+P,
// a macro's key or Alt with a digit may be the hot key the user wants, and the
// router would otherwise take it first.
func (e *GlobalHotkeyEdit) CapturesKeys() bool { return true }

// Key is the hot key the field holds, in the spelling it was given in.
func (e *GlobalHotkeyEdit) Key() string { return e.key }

// SetKey puts key into the field, shown the way menus show keys.
func (e *GlobalHotkeyEdit) SetKey(key string) {
	e.key = strings.TrimSpace(key)
	e.SetText(keymap.FormatKeyForUI(e.key))
}

func (e *GlobalHotkeyEdit) SetFocus(focused bool) {
	if !focused {
		e.shiftHeld = false
	}
	e.Edit.SetFocus(focused)
}

// Show keeps the start of the key in view instead of scrolling to a caret
// after it.
func (e *GlobalHotkeyEdit) Show(scr *vtui.ScreenBuf) {
	e.Edit.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_HOME})
	e.Edit.Show(scr)
}

func (e *GlobalHotkeyEdit) ProcessKey(ev *vtinput.InputEvent) bool {
	if ev == nil {
		return false
	}
	switch ev.Type {
	case vtinput.PasteEventType:
		e.pasting = ev.PasteStart
		return true
	case vtinput.KeyEventType:
	default:
		return e.Edit.ProcessKey(ev)
	}
	switch ev.VirtualKeyCode {
	case vtinput.VK_SHIFT, vtinput.VK_LSHIFT, vtinput.VK_RSHIFT:
		e.shiftHeld = ev.KeyDown
		return true
	}
	if !ev.KeyDown || e.pasting {
		return true
	}
	const ctrlAlt = vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.LeftAltPressed | vtinput.RightAltPressed
	shift := ev.ControlKeyState&vtinput.ShiftPressed != 0 || e.shiftHeld
	noCtrlAlt := ev.ControlKeyState&ctrlAlt == 0
	plain := noCtrlAlt && !shift
	switch ev.VirtualKeyCode {
	case vtinput.VK_TAB:
		if noCtrlAlt {
			return false
		}
	case vtinput.VK_RETURN, vtinput.VK_ESCAPE, vtinput.VK_UP, vtinput.VK_DOWN, vtinput.VK_PRIOR, vtinput.VK_NEXT:
		if plain {
			return false
		}
	case vtinput.VK_LEFT, vtinput.VK_RIGHT, vtinput.VK_HOME, vtinput.VK_END, vtinput.VK_INSERT:
		if plain {
			return true
		}
	case vtinput.VK_DELETE, vtinput.VK_BACK:
		if plain {
			e.offer("")
			return true
		}
	case vtinput.VK_CONTROL, vtinput.VK_LCONTROL, vtinput.VK_RCONTROL,
		vtinput.VK_MENU, vtinput.VK_LMENU, vtinput.VK_RMENU,
		vtinput.VK_LWIN, vtinput.VK_RWIN,
		vtinput.VK_CAPITAL, vtinput.VK_NUMLOCK, vtinput.VK_SCROLL:
		return true
	}
	pressed := *ev
	if e.shiftHeld {
		pressed.ControlKeyState |= vtinput.ShiftPressed
	}
	if key := keymap.EventToHotkeyString(&pressed); key != "" {
		e.offer(key)
	}
	return true
}

func (e *GlobalHotkeyEdit) offer(key string) {
	if e.OnKey != nil {
		e.OnKey(key)
		return
	}
	e.SetKey(key)
}
