package app

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
)

// Alt+F4 over a file runs the AltEdit command of the matching association
// (far2l: the second way to edit a file, f4#1741).
func TestActionAltEditFile_AltEditAssociationRuns(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.txt",
			Commands: [panel.AssocKindCount]string{panel.AssocEdit: "vi !.!", panel.AssocAltEdit: "gedit !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocEdit: true, panel.AssocAltEdit: true},
		},
	})

	pf, pty := setupPanelWithFile(t, "note.txt")
	actionAltEditFile(pf)

	written := string(pty.Written)
	if !strings.Contains(written, "gedit note.txt") {
		t.Errorf("Alt+F4 did not run the AltEdit command; PTY got %q", written)
	}
	if strings.Contains(written, "vi note.txt") {
		t.Errorf("Alt+F4 ran the Edit command; PTY got %q", written)
	}
}

// F4 does not run the AltEdit slot: the slots are not interchangeable.
func TestActionEditFile_AltEditOnlyAssociationIsNotRunByF4(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.txt",
			Commands: [panel.AssocKindCount]string{panel.AssocAltEdit: "gedit !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocAltEdit: true},
		},
	})

	withStubHistoryProvider(t)
	pf, pty := setupPanelWithFile(t, "note.txt")
	actionEditFile(pf)

	if strings.Contains(string(pty.Written), "gedit") {
		t.Errorf("F4 ran a command of the AltEdit slot; PTY got %q", pty.Written)
	}
	assertEditorOpened(t, pf, "note.txt")
}

// Without an AltEdit association Alt+F4 opens the editor F4 does not use:
// with UseExternalEditor on, F4 starts the external editor, so Alt+F4 opens
// the built-in one.
func TestActionAltEditFile_NoAssociationOpensTheOtherEditor(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.txt",
			Commands: [panel.AssocKindCount]string{panel.AssocEdit: "vi !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocEdit: true},
		},
	})
	old := config.App.UseExternalEditor
	config.App.UseExternalEditor = true
	t.Cleanup(func() { config.App.UseExternalEditor = old })

	withStubHistoryProvider(t)
	pf, pty := setupPanelWithFile(t, "note.txt")
	actionAltEditFile(pf)

	if len(pty.Written) != 0 {
		t.Errorf("Alt+F4 ran a command of the Edit slot; PTY got %q", pty.Written)
	}
	assertEditorOpened(t, pf, "note.txt")
}

// assertEditorOpened checks that the built-in editor was asked to open name
// (the history entry is the witness, as in assertViewerOpened) and closes it.
func assertEditorOpened(t *testing.T, pf *panel.PanelsFrame, name string) {
	t.Helper()
	if entries := loadViewerEditorHistory(); len(entries) != 1 || entries[0].Mode != historyModeEdit {
		t.Fatalf("the built-in editor was not asked to open the file: %#v", entries)
	}
	fsp := pf.GetActivePanel()
	closeOpenedHistoryFile(t, fsp.Vfs.Join(fsp.Vfs.GetPath(), name), historyModeEdit)
}
