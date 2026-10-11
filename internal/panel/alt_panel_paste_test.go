package panel

import (
	"testing"

	"github.com/unxed/vtui"
)

type pastingAltPanel struct {
	*InfoPanel
	pasted int
}

func (p *pastingAltPanel) PasteClipboard() bool { p.pasted++; return true }

// f4#1842: Ctrl+V with a focused panel that has a paste of its own (the AI
// chat) goes to that panel, not to the folder or the command line.
func TestPasteGoesToFocusedAltPanelWithItsOwnPaste(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	pf := NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	src, ok := pf.Panels[pf.ActiveIdx].(*FileSystemPanel)
	if !ok {
		t.Fatalf("active panel is %T", pf.Panels[pf.ActiveIdx])
	}
	alt := &pastingAltPanel{InfoPanel: NewInfoPanel(src)}
	pf.AltPanels[pf.ActiveIdx] = alt
	alt.SetFocus(true)
	if !ActionPasteClipboard(pf) || alt.pasted != 1 {
		t.Fatalf("the paste did not reach the focused panel (pasted %d times)", alt.pasted)
	}
	alt.SetFocus(false)
	alt.pasted = 0
	ActionPasteClipboard(pf)
	if alt.pasted != 0 {
		t.Error("an unfocused panel took the paste")
	}
}
