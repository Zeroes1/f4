package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestShowEditor_MapsLocalFilesOnEveryPlatform guards the editor's large-file
// path. A local UTF-8 file opened in the editor is memory mapped, on Windows
// as everywhere else, and stays mapped across a save.
//
// Mapping was once switched off on Windows (f0206e37) to get round a save that
// failed because Windows will not replace a mapped file (f4#912). Every file
// then went through the chunk buffer, and a 1 GB log took that much heap and
// indexed nine times slower. The save releases the mapping before it replaces
// the file now (internal/editor/save_mapping.go). If a save fails on a mapped
// file again, fix it there: this test failing means the editor has lost its
// mapping, not that the test needs a Windows exception.
func TestShowEditor_MapsLocalFilesOnEveryPlatform(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	oldMemoryMap := config.App.EditorMemoryMap
	config.App.EditorMemoryMap = true
	defer func() { config.App.EditorMemoryMap = oldMemoryMap }()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.log")
	text := strings.Repeat("2025-01-21 12:00:00 INFO inventory refreshed\n", 20000) // ~1 MB
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}

	filesystem := vfs.NewOSVFS(dir)
	pf := panel.NewPanelsFrame()
	pf.Panels[0] = panel.NewFileSystemPanel(0, 0, 40, 20, filesystem)
	pf.Panels[1] = panel.NewFileSystemPanel(40, 0, 40, 20, filesystem.Clone())
	pf.ResizeConsole(120, 60)
	vtui.FrameManager.Push(pf)

	f, err := filesystem.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	showEditor(pf, filesystem, path, f)
	ev, _ := findOpenedEditor(filesystem, path)
	if ev == nil {
		t.Fatal("editor was not opened")
	}
	defer ev.Close()

	assertMapped := func(when string) {
		t.Helper()
		if ev.Mapped == nil {
			t.Fatalf("%s: the editor did not map the file", when)
		}
		if ev.AsyncBuf != nil {
			t.Fatalf("%s: the editor reads the file through the chunk buffer", when)
		}
		orig, ok := ev.Pt.GetOriginalBuffer().(piecetable.MemoryBuffer)
		mapping := ev.Mapped.Bytes()
		if !ok || len(orig) == 0 || len(orig) != len(mapping) || &orig[0] != &mapping[0] {
			t.Fatalf("%s: the piece table does not read the mapping", when)
		}
	}
	assertMapped("after opening")

	ev.CancelIndexing()
	ev.WaitForIndexing()
	testutil.DrainPendingTasks()

	ev.Pt.Insert(0, []byte("# edited\n"))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()
	if ev.Modified {
		t.Fatal("saving the mapped file failed")
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# edited\n"+text {
		t.Fatal("the saved file does not hold the edited text")
	}
	assertMapped("after saving")
}
