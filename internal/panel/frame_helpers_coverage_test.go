package panel

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
)

func TestPanelsFrameConvenienceHelpersCoverage(t *testing.T) {
	dir := t.TempDir()
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ActiveIdx = 0
	fsp := pf.Panels[0].(*FileSystemPanel)
	fsp.Vfs = vfs.NewOSVFS(dir)
	fsp.Entries = []*FileEntry{
		{VFSItem: vfs.VFSItem{Name: "one.txt"}, Selected: true},
		{VFSItem: vfs.VFSItem{Name: "two.txt"}},
	}
	other := pf.Panels[1].(*FileSystemPanel)

	if pf.GetActivePanelVFS() != fsp.Vfs || pf.GetPassivePanelVFS() != other.Vfs {
		t.Fatal("panel VFS helpers returned the wrong side")
	}
	if got := pf.GetSelectedNames(); len(got) != 1 || got[0] != "one.txt" {
		t.Fatalf("selected names = %v", got)
	}
	if got := pf.GetMarkedNames(); len(got) != 1 || got[0] != "one.txt" {
		t.Fatalf("marked names = %v", got)
	}
	pf.ReplaceMarkedNames([]string{"two.txt"})
	if fsp.Entries[0].Selected || !fsp.Entries[1].Selected {
		t.Fatalf("replacement selection = %+v", fsp.Entries)
	}
	if got := pf.GetSelectedName(); got != "two.txt" {
		t.Fatalf("selected name = %q", got)
	}
	pf.SetPendingSelection("after.txt")
	if fsp.PendingSelection != "after.txt" {
		t.Fatalf("pending selection = %q", fsp.PendingSelection)
	}
	if filepath.Base(fsp.Vfs.GetPath()) != filepath.Base(dir) {
		t.Fatalf("panel path = %q", fsp.Vfs.GetPath())
	}

	empty := &PanelsFrame{}
	if got := empty.GetMarkedNames(); got != nil {
		t.Fatalf("empty marked names = %v", got)
	}
	empty.ReplaceMarkedNames([]string{"ignored"})
	empty.SetPendingSelection("ignored")
}
