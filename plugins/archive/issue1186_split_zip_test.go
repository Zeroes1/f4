package archive

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/zip"
)

// issue1186BuildSplitZip writes a ZIP split archive the way WinZip, WinRAR and
// "zip -s" do: one archive cut in two, with archive.z01 holding the first
// entry and archive.zip the second one plus the central directory, where every
// entry names the volume it starts on and its offset counts from that volume
// (APPNOTE 4.4.15, 4.4.16). It returns the directory and the member contents.
func issue1186BuildSplitZip(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	members := map[string][]byte{
		"first.bin":  bytes.Repeat([]byte("first volume payload\n"), 500),
		"second.bin": bytes.Repeat([]byte("second volume payload\n"), 500),
	}

	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range []string{"first.bin", "second.bin"} {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(members[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data := buf.Bytes()

	split := bytes.Index(data[4:], []byte("PK\x03\x04"))
	if split < 0 {
		t.Fatal("second local header not found")
	}
	split += 4
	end := bytes.LastIndex(data, []byte("PK\x05\x06"))
	if end < 0 {
		t.Fatal("end of central directory not found")
	}
	directoryOffset := int(binary.LittleEndian.Uint32(data[end+16 : end+20]))
	binary.LittleEndian.PutUint16(data[end+4:end+6], 1)
	binary.LittleEndian.PutUint16(data[end+6:end+8], 1)
	binary.LittleEndian.PutUint32(data[end+16:end+20], uint32(directoryOffset-split))
	for p := directoryOffset; p < end; {
		nameLen := int(binary.LittleEndian.Uint16(data[p+28 : p+30]))
		extraLen := int(binary.LittleEndian.Uint16(data[p+30 : p+32]))
		commentLen := int(binary.LittleEndian.Uint16(data[p+32 : p+34]))
		if offset := int(binary.LittleEndian.Uint32(data[p+42 : p+46])); offset >= split {
			binary.LittleEndian.PutUint16(data[p+34:p+36], 1)
			binary.LittleEndian.PutUint32(data[p+42:p+46], uint32(offset-split))
		}
		p += 46 + nameLen + extraLen + commentLen
	}

	if err := os.WriteFile(filepath.Join(dir, "archive.z01"), data[:split], 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "archive.zip"), data[split:], 0o600); err != nil {
		t.Fatal(err)
	}
	return members
}

// Issue #1186: a split ZIP must list and copy out whole from either of its
// names. Copying used to read the one file the panel sits on, so an entry
// stored in another volume came back as "zip: not a valid zip file".
func TestIssue1186SplitZipListsAndCopiesFromEveryVolume(t *testing.T) {
	root := t.TempDir()
	members := issue1186BuildSplitZip(t, root)
	ctx := context.Background()

	for _, name := range []string{"archive.zip", "archive.z01"} {
		archiveVFS, err := NewArchiveVFSContext(ctx, vfs.NewOSVFS(root), name)
		if err != nil {
			t.Fatalf("enter %s: %v", name, err)
		}
		listed := 0
		if err := archiveVFS.ReadDir(ctx, archiveVFS.GetPath(), func(items []vfs.VFSItem) { listed += len(items) }); err != nil {
			t.Fatalf("list %s: %v", name, err)
		}
		if listed != len(members) {
			t.Fatalf("list %s: %d entries, want %d", name, listed, len(members))
		}

		copied := t.TempDir()
		selected := []string{"first.bin", "second.bin"}
		if err := archiveVFS.CopyBulkAt(ctx, archiveVFS.GetPath(), selected, vfs.NewOSVFS(copied), copied, &issue915ProgressRecorder{}); err != nil {
			t.Fatalf("copy out of %s: %v", name, err)
		}
		_ = archiveVFS.Close()
		for member, want := range members {
			got, err := os.ReadFile(filepath.Join(copied, member))
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("%s: copied %s as %d bytes, want %d (err %v)", name, member, len(got), len(want), err)
			}
		}
	}
}
