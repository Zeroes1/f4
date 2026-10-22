package panel

import "github.com/unxed/vtinput"

// A paste into the command line is text (docs/FAR2L_COMPAT.md, B-03, B-04,
// B-06; unxed/f4#2155). The terminal sends the pasted bytes between the paste
// markers as ordinary key events, and a CR among them is the Enter key: left
// to the frame it ran the line typed so far, and the rest of the paste went
// on typing into an empty line. While a paste lasts, the keys go straight to
// the command line, which takes them as text, and a line break of any kind
// (CR, LF, CRLF) becomes one space.

// cmdlinePasteKey routes the events of a bracketed paste whose target is the
// command line. It reports whether it took the event.
func (pf *PanelsFrame) cmdlinePasteKey(e *vtinput.InputEvent) bool {
	if pf.CmdLine == nil {
		return false
	}
	if e.Type == vtinput.PasteEventType {
		// Only a paste that lands in the command line: with the panels shown
		// and no fast find running; a paste for a program in the terminal is
		// forwarded elsewhere.
		fsp := pf.GetActivePanel()
		pf.cmdlinePasting = e.PasteStart && pf.ShowPanels && (fsp == nil || !fsp.FastFindMode)
		pf.cmdlinePasteBreak = false
		return false // the markers themselves go the usual way, to the edit line
	}
	if !pf.cmdlinePasting || e.Type != vtinput.KeyEventType {
		return false
	}
	if isPasteLineBreak(e) {
		if e.KeyDown && !pf.cmdlinePasteBreak {
			pf.cmdlinePasteBreak = true
			pf.CmdLine.ProcessKey(&vtinput.InputEvent{
				Type: vtinput.KeyEventType, KeyDown: true, Char: ' ', VirtualKeyCode: vtinput.VK_SPACE,
			})
		}
		return true
	}
	if e.KeyDown {
		pf.cmdlinePasteBreak = false
	}
	pf.CmdLine.ProcessKey(e)
	return true
}

// isPasteLineBreak: the Enter key, a CR or LF character, or the Ctrl+J a
// terminal driver shows for the byte 0x0A.
func isPasteLineBreak(e *vtinput.InputEvent) bool {
	if e.VirtualKeyCode == vtinput.VK_RETURN || e.Char == '\r' || e.Char == '\n' {
		return true
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	return ctrl && e.VirtualKeyCode == vtinput.VK_J
}
