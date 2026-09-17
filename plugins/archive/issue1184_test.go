package archive

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// issue1184ZIPContainer builds the bytes of a small but real ZIP container.
// An office document, a .jar and a plain archive differ by name only, which
// is the whole point of the issue.
func issue1184ZIPContainer(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("[Content_Types].xml")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("<Types/>")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func issue1184Write(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestIssue1184DocumentsKeepEnterForAssociation is the regression test for
// issue #1184: office documents are ZIP containers, so the content probe that
// finds self-extracting archives claimed them too, and Enter browsed them as
// archives instead of opening them by extension association. Ctrl+PgDn, which
// asks for the archive explicitly, must keep working on them.
func TestIssue1184DocumentsKeepEnterForAssociation(t *testing.T) {
	root := t.TempDir()
	container := issue1184ZIPContainer(t)
	for _, name := range []string{"report.docx", "budget.xlsx", "deck.pptx", "notes.odt", "lib.jar", "book.epub"} {
		issue1184Write(t, root, name, container)
	}

	provider := &ArchiveProvider{}
	parent := vfs.NewOSVFS(root)
	ctx := context.Background()

	for _, name := range []string{"report.docx", "budget.xlsx", "deck.pptx", "notes.odt", "lib.jar", "book.epub"} {
		if !provider.CanOpen(ctx, parent, name) {
			t.Errorf("%s: CanOpen=false, Ctrl+PgDn must still browse the container", name)
		}
		if provider.PanelEnterAllowed(ctx, parent, name) {
			t.Errorf("%s: ordinary Enter must go to the extension association, not into the archive", name)
		}
	}
}

// TestIssue1184ArchiveNamesKeepEnter pins the other side of the rule: an
// entry whose name declares an archive still opens on Enter, split volumes
// and extension-less archives included.
func TestIssue1184ArchiveNamesKeepEnter(t *testing.T) {
	root := t.TempDir()
	container := issue1184ZIPContainer(t)
	issue1184Write(t, root, "data.zip", container)
	issue1184Write(t, root, "volume.z01", container)
	issue1184Write(t, root, "legacy.r00", container)
	issue1184Write(t, root, "noextension", container)
	issue1184Write(t, root, "split.7z.001", validSevenZipStartHeader())

	provider := &ArchiveProvider{}
	parent := vfs.NewOSVFS(root)
	ctx := context.Background()

	for _, name := range []string{"data.zip", "volume.z01", "legacy.r00", "noextension", "split.7z.001"} {
		if !provider.PanelEnterAllowed(ctx, parent, name) {
			t.Errorf("%s: ordinary Enter must still open the archive", name)
		}
	}

	// A self-extracting archive stays out of ordinary Enter whether or not
	// it carries an extension: Enter runs it, Ctrl+PgDn browses it.
	issue1184Write(t, root, "setup.exe", append([]byte("stub code\n"), container...))
	issue1184Write(t, root, "setup", append([]byte("stub code\n"), container...))
	for _, name := range []string{"setup.exe", "setup"} {
		if provider.PanelEnterAllowed(ctx, parent, name) {
			t.Errorf("%s: self-extracting archive must not open on ordinary Enter", name)
		}
	}
}

func TestIssue1184NameDeclaresArchive(t *testing.T) {
	declared := []string{
		"data.zip", "DATA.ZIP", "src.tar", "src.tar.gz", "src.tar.zst", "src.tgz",
		"src.txz", "src.tbz2", "src.tzst", "dump.gz", "dump.bz2", "dump.xz",
		"dump.zst", "media.rar", "media.part1.rar", "media.7z",
		"split.7z.001", "split.zip.002", "volume.z01", "legacy.r00",
	}
	for _, name := range declared {
		if !nameDeclaresArchive(name) {
			t.Errorf("%s: expected the name to declare an archive", name)
		}
	}

	plain := []string{
		"report.docx", "budget.xlsx", "deck.pptx", "notes.odt", "lib.jar",
		"book.epub", "app.apk", "setup.exe", "photo.jpeg", "noextension",
		"trailing.", "archive.zipx",
	}
	for _, name := range plain {
		if nameDeclaresArchive(name) {
			t.Errorf("%s: expected the name not to declare an archive", name)
		}
	}
}
