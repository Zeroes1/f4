package fileops

import (
	"context"
	"io"

	"github.com/unxed/f4/vfs"
)

// withTransferContent returns the source to copy srcPath from. When the
// source hands over a ready file for the item (vfs.TransferContentProvider),
// the item is copied as that one file, even if it is a directory; otherwise
// the source is returned as it is. A copy inside the same session gets the
// item itself.
func withTransferContent(ctx context.Context, srcVFS vfs.VFS, srcPath string, dstVFS vfs.VFS) (vfs.VFS, error) {
	provider, ok := srcVFS.(vfs.TransferContentProvider)
	if !ok || vfs.SameSession(srcVFS, dstVFS) {
		return srcVFS, nil
	}
	content, ok, err := provider.TransferContent(ctx, srcPath, dstVFS)
	if err != nil || !ok {
		return srcVFS, err
	}
	return exportedItemVFS{VFS: srcVFS, path: srcPath, content: content}, nil
}

// ContentVFS shows the path of src as a file holding content, for the viewer
// of a folder that src hands over as one file (vfs.TransferContentProvider);
// every other path is src's own.
func ContentVFS(src vfs.VFS, path string, content []byte) vfs.VFS {
	return exportedItemVFS{VFS: src, path: path, content: content}
}

// exportedItemVFS shows one path of a source as a file with the content the
// source handed over for it; every other call goes to the source.
type exportedItemVFS struct {
	vfs.VFS
	path    string
	content []byte
}

func (v exportedItemVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	item, err := v.VFS.Stat(ctx, p)
	if err != nil || p != v.path {
		return item, err
	}
	item.IsDir = false
	item.IsSymlink = false
	item.Size = int64(len(v.content))
	item.SizeKnown = true
	return item, nil
}

func (v exportedItemVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	if p != v.path {
		return v.VFS.Open(ctx, p)
	}
	return &contentReader{data: v.content}, nil
}

// EditableContentVFS is ContentVFS whose file can also be saved: the text
// written to the path goes back to the importer when the file is closed, for
// the editor of a folder that src hands over as one file (f4#242: a registry
// key edited as .reg).
func EditableContentVFS(src vfs.VFS, path string, content []byte, importer vfs.TransferContentImporter) vfs.VFS {
	return &editedItemVFS{exportedItemVFS: exportedItemVFS{VFS: src, path: path, content: content}, importer: importer}
}

type editedItemVFS struct {
	exportedItemVFS
	importer vfs.TransferContentImporter
}

// GetCapabilities says the file is written in place: the editor then saves
// straight through Create instead of a temporary file renamed over it.
func (v *editedItemVFS) GetCapabilities() vfs.VFSCapabilities {
	caps := v.VFS.GetCapabilities()
	caps.HasWrite = true
	caps.HasIdentityPreservingWrite = true
	return caps
}

func (v *editedItemVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	return v.exportedItemVFS.Stat(ctx, p)
}

func (v *editedItemVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	return v.exportedItemVFS.Open(ctx, p)
}

func (v *editedItemVFS) Create(ctx context.Context, p string) (io.WriteCloser, error) {
	if p != v.path {
		return v.VFS.Create(ctx, p)
	}
	return &importWriter{ctx: ctx, v: v}, nil
}

// importWriter collects the saved text and imports it on Close; once the
// import succeeds the file reads as the saved text.
type importWriter struct {
	ctx context.Context
	v   *editedItemVFS
	buf []byte
}

func (w *importWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	return len(p), nil
}

func (w *importWriter) Close() error {
	if err := w.v.importer.ImportContent(w.ctx, w.v.path, w.buf); err != nil {
		return err
	}
	w.v.content = w.buf
	return nil
}

// contentReader reads a file that is held in memory, from the start or at
// an offset.
type contentReader struct {
	data []byte
	pos  int64
}

func (r *contentReader) Size() int64 { return int64(len(r.data)) }

func (r *contentReader) Read(ctx context.Context, p []byte) (int, error) {
	n, err := r.ReadAt(ctx, p, r.pos)
	r.pos += int64(n)
	return n, err
}

func (r *contentReader) ReadAt(_ context.Context, p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (r *contentReader) Close() error { return nil }
