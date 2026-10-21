package fileops

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/unxed/f4/vfs"
)

type recordingImporter struct {
	path    string
	content []byte
	err     error
}

func (r *recordingImporter) ImportContent(_ context.Context, p string, content []byte) error {
	r.path, r.content = p, append([]byte(nil), content...)
	return r.err
}

// The file of a folder handed over as one file is saved back through the
// importer (f4#242: a registry key edited as .reg).
func TestEditableContentVFSImportsWhatIsSaved_Issue242(t *testing.T) {
	dir := t.TempDir()
	src := vfs.NewOSVFS(dir)
	path := src.Join(dir, "key")
	imp := &recordingImporter{}
	v := EditableContentVFS(src, path, []byte("old"), imp)

	if caps := v.GetCapabilities(); !caps.HasWrite || !caps.HasIdentityPreservingWrite {
		t.Fatalf("capabilities %+v: the editor must write the file in place", caps)
	}
	ctx := context.Background()
	w, err := v.Create(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "new text"); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if imp.path != path || string(imp.content) != "new text" {
		t.Fatalf("imported %q at %q, want %q at %q", imp.content, imp.path, "new text", path)
	}
	r, err := v.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64)
	n, _ := r.ReadAt(ctx, buf, 0)
	if string(buf[:n]) != "new text" {
		t.Errorf("after the save the file reads %q, want the saved text", buf[:n])
	}

	imp.err = errors.New("refused")
	w, err = v.Create(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "bad")
	if err := w.Close(); err == nil {
		t.Fatal("a refused import did not fail the save")
	}
}
