package app

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/panel"
)

// Ctrl+PgDn over a file runs the AltExec command of the matching association
// (far2l: the second way to run a file, f4#1741).
func TestActionEnterDirectory_AltExecAssociationRuns(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocExecute: "eog !.!", panel.AssocAltExec: "convert !.! out.pdf"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocExecute: true, panel.AssocAltExec: true},
		},
	})

	pf, pty := setupPanelWithFile(t, "pic.png")
	actionEnterDirectory(pf)

	written := string(pty.Written)
	if !strings.Contains(written, "convert pic.png out.pdf") {
		t.Errorf("Ctrl+PgDn did not run the AltExec command; PTY got %q", written)
	}
	if strings.Contains(written, "eog") {
		t.Errorf("Ctrl+PgDn ran the Execute command; PTY got %q", written)
	}
}

// A file whose association has only the Execute slot is left alone by
// Ctrl+PgDn: the slots are not interchangeable.
func TestActionEnterDirectory_ExecuteOnlyAssociationIsNotRun(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocExecute: "eog !.!"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocExecute: true},
		},
	})

	pf, pty := setupPanelWithFile(t, "pic.png")
	actionEnterDirectory(pf)

	if len(pty.Written) != 0 {
		t.Errorf("Ctrl+PgDn ran a command of the Execute slot; PTY got %q", pty.Written)
	}
}

// A disabled AltExec slot is skipped.
func TestActionEnterDirectory_DisabledAltExecIsNotRun(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocAltExec: "convert !.! out.pdf"},
		},
	})

	pf, pty := setupPanelWithFile(t, "pic.png")
	actionEnterDirectory(pf)
	if len(pty.Written) != 0 {
		t.Errorf("a disabled AltExec slot ran; PTY got %q", pty.Written)
	}
}

// A file that matches no association keeps what Ctrl+PgDn did before.
func TestActionEnterDirectory_NoMatchingAssociationIsNotRun(t *testing.T) {
	withTempAssociations(t, []panel.FileAssoc{
		{
			Mask:     "*.png",
			Commands: [panel.AssocKindCount]string{panel.AssocAltExec: "convert !.! out.pdf"},
			Enabled:  [panel.AssocKindCount]bool{panel.AssocAltExec: true},
		},
	})

	pf, pty := setupPanelWithFile(t, "readme.txt")
	actionEnterDirectory(pf)
	if len(pty.Written) != 0 {
		t.Errorf("a file without an association ran a command; PTY got %q", pty.Written)
	}
}
