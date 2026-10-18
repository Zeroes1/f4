package terminal

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestShowDrawsTheTerminalColoursOfTheSettings(t *testing.T) {
	old := config.App
	t.Cleanup(func() { config.App = old })
	config.App.TerminalForeground = "#112233"
	config.App.TerminalBackground = "#445566"

	tv := NewTerminalView(10, 3)
	tv.SetPosition(0, 0, 9, 2)
	tv.PutChar('x', DefaultTermAttr)
	tv.PutChar('r', DefaultTermAttr)
	tv.Lines[0][1].Attributes = vtui.SetIndexBoth(0, 1, 4) // red on blue, as SGR 31;44 leaves it
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(10, 3)
	tv.Show(scr)

	for _, at := range [][2]int{{0, 0}, {5, 2}} {
		got := scr.GetCell(at[0], at[1]).Attributes
		if got&vtui.IsFgRGB == 0 || vtui.GetRGBFore(got) != 0x112233 || got&vtui.IsBgRGB == 0 || vtui.GetRGBBack(got) != 0x445566 {
			t.Errorf("cell %v = %#x, want #112233 on #445566", at, got)
		}
	}
	if got := scr.GetCell(1, 0).Attributes; got != vtui.SetIndexBoth(0, 1, 4) {
		t.Errorf("a cell a program coloured itself changed: %#x", got)
	}

	// The mirror of a host console keeps the host terminal's own defaults.
	tv.DefaultColors = true
	tv.Show(scr)
	if got := scr.GetCell(0, 0).Attributes; got&vtui.ForegroundDefault == 0 || got&vtui.BackgroundDefault == 0 {
		t.Errorf("host console mirror = %#x, want the terminal's default colours", got)
	}
}

func TestSelectionUsesTheSelectionColoursOfTheSettings(t *testing.T) {
	cell := vtui.SetIndexBoth(0, 2, 4)
	if got := selectedAttr(cell, config.TerminalColors{}); got != InvertAttrColors(cell) {
		t.Errorf("no selection colours set: %#x, want the swapped colours %#x", got, InvertAttrColors(cell))
	}
	both := config.TerminalColors{SelectionForeground: 0x010203, HasSelectionFg: true, SelectionBackground: 0xA0B0C0, HasSelectionBg: true}
	got := selectedAttr(cell, both)
	if vtui.GetRGBFore(got) != 0x010203 || vtui.GetRGBBack(got) != 0xA0B0C0 {
		t.Errorf("both selection colours set: %#x", got)
	}
	onlyBg := config.TerminalColors{SelectionBackground: 0xA0B0C0, HasSelectionBg: true}
	got = selectedAttr(cell, onlyBg)
	if got&vtui.IsFgRGB != 0 || vtui.GetIndexFore(got) != 2 || vtui.GetRGBBack(got) != 0xA0B0C0 {
		t.Errorf("only the selection background set: %#x, want text index 2 on #A0B0C0", got)
	}
}
