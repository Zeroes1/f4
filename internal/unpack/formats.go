//go:build !lite

package unpack

import (
	"bytes"
	"io"

	gzip "github.com/klauspost/pgzip"
	"github.com/unxed/sevenzip"
	"github.com/unxed/zip"
)

// The regular build reads the archives it downloads with the same libraries
// the archive plugin uses. formats_lite.go swaps them for the standard
// library, which covers everything f4's own release and plugin archives hold.

func newGzipReader(r io.Reader) (io.ReadCloser, error) {
	return gzip.NewReader(r)
}

func zipEntries(data []byte) ([]archiveEntry, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	entries := make([]archiveEntry, len(zr.File))
	for i, f := range zr.File {
		entries[i] = archiveEntry{
			name:  f.Name,
			isDir: f.FileInfo().IsDir(),
			mode:  f.Mode(),
			open:  f.Open,
		}
	}
	return entries, nil
}

func SevenZip(data []byte, destDir string) error {
	szr, err := sevenzip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	for _, f := range szr.File {
		if err := extractEntry(archiveEntry{
			name:  f.Name,
			isDir: f.FileInfo().IsDir(),
			mode:  f.Mode(),
			open:  f.Open,
		}, destDir); err != nil {
			return err
		}
	}
	return nil
}
