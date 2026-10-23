package panel

import (
	"strings"

	"github.com/unxed/f4/internal/cmdline"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Native keys are the keys the panels handle themselves rather than through
// the hotkey table: Enter, Tab, the command line's editing keys, the keys of
// the quick view or the player. Each is declared as a NativeKeys entry of an
// action, so the Hotkey Configurator shows it and a feature that picks a key
// can see it is taken (cmd/f4/key_inventory_test.go). The key is the
// definition of what such an action does, so the action presses it. The
// alternative panels that take keys ahead of the hotkey table say which
// (altPanelKeyOwner), and those keys are declared here too.

// PressNativeKey does what key does in the panels, as the key router would
// deliver it: the plugin keys first, then the frame. It reports whether
// anything took the key.
func (pf *PanelsFrame) PressNativeKey(key string) bool {
	e := nativeKeyEvent(key)
	if pf == nil || e == nil {
		return false
	}
	if pf.InterceptPluginKey(e) {
		return true
	}
	return pf.ProcessKey(e)
}

// PressDriveMenuKey does what key does in the drive menu (Alt+F1, Alt+F2),
// when one is open in the active workspace, and reports whether it was.
func PressDriveMenuKey(key string) bool {
	menu := openDriveMenu()
	e := nativeKeyEvent(key)
	if menu == nil || e == nil {
		return false
	}
	menu.ProcessKey(e)
	return true
}

// DriveMenuOpen reports whether a drive menu is open in the active workspace.
func DriveMenuOpen() bool { return openDriveMenu() != nil }

func openDriveMenu() *DriveMenuFrame {
	if vtui.FrameManager == nil {
		return nil
	}
	frames := vtui.FrameManager.GetActiveFrames(vtui.FrameManager.ActiveIdx)
	for i := len(frames) - 1; i >= 0; i-- {
		if menu, ok := frames[i].(*DriveMenuFrame); ok && !menu.IsDone() {
			return menu
		}
	}
	return nil
}

// CompleteCommandLine offers completions for the text of the command line,
// which Tab does when there is no other panel to switch to.
func (pf *PanelsFrame) CompleteCommandLine() bool {
	if pf == nil || pf.CmdLine == nil || pf.CmdLine.IsEmpty() || vtui.FrameManager == nil {
		return false
	}
	menu := cmdline.NewCompletionMenu(pf.CmdLine.Edit)
	if !menu.HasMatches() {
		return false
	}
	vtui.FrameManager.Push(menu)
	return true
}

// nativeKeyEvent is key as a keystroke, or nil when key is not a key in the
// hotkey table's spelling (ParseFarKey makes a keystroke of any word, from its
// first letter). ParseFarKey leaves the character out of the keys it names
// after the virtual key ("VK_BB", "Add"), while some handlers read the
// character -- the player's volume keys -- so it is put back from the key's
// display name when no Ctrl or Alt is held.
func nativeKeyEvent(key string) *vtinput.InputEvent {
	e := keymap.ParseFarKey(key)
	if e == nil || e.VirtualKeyCode == 0 || !strings.EqualFold(keymap.EventToHotkeyString(e), key) {
		return nil
	}
	if e.Char != 0 || e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0 {
		return e
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ADD:
		e.Char = '+'
	case vtinput.VK_SUBTRACT:
		e.Char = '-'
	case vtinput.VK_MULTIPLY:
		e.Char = '*'
	default:
		display := keymap.FormatKeyForUI(key)
		if r := []rune(display[strings.LastIndex(display, "+")+1:]); len(r) == 1 {
			e.Char = r[0]
		}
	}
	return e
}
