package app

import (
	"testing"

	"github.com/unxed/f4/internal/history"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// New File must share the compact input geometry of select/deselect group,
// including fixed height during screen resizing and both corner drag directions.
func TestNewFileDialogLayout(t *testing.T) {
	checkMaskDialogLayout(t, "File.New")
}

func TestNewFileDialogHistoryAndPathHints(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	theme.SetDefaultF4Palette()
	store := useStubHistory(t)
	store[history.NewEditHistoryID] = []string{"previous.txt"}
	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	actionNewFile(pf)
	dlg := vtui.FrameManager.GetTopFrame().(vtui.Container)
	defer vtui.FrameManager.Pop()
	edit := findHistoryEdit(t, dlg, history.NewEditHistoryID)
	// The prompt opens on the last name entered, selected (#2124).
	if edit.GetText() != "previous.txt" || !edit.PathHintsEnabled || !edit.ShowHistoryButton {
		t.Fatalf("initial input %q, want the last name; path hints and history button must be preserved", edit.GetText())
	}
	edit.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		Char: 'n', VirtualKeyCode: vtinput.VK_N})
	if edit.GetText() != "n" {
		t.Fatalf("typing over the last name gave %q, want %q", edit.GetText(), "n")
	}
	edit.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true,
		VirtualKeyCode: vtinput.VK_E, ControlKeyState: vtinput.LeftCtrlPressed})
	if edit.GetText() != "previous.txt" {
		t.Fatal("Ctrl+E must retrieve the previous file name")
	}
}
