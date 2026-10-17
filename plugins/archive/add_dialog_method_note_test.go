package archive

import (
	"testing"

	"github.com/unxed/vtui"
)

func addArchiveMethodIndex(t *testing.T, format int, id string) int {
	t.Helper()
	for i, m := range addArchiveFormats[format].Methods {
		if m.ID == id {
			return i
		}
	}
	t.Fatalf("format %d has no method %q", format, id)
	return -1
}

// Zstandard and LZMA in a zip are said to open in fewer tools, next to the
// method; Deflate and Store are not (f4#2002).
func TestAddArchiveDialogNotesTheRareZipMethods(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	d := newAddArchiveDialog("docs.zip", func(addArchiveSettings) {})
	zipIdx := addArchiveFormatIndex(t, ".zip")

	choose := func(id string) {
		i := addArchiveMethodIndex(t, zipIdx, id)
		d.method.Menu.SetSelectPos(i)
		d.method.Menu.OnAction(i)
	}
	if d.methodNote.IsVisible() {
		t.Fatal("the note is shown for Deflate, the default")
	}
	for _, tc := range []struct {
		id   string
		note bool
	}{{"zstd", true}, {"lzma", true}, {"store", false}, {zipDeflateMethod, false}} {
		choose(tc.id)
		if d.methodNote.IsVisible() != tc.note {
			t.Errorf("method %s: note shown = %v, want %v", tc.id, d.methodNote.IsVisible(), tc.note)
		}
	}

	choose("zstd")
	sevenIdx := addArchiveFormatIndex(t, ".7z")
	d.format.Menu.SetSelectPos(sevenIdx)
	d.format.Menu.OnAction(sevenIdx)
	if !d.methodNote.IsVisible() || d.methodNote.GetText() != vtui.Msg("Archive.Add.Method7zNote") {
		t.Errorf("7z: note %q (shown %v), want what the engine uses", d.methodNote.GetText(), d.methodNote.IsVisible())
	}

	d.format.Menu.SetSelectPos(zipIdx)
	d.format.Menu.OnAction(zipIdx)
	if d.methodNote.IsVisible() {
		t.Error("the 7z note stays after switching back to zip with Deflate")
	}
}
