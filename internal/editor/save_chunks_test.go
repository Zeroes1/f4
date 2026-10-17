package editor

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// The chunk buffer keeps only part of the file now, so a save can no longer
// count on every unchanged piece being in memory, which the old one did when
// it closed the file before renaming over it. These are its replacements:
// the save reads the file, and lets go of it the way it lets go of a mapping.

// chunkedEditorText is a file well over a small chunk buffer's cache: a save
// that leaned on the cache would write holes.
func chunkedEditorText() string {
	var b strings.Builder
	for i := 0; b.Len() < 400*1024; i++ {
		b.WriteString("line ")
		b.WriteString(strings.Repeat("ab", i%40))
		b.WriteString("\n")
	}
	return b.String()
}

// openChunkedEditor opens path the way showEditor does when it cannot map a
// file, through a chunk buffer with small chunks and the smallest cache.
func openChunkedEditor(t *testing.T, v vfs.VFS, path string) (*EditorView, *countingReadAtCloser) {
	t.Helper()
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	counter := &countingReadAtCloser{ReadAtCloser: f}
	buf := NewAsyncBuffer(context.Background(), counter)
	buf.chunkSize = 1024
	buf.limit = 1
	ev := NewEditorViewIndexedLater(piecetable.NewWithBuffer(buf), v, path)
	ev.File = counter
	ev.AsyncBuf = buf
	ev.Codepage = 65001
	return ev, counter
}

// chunkedText reads the whole of the editor's text, waiting for the chunks.
func chunkedText(t *testing.T, ev *EditorView) string {
	t.Helper()
	stop := pumpUITasks()
	defer stop()
	data, err := readSearchSnapshot(nil, ev.Pt, nil)
	if err != nil {
		t.Fatalf("reading the buffer: %v", err)
	}
	return string(data)
}

func TestChunkedEditor_SavesAFileLargerThanItsCache(t *testing.T) {
	for _, release := range []bool{false, true} {
		name := "keeping the file open"
		if release {
			name = "closing the file first"
		}
		t.Run(name, func(t *testing.T) {
			if !release && runtime.GOOS == "windows" {
				t.Skip("Windows will not rename over a file that is open")
			}
			previous := releaseMappingBeforeReplace
			releaseMappingBeforeReplace = release
			t.Cleanup(func() { releaseMappingBeforeReplace = previous })
			vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
			testutil.DrainPendingTasks()

			dir := t.TempDir()
			path := filepath.Join(dir, "big.txt")
			original := chunkedEditorText()
			if err := os.WriteFile(path, []byte(original), 0600); err != nil {
				t.Fatal(err)
			}
			ev, counter := openChunkedEditor(t, vfs.NewOSVFS(dir), path)
			defer ev.Close()

			mid := len(original) / 2
			ev.Pt.Insert(mid, []byte("<middle>"))
			ev.Pt.Insert(0, []byte("<start>"))
			want := "<start>" + original[:mid] + "<middle>" + original[mid:]
			callsBefore, _ := counter.counted()

			ev.Modified = true
			ev.SaveToFile(nil)
			waitEditorSave(t, ev)
			testutil.DrainPendingTasks()

			if ev.Modified {
				t.Fatal("the save failed")
			}
			if got := readFileString(t, path); got != want {
				t.Fatalf("the saved file is %d bytes and differs from the %d bytes of text", len(got), len(want))
			}
			// The unchanged text came out of the file in large reads, not a
			// thousand-odd chunk fetches.
			if calls, _ := counter.counted(); calls-callsBefore > 20 {
				t.Errorf("the save read the file in %d calls", calls-callsBefore)
			}
			if got := chunkedText(t, ev); got != want {
				t.Error("after saving, the editor does not show the saved text")
			}
			assertNoEditorTempSiblings(t, path)
		})
	}
}

// A save that let go of the file and then could not replace it reads the
// file it left alone again: the text, the edits and undo all survive.
func TestChunkedEditor_FailedRenameReadsTheFileAgain(t *testing.T) {
	releaseMappingEverywhere(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "keep.txt")
	original := chunkedEditorText()
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	v := &renameFailingVFS{OSVFS: vfs.NewOSVFS(dir)}
	ev, _ := openChunkedEditor(t, v, path)
	defer ev.Close()
	oldBuf := ev.AsyncBuf

	before := ev.Pt.GetState()
	ev.Pt.Insert(0, []byte(">> "))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()

	if v.renames.Load() == 0 {
		t.Fatal("precondition: the save never tried to rename")
	}
	if !ev.Modified {
		t.Fatal("a failed save left the editor marked saved")
	}
	if got := readFileString(t, path); got != original {
		t.Fatal("the file on disk changed")
	}
	if ev.AsyncBuf == nil || ev.AsyncBuf == oldBuf || ev.Mapped != nil {
		t.Fatal("the file was not opened again through a chunk buffer")
	}
	if got := chunkedText(t, ev); got != ">> "+original {
		t.Fatal("the editor lost its text")
	}
	ev.Pt.LoadState(before)
	if got := chunkedText(t, ev); got != original {
		t.Error("after undo the editor does not show the original text")
	}
}

// A file with other names is rewritten in place, which leaves no old file to
// read back. The text is copied first, and the other name sees the save.
func TestChunkedEditor_SavesAHardLinkedFileInPlace(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "linked.txt")
	other := filepath.Join(dir, "other-name.txt")
	original := chunkedEditorText()
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, other); err != nil {
		t.Skipf("hard links unavailable: %v", err)
	}
	ev, _ := openChunkedEditor(t, vfs.NewOSVFS(dir), path)
	defer ev.Close()

	ev.Pt.Insert(len(original), []byte("appended\n"))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()

	if ev.Modified {
		t.Fatal("the save failed")
	}
	want := original + "appended\n"
	if got := readFileString(t, other); got != want {
		t.Fatal("the other name does not see the saved text")
	}
	if got := chunkedText(t, ev); got != want {
		t.Error("after saving, the editor does not show the saved text")
	}
}

// Switching codepage reads the whole text. On a chunk buffer that once had
// the whole file loaded after indexing, and Pt.Bytes was enough; it keeps a
// part of the file now, and the switch quietly did nothing.
func TestChunkedEditor_SwitchesCodepage(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	original := chunkedEditorText()
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	ev, _ := openChunkedEditor(t, vfs.NewOSVFS(dir), path)
	defer ev.Close()
	if _, err := ev.Pt.Bytes(); err != piecetable.ErrLoading {
		t.Fatalf("precondition: want a buffer that is not all loaded, got %v", err)
	}

	// The text is read in the background; the switch lands on the UI thread.
	ev.ReloadWithCodepage(866)
	deadline := time.After(10 * time.Second)
	for ev.Codepage != 866 {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the codepage was not switched")
		}
	}
	if got := editorText(t, ev); got != original {
		t.Error("the text changed in an ASCII file")
	}
}
