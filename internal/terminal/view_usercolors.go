package terminal

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// userDefaultColors gives the cells a terminal program leaves at the default
// colours (palette index 7 on 0, see hostDefaultColors) the text and
// background colours the user chose for the embedded terminal (f4#2071).
// The emulator keeps SGR 37 and 40 as the same indexes, so white text and a
// black background a program asks for explicitly change with them.
func userDefaultColors(attr uint64, c config.TerminalColors) uint64 {
	if c.HasForeground && attr&vtui.IsFgRGB == 0 && vtui.GetIndexFore(attr) == vtui.GetIndexFore(DefaultTermAttr) {
		attr = vtui.SetRGBFore(attr, c.Foreground)
	}
	if c.HasBackground && attr&vtui.IsBgRGB == 0 && vtui.GetIndexBack(attr) == vtui.GetIndexBack(DefaultTermAttr) {
		attr = vtui.SetRGBBack(attr, c.Background)
	}
	return attr
}

// selectedAttr is how a cell inside the mouse selection is drawn: in the
// selection colours the user chose, or with text and background swapped when
// neither is set. A half left empty keeps the cell's own colour.
func selectedAttr(attr uint64, c config.TerminalColors) uint64 {
	if !c.HasSelectionFg && !c.HasSelectionBg {
		return InvertAttrColors(attr)
	}
	if c.HasSelectionFg {
		attr = vtui.SetRGBFore(attr, c.SelectionForeground)
	}
	if c.HasSelectionBg {
		attr = vtui.SetRGBBack(attr, c.SelectionBackground)
	}
	return attr
}
