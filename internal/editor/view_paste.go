package editor

import "github.com/unxed/vtinput"

// collectPasteKey adds the key of a bracketed paste to the text being
// collected. The pasted bytes arrive as key events: a character is text, and
// a line break arrives as the Enter key (CR), as a LF character or as the
// Ctrl+J a terminal driver shows for the byte 0x0A. Each of CR, LF and CRLF
// is one line break in the text (docs/FAR2L_COMPAT.md, B-06; unxed/f4#2156);
// before, CR was ignored and a LF without a character was dropped, so a paste
// came out as one line.
func (ev *EditorView) collectPasteKey(e *vtinput.InputEvent) {
	switch pasteBreakOf(e) {
	case '\r':
		ev.pasteBuffer = append(ev.pasteBuffer, '\n')
		ev.pasteAfterCR = true
		return
	case '\n':
		if !ev.pasteAfterCR {
			ev.pasteBuffer = append(ev.pasteBuffer, '\n')
		}
		ev.pasteAfterCR = false
		return
	}
	ev.pasteAfterCR = false
	if e.Char != 0 {
		ev.pasteBuffer = append(ev.pasteBuffer, e.Char)
	}
}

// pasteBreakOf returns '\r' for the CR (Enter) key, '\n' for a LF, and 0 for
// anything that is not a line break.
func pasteBreakOf(e *vtinput.InputEvent) rune {
	if e.Char == '\n' {
		return '\n'
	}
	if e.Char == '\r' || e.VirtualKeyCode == vtinput.VK_RETURN {
		return '\r'
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	if ctrl && e.VirtualKeyCode == vtinput.VK_J {
		return '\n'
	}
	return 0
}
