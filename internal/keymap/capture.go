package keymap

import "github.com/unxed/vtui"

// KeyCapturer is a control that takes every key pressed while it has the
// focus, ahead of the key router: a field a key is pressed into to become a hot
// key (panel.GlobalHotkeyEdit, f4#2171). The router hands such a control the
// key before keymap.ini, macro recording, the command palette, the macros, the
// hotkey table or vtui's own keys could act on it.
type KeyCapturer interface {
	CapturesKeys() bool
}

// FocusCapturesKeys follows the focus from frame down to the focused control
// and reports whether something on the way captures keys.
func FocusCapturesKeys(frame vtui.Frame) bool {
	var el any = frame
	for depth := 0; el != nil && depth < 16; depth++ {
		if c, ok := el.(KeyCapturer); ok && c.CapturesKeys() {
			return true
		}
		container, ok := el.(interface{ GetFocusedItem() vtui.UIElement })
		if !ok {
			return false
		}
		next := container.GetFocusedItem()
		if next == nil || any(next) == el {
			return false
		}
		el = next
	}
	return false
}
