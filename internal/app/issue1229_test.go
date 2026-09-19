package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// pumpUntil runs queued UI tasks until cond holds or the deadline passes.
func pumpUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for !cond() {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(5 * time.Millisecond):
		case <-deadline:
			t.Fatalf("timed out waiting for %s", what)
		}
	}
}

func setupRenameConflict(t *testing.T) (*panel.PanelsFrame, *panel.FileSystemPanel, string) {
	t.Helper()
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	theme.SetDefaultF4Palette()

	dir := t.TempDir()
	for name, data := range map[string]string{"a.txt": "A", "b.txt": "B"} {
		// #nosec G703 -- the path is inside the private test temp directory.
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	pf := panel.NewPanelsFrame()
	t.Cleanup(pf.Close)
	pf.ResizeConsole(80, 25)
	fsp := pf.Panels[0].(*panel.FileSystemPanel)
	fsp.Vfs = vfs.NewOSVFS(dir)
	pf.ActiveIdx = 0
	return pf, fsp, dir
}

func topDialog(t *testing.T) *vtui.Window {
	t.Helper()
	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("top frame is %T, want a dialog", vtui.FrameManager.GetTopFrame())
	}
	return dlg
}

func readFile(t *testing.T, path string) (string, bool) {
	t.Helper()
	// #nosec G304 G703 -- the path is inside the private test temp directory.
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data), true
}

// TestIssue1229RenameOntoExistingFileAsksToOverwrite is the regression test
// for issue #1229: Shift+F6 to a name that is taken only reported "file
// exists", while Shift+F5 (copy) offers to overwrite. Rename now asks first,
// and nothing is replaced until the user agrees.
func TestIssue1229RenameOntoExistingFileAsksToOverwrite(t *testing.T) {
	pf, fsp, dir := setupRenameConflict(t)

	renameEntry(pf, fsp, "a.txt", "b.txt")
	pumpUntil(t, "the overwrite question", func() bool {
		dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
		return ok && dlg.OnResult != nil
	})
	if got, _ := readFile(t, filepath.Join(dir, "b.txt")); got != "B" {
		t.Fatalf("b.txt was replaced before the user agreed: %q", got)
	}

	topDialog(t).OnResult(0) // "Overwrite" is the first button
	pumpUntil(t, "the rename", func() bool {
		_, oldExists := readFile(t, filepath.Join(dir, "a.txt"))
		return !oldExists
	})
	if got, _ := readFile(t, filepath.Join(dir, "b.txt")); got != "A" {
		t.Errorf("b.txt = %q after overwriting, want the content of a.txt", got)
	}
}

// TestIssue1229RenameOntoExistingFileCanBeCancelled checks the other answer.
func TestIssue1229RenameOntoExistingFileCanBeCancelled(t *testing.T) {
	pf, fsp, dir := setupRenameConflict(t)

	renameEntry(pf, fsp, "a.txt", "b.txt")
	pumpUntil(t, "the overwrite question", func() bool {
		dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
		return ok && dlg.OnResult != nil
	})
	topDialog(t).OnResult(1) // Cancel
	for i := 0; i < 20; i++ {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-time.After(10 * time.Millisecond):
		}
	}

	if got, ok := readFile(t, filepath.Join(dir, "a.txt")); !ok || got != "A" {
		t.Errorf("a.txt = %q (exists=%v) after cancelling, want it untouched", got, ok)
	}
	if got, _ := readFile(t, filepath.Join(dir, "b.txt")); got != "B" {
		t.Errorf("b.txt = %q after cancelling, want it untouched", got)
	}
}
