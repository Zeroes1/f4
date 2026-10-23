package panel

import (
	"testing"

	"github.com/unxed/f4/internal/keymap"
)

// The key inventory (cmd/f4/key_inventory_test.go): a native key's action
// presses the key, which does in the panels what it does when typed.
func TestPressNativeKeyDoesWhatTheKeyDoes(t *testing.T) {
	pf, _ := livePanels(t)
	pf.CmdLine.Edit.SetText("echo hello")
	if !pf.PressNativeKey("CtrlY") || !pf.CmdLine.IsEmpty() {
		t.Errorf("Ctrl+Y pressed by its action left %q in the command line", pf.CmdLine.Edit.GetText())
	}
	if pf.PressNativeKey("no such key") {
		t.Error("a name that is no key was pressed")
	}
}

func TestNativeKeyEventPutsTheCharacterBack(t *testing.T) {
	for key, want := range map[string]rune{
		"VK_BB":      '=',
		"ShiftVK_BB": '=',
		"VK_BD":      '-',
		"Add":        '+',
		"Subtract":   '-',
		"Z":          'Z',
		"CtrlVK_BB":  0,
		"F7":         0,
	} {
		e := nativeKeyEvent(key)
		if e == nil {
			t.Errorf("%s: no keystroke", key)
			continue
		}
		if e.Char != want {
			t.Errorf("%s: character %q, want %q", key, e.Char, want)
		}
	}
}

// The conditions that say when a native key works follow the state of the
// panels.
func TestNativeKeyConditionsFollowThePanels(t *testing.T) {
	pf, fsp := livePanels(t)
	for _, name := range []string{"FastFind", "AutoFilter", "TempPanel", "PluginPanel", "QuickViewFocused", "PlayerFocused", "AIChatFocused", "RemoteCommand"} {
		if keymap.ConditionTrue(name) {
			t.Errorf("%s holds on a plain file panel", name)
		}
	}
	fsp.FastFindMode = true
	if !keymap.ConditionTrue("FastFind") || keymap.ConditionTrue("AutoFilter") {
		t.Error("the quick search is not told from the autofilter")
	}
	fsp.autoFilterMode = true
	if !keymap.ConditionTrue("AutoFilter") {
		t.Error("the autofilter is not seen")
	}
	fsp.FastFindMode, fsp.autoFilterMode = false, false

	pf.AltPanels[pf.ActiveIdx] = NewQuickViewPanel(fsp)
	if !keymap.ConditionTrue("QuickViewFocused") || !keymap.ConditionTrue("AltPanelFocused") || keymap.ConditionTrue("PlayerFocused") {
		t.Error("a quick view on the active side is not the focused quick view")
	}
	pf.CommandLineFocused = true
	if keymap.ConditionTrue("QuickViewFocused") {
		t.Error("the quick view counts as focused while the command line has the focus")
	}
	pf.CommandLineFocused = false
	pf.AltPanels[pf.ActiveIdx] = nil

	pf.SwitchToVFS(fsp, NewTempPanelVFS(nil, GlobalTempPanelStore, 0))
	if !keymap.ConditionTrue("TempPanel") {
		t.Error("a temporary panel is not seen")
	}
}

// The drive menu's keys reach the menu only while one is open.
func TestPressDriveMenuKeyNeedsAnOpenMenu(t *testing.T) {
	pf, _ := livePanels(t)
	if DriveMenuOpen() || PressDriveMenuKey("Esc") {
		t.Fatal("a key went to a drive menu that is not open")
	}
	pf.ShowDriveMenu(pf.ActiveIdx)
	if !DriveMenuOpen() {
		t.Fatal("the drive menu is not seen as open")
	}
	if !PressDriveMenuKey("Esc") {
		t.Fatal("Esc did not reach the open drive menu")
	}
	if DriveMenuOpen() {
		t.Error("Esc pressed in the drive menu did not close it")
	}
}
