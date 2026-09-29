package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"math/rand"
	"testing"
)

// A many-member tar.gz inside a zip is read through the checkpointed view:
// members near the end, near the start and in the middle all come out right
// and out of order (f4#1678).
func TestArchiveVFSNestedTarGzipMembersOutOfOrder(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewSource(5))
	const members = 24
	contents := make([][]byte, members)
	var raw bytes.Buffer
	tw := tar.NewWriter(&raw)
	for i := range contents {
		size := 200<<10 + rng.Intn(400<<10)
		b := make([]byte, size)
		for j := range b {
			b[j] = "abcdefgh\n"[rng.Intn(9)]
		}
		contents[i] = b
		if err := tw.WriteHeader(&tar.Header{Name: fmt.Sprintf("dir/file%02d.txt", i), Mode: 0o600, Size: int64(size)}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(b); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	var packed bytes.Buffer
	zw := gzip.NewWriter(&packed)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	outer, outerPath := openOuterArchive(t, map[string][]byte{"big.tar.gz": packed.Bytes()})
	bigPath := outer.Join(outerPath, "big.tar.gz")
	big, err := NewArchiveVFSContext(ctx, outer, bigPath)
	if err != nil {
		t.Fatalf("open tar.gz inside zip: %v", err)
	}
	t.Cleanup(func() { _ = big.Close() })
	requireReaderBacked(t, big, "big.tar.gz")

	for _, i := range []int{members - 1, 3, 12, 0, members - 2, 7} {
		member := big.Join(bigPath, fmt.Sprintf("dir/file%02d.txt", i))
		if got := readArchiveMember(t, big, member); !bytes.Equal(got, contents[i]) {
			t.Fatalf("member %d differs (%d bytes, want %d)", i, len(got), len(contents[i]))
		}
	}
}

func TestTarNameOf(t *testing.T) {
	for in, want := range map[string]string{
		"a.tar.gz": "a.tar", "A.TAR.GZ": "A.TAR", "a.tgz": "a.tar", "a.gz": "a.tar", "a.bin": "a.bin.tar",
	} {
		if got := tarNameOf(in); got != want {
			t.Errorf("tarNameOf(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOpenGzipTarViewDeclinesOtherData(t *testing.T) {
	ctx := context.Background()
	for name, data := range map[string][]byte{
		"tiny":     []byte("x"),
		"not gzip": bytes.Repeat([]byte("plain text, not a gzip file "), 20),
		"gzip of text": func() []byte {
			var b bytes.Buffer
			zw := gzip.NewWriter(&b)
			_, _ = zw.Write(bytes.Repeat([]byte("just text, no tar header here "), 40))
			_ = zw.Close()
			return b.Bytes()
		}(),
	} {
		if view, _ := openGzipTarView(ctx, bytes.NewReader(data), int64(len(data)), "x.gz"); view != nil {
			t.Errorf("%s: a view was opened", name)
		}
	}
}
