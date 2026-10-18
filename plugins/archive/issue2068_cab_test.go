package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"path/filepath"
	"testing"
)

// A Microsoft Cabinet (.cab) opens in the archive plugin like any other
// archive: the format is recognised, the members are listed, and each one reads
// back unchanged (issue #2068). The fixture is the makecab.exe cabinet from
// go-cabinet, one stored and one MSZIP folder (testdata/*.NOTICE).
func TestIssue2068CabinetOpensAsArchive(t *testing.T) {
	cabPath := filepath.Join("testdata", "mixed_compression.cab")

	format, err := identifyArchiveFormat(context.Background(), cabPath, "mixed_compression.cab")
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	if format.Extension() != ".cab" {
		t.Fatalf("identified as %T (%s), want the cabinet format", format, format.Extension())
	}

	fsys, err := openArchiveFileSystem(context.Background(), cabPath, "mixed_compression.cab", "")
	if err != nil {
		t.Fatalf("open cabinet: %v", err)
	}
	t.Cleanup(func() { _ = fsys.Close() })

	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("ReadDir = %d entries, want 2", len(entries))
	}

	want := map[string]struct {
		size   int
		sha256 string
	}{
		"compressed.txt": {256, "925b08e69ad682eaae8ddc6c6e048372d8c66a01006c6b04dbc35e200793a728"},
		"plain.txt":      {128, "d52969473f969db7657157d0275812ad01785bfe9ca73c419d0274fd7ca59cf2"},
	}
	for name, w := range want {
		file, err := fsys.Open(name)
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		body, err := io.ReadAll(file)
		_ = file.Close()
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sum := sha256.Sum256(body)
		if len(body) != w.size || hex.EncodeToString(sum[:]) != w.sha256 {
			t.Errorf("%s: got %d bytes, sha256 %x; want %d bytes, %s", name, len(body), sum, w.size, w.sha256)
		}
	}
}
