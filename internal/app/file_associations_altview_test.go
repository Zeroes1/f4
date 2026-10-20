package app

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/panel"
)

// Alt+F3 over a file runs the AltView command of the matching association
// (far2l: the second way to view a file, f4#1741).
func TestActionViewFileHex_AltViewAssociationRuns(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocView: "eog !.!", panel.AssocAltView: "feh !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocView: true, panel.AssocAltView: true},
		},
	})

	pf, pty := setupPanelWithFile(t, "pic.png")
	actionViewFileHex(pf)

	written := string(pty.Written)
	if !strings.Contains(written, "feh pic.png") {
		t.Errorf("Alt+F3 did not run the AltView command; PTY got %q", written)
	}
	if strings.Contains(written, "eog") {
		t.Errorf("Alt+F3 ran the View command; PTY got %q", written)
	}
}

// F3 does not run the AltView slot: the slots are not interchangeable.
func TestActionViewFile_AltViewOnlyAssociationIsNotRunByF3(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.txt",
			Commands: [panel.AssocKindCount]string{panel.AssocAltView: "feh !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocAltView: true},
		},
	})

	withStubHistoryProvider(t)
	pf, pty := setupPanelWithFile(t, "note.txt")
	actionViewFile(pf)

	if len(pty.Written) != 0 {
		t.Errorf("F3 ran a command of the AltView slot; PTY got %q", pty.Written)
	}
	assertViewerOpened(t, pf, "note.txt")
}

// A file whose association has only the View slot keeps the hex viewer on
// Alt+F3: nothing runs and the viewer opens.
func TestActionViewFileHex_ViewOnlyAssociationOpensHexViewer(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocView: "eog !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocView: true},
		},
	})

	withStubHistoryProvider(t)
	pf, pty := setupPanelWithFile(t, "pic.png")
	actionViewFileHex(pf)

	if len(pty.Written) != 0 {
		t.Errorf("Alt+F3 ran a command of the View slot; PTY got %q", pty.Written)
	}
	assertViewerOpened(t, pf, "pic.png")
}

// A disabled AltView slot is skipped and the hex viewer opens.
func TestActionViewFileHex_DisabledAltViewOpensHexViewer(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocAltView: "feh !.!"},
		},
	})

	withStubHistoryProvider(t)
	pf, pty := setupPanelWithFile(t, "pic.png")
	actionViewFileHex(pf)

	if len(pty.Written) != 0 {
		t.Errorf("a disabled AltView slot ran; PTY got %q", pty.Written)
	}
	assertViewerOpened(t, pf, "pic.png")
}

// assertViewerOpened checks that the viewer was asked to open name: the
// history entry is remembered synchronously before the async open, so it is
// the witness (as in TestOpenViewerEditorHistoryEntry_DispatchesByMode). The
// viewer the open then brings up is closed again.
func assertViewerOpened(t *testing.T, pf *panel.PanelsFrame, name string) {
	t.Helper()
	if entries := loadViewerEditorHistory(); len(entries) != 1 || entries[0].Mode != historyModeView {
		t.Fatalf("the built-in viewer was not asked to open the file: %#v", entries)
	}
	fsp := pf.GetActivePanel()
	closeOpenedHistoryFile(t, fsp.Vfs.Join(fsp.Vfs.GetPath(), name), historyModeView)
}
