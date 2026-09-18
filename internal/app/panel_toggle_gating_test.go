package app

import (
	"testing"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/vtui"
)

// TestPanelToggle_CtrlOBelongsToRunningProgram_Issue249 pins the arbitration
// of Ctrl+O between f4 and whatever runs in the built-in terminal.
//
// The panel toggle used to be gated by NoAltScreenApp, which hands the key to
// the program only while it is on the alternate screen. mc's own Ctrl+O leaves
// the alternate screen to show its subshell, so the very next Ctrl+O was
// claimed by f4: the panels came back and mc was left running, invisible,
// behind them. Far, far2l and mc all treat the panel toggle as a prompt-level
// key -- a program that owns the terminal owns every key it is sent -- which
// is what NoTerminalApp expresses.
func TestPanelToggle_CtrlOBelongsToRunningProgram_Issue249(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	pf.ShellMode = terminal.ShellModeOwn
	pf.ShowPanels = false
	vtui.FrameManager.Push(pf)

	if keymap.GlobalHotkeysMgr == nil {
		keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	}

	// A full-screen program: the key is its own, as before.
	pf.TermView.UseAltScreen = true
	pf.Executing = false
	if got := keymap.GlobalHotkeysMgr.GetAction("Terminal", "CtrlO"); got != "" {
		t.Errorf("Terminal CtrlO with an alt-screen program: got %q, want empty (must reach the program)", got)
	}

	// The same program after its own Ctrl+O: off the alternate screen, still
	// running. This is the case that used to steal the key from mc.
	pf.TermView.UseAltScreen = false
	pf.Executing = true
	if got := keymap.GlobalHotkeysMgr.GetAction("Terminal", "CtrlO"); got != "" {
		t.Errorf("Terminal CtrlO with a running program off the alt screen: got %q, want empty (must reach the program)", got)
	}

	// Back at the shell prompt: f4 takes the key and shows the panels.
	pf.Executing = false
	if got := keymap.GlobalHotkeysMgr.GetAction("Terminal", "CtrlO"); got != "Panel.Toggle" {
		t.Errorf("Terminal CtrlO at the idle prompt: got %q, want Panel.Toggle", got)
	}

	// With the panels up the toggle is unconditional: nothing is in the way.
	pf.ShowPanels = true
	pf.Executing = true
	if got := keymap.GlobalHotkeysMgr.GetAction("Shell", "CtrlO"); got != "Panel.Toggle" {
		t.Errorf("Shell CtrlO with the panels shown: got %q, want Panel.Toggle", got)
	}
}
