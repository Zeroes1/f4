package editor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostmode"
	"github.com/unxed/vtui"
)

// loadingBuffer stands in for a mapping a save has released and not yet
// replaced. It has the mapping's size, so the piece table's offsets stay
// valid, and no bytes: every read reports ErrLoading, which the paint path
// draws as "[ Loading... ]" until the save opens the file again. Saving a
// mapped file by a rename does not get here (renameOverMapping); saving one
// read through a chunk buffer does, since the handle it reads with is closed.
type loadingBuffer int

func (b loadingBuffer) Size() int                     { return int(b) }
func (b loadingBuffer) Read(int, int) ([]byte, error) { return nil, piecetable.ErrLoading }
func (b loadingBuffer) View(int, int) ([]byte, bool)  { return nil, false }

// errTextUnavailable is what a save reports for a buffer whose unchanged text
// went with a released mapping and could not be read back. Waiting for it, as
// a save waits for a chunk that is still loading, would wait forever.
var errTextUnavailable = errors.New("the unchanged text of this file could not be read back; close the editor and open the file again")

// errMappingBusy is what a save reports when something kept reading the
// mapping past mappingRetireTimeout, so it could not be released.
var errMappingBusy = errors.New("the file is still being read by a search; wait for it to finish, then save again")

// mappingRetireTimeout bounds how long a save waits for readers of the
// mapping it cannot cancel. Those are short — a repaint, a line count — and a
// search, the one long reader, is cancelled outright.
const mappingRetireTimeout = 10 * time.Second

// mappingRelease is what releaseMappingForSave or releaseChunksForSave did,
// kept so that a save which then fails can give the piece table its text back.
type mappingRelease struct {
	// copied: the text was copied into memory, so the editor still has all
	// of it and there is nothing to recover.
	copied bool
	// mapped: the text was read through a mapping, so recovery maps the file
	// again rather than reading it through a chunk buffer.
	mapped bool
	offset int64
	size   int
}

// describes reports whether path is the file that is mapped, or might be.
func (m *MappedFile) describes(path string) bool {
	return sameFileInfo(m.info, path)
}

// fileIs reports whether the open file f is the file at path, or might be.
func fileIs(f vfs.ReadAtCloser, path string) bool {
	st, ok := f.(interface{ Stat() (os.FileInfo, error) })
	if !ok {
		return true
	}
	info, err := st.Stat()
	if err != nil {
		return true
	}
	return sameFileInfo(info, path)
}

// sameFileInfo reports whether path is the file info describes, or might be:
// a file that cannot be compared is assumed to be it, since releasing a
// reader that did not need it costs a moment and keeping one that did fails
// the save. Comparing identities rather than paths is the point (f4#912): a
// SUBST drive or a symlink reaches the same file by another name.
func sameFileInfo(info os.FileInfo, path string) bool {
	// Under a POSIX host the file information comes from the host's own
	// stat, which os.SameFile cannot compare.
	if info == nil || hostmode.Posix() {
		return true
	}
	other, err := os.Stat(path)
	if err != nil {
		return true
	}
	return os.SameFile(info, other)
}

// isOriginalOf reports whether the piece table still reads its unchanged text
// out of this mapping.
func (m *MappedFile) isOriginalOf(Pt *piecetable.PieceTable) bool {
	data := m.Bytes()
	orig, ok := Pt.GetOriginalBuffer().(piecetable.MemoryBuffer)
	return ok && len(data) > 0 && len(orig) == len(data) && &orig[0] == &data[0]
}

// onUIAndWait runs fn on the UI thread and waits until it has. It answers
// false, without waiting further, once the editor has been closed: nothing
// would run the task then.
func (ev *EditorView) onUIAndWait(ctx *vtui.TaskContext, fn func()) bool {
	done := make(chan struct{})
	ctx.RunOnUI(func() {
		defer close(done)
		fn()
	})
	for {
		select {
		case <-done:
			return true
		case <-time.After(100 * time.Millisecond):
			if ev.IsDone() {
				return false
			}
		}
	}
}

// releaseMappingForSave unmaps the file a save is about to replace, on systems
// where the mapping would stop the replacement (releaseMappingBeforeReplace),
// and only when path is that file. It runs on the save goroutine, after the
// new text has been written out: before the file is rewritten in place, or
// after a rename over it was refused (renameOverMapping).
//
// The piece table still reads its unchanged text out of the mapping, so it is
// pointed somewhere else first. With keepCopy that is a copy of the mapped
// text — needed when the old file itself is about to be rewritten, because
// then it cannot be read back if the save fails. Without it the piece table
// gets a loadingBuffer: the save that follows either succeeds, and the
// reopened file replaces the whole table, or fails with the old file
// untouched, and recoverMapping maps it again.
//
// The swap and the unmapping happen on the UI thread. Code there reads the
// piece table outside any guard — highlighting, key handling — and between two
// tasks it holds no window onto the mapping, so a swap made there leaves it
// none to lose. Background readers are counted by guardMapping and waited for
// in between; the searches among them are cancelled instead.
//
// It answers nil when there was nothing to release, and an error, with the
// mapping restored, when it could not be released.
func (ev *EditorView) releaseMappingForSave(ctx *vtui.TaskContext, Pt *piecetable.PieceTable, m *MappedFile, path string, keepCopy bool) (*mappingRelease, error) {
	// A file that is about to be rewritten in place goes on every system:
	// rewriting it truncates the pages under the mapping, which a reader of
	// them then faults on, and a rewrite that fails halfway leaves nothing
	// to read the old text back from. Unix refuses to map a file with other
	// names for that reason, but a link can be made after the file is open.
	if (!releaseMappingBeforeReplace && !keepCopy) || Pt == nil || m == nil || m.Size() == 0 || !m.describes(path) {
		return nil, nil
	}
	data := m.Bytes()
	var standIn piecetable.Buffer = loadingBuffer(len(data))
	if keepCopy {
		// Copied here rather than on the UI thread: it is the whole file.
		standIn = piecetable.MemoryBuffer(bytes.Clone(data))
	}
	swapped := false
	if !ev.onUIAndWait(ctx, func() {
		// Replace All and a codepage reload keep the table and its
		// original buffer (#2051), so this is normally the mapping. A table
		// given another buffer in the meantime does not read it; it just goes.
		if m.isOriginalOf(Pt) {
			Pt.UpdateOriginalBuffer(standIn)
			swapped = true
		}
	}) {
		return nil, errEditorClosed
	}
	// One reader is the save itself, which is not going to read the mapping
	// again.
	if !m.retire(1, mappingRetireTimeout) {
		ev.onUIAndWait(ctx, func() {
			if swapped {
				Pt.UpdateOriginalBuffer(piecetable.MemoryBuffer(data))
			}
		})
		return nil, errMappingBusy
	}
	ev.onUIAndWait(ctx, func() {
		if err := m.Close(); err != nil {
			vtui.DebugLog("EDITOR_MMAP: releasing the mapping before saving: %v", err)
		}
	})
	if !swapped {
		return nil, nil
	}
	return &mappingRelease{copied: keepCopy, mapped: true, offset: m.FileOffset(), size: len(data)}, nil
}

// releaseForSave lets go of whatever the piece table reads the file at path
// through, before a save replaces or rewrites that file: the mapping, or the
// chunk buffer. See releaseMappingForSave and releaseChunksForSave.
func (ev *EditorView) releaseForSave(ctx *vtui.TaskContext, Pt *piecetable.PieceTable, m *MappedFile, buf *AsyncBuffer, path string, keepCopy bool) (*mappingRelease, error) {
	if r, err := ev.releaseMappingForSave(ctx, Pt, m, path, keepCopy); r != nil || err != nil {
		return r, err
	}
	return ev.releaseChunksForSave(ctx, Pt, buf, path, keepCopy)
}

// releaseChunksForSave is releaseMappingForSave for a local file read through
// a chunk buffer. The buffer keeps only the chunks used last, so the rest of
// the unchanged text has to be read from the file whenever it is wanted —
// and the save is about to close the file (Windows will not replace it while
// it is open) or rewrite it (keepCopy). The piece table is pointed at a copy
// of the text when the file is rewritten, since nothing could read the old
// text back then, and at a stand-in otherwise, which recoverMapping replaces
// if the save fails.
//
// Elsewhere, a file that is renamed over stays readable through the open
// handle, which reads the old file, and nothing is released.
func (ev *EditorView) releaseChunksForSave(ctx *vtui.TaskContext, Pt *piecetable.PieceTable, buf *AsyncBuffer, path string, keepCopy bool) (*mappingRelease, error) {
	if Pt == nil || buf == nil || buf.File == nil || buf.Size() == 0 ||
		(!keepCopy && !releaseMappingBeforeReplace) ||
		!fileops.IsLocalOSVFS(ev.Vfs) || !fileIs(buf.File, path) {
		return nil, nil
	}
	if Pt.GetOriginalBuffer() != piecetable.Buffer(buf) {
		// A table given another buffer (a codepage reload, a Replace All)
		// does not read the file.
		return nil, nil
	}
	var standIn piecetable.Buffer = loadingBuffer(buf.Size())
	if keepCopy {
		data := make([]byte, buf.Size())
		n, err := buf.File.ReadAt(ctx.Context, data, buf.FileOffset())
		if n < len(data) {
			if err == nil || err == io.EOF {
				err = io.ErrUnexpectedEOF
			}
			return nil, fmt.Errorf("reading the text before the file is rewritten: %w", err)
		}
		standIn = piecetable.MemoryBuffer(data)
	}
	swapped := false
	if !ev.onUIAndWait(ctx, func() {
		if Pt.GetOriginalBuffer() == piecetable.Buffer(buf) {
			Pt.UpdateOriginalBuffer(standIn)
			swapped = true
		}
	}) {
		return nil, errEditorClosed
	}
	if !swapped {
		return nil, nil
	}
	return &mappingRelease{copied: keepCopy, offset: buf.FileOffset(), size: buf.Size()}, nil
}

// saveStep is how much of the text a save writes at a time.
const saveStep = 4 << 20

// writeTableText writes the text of Pt to w. Unchanged stretches are read out
// of the file in large pieces when read can (see chunkReader), edited ones
// through the piece table. The piece table is the slow way for a file read
// through a chunk buffer: it reads a stretch only once the buffer has fetched
// it, and has to be waited for, so the buffer is asked to fetch ahead of the
// writer.
func writeTableText(ctx context.Context, w io.Writer, Pt *piecetable.PieceTable, read fileChunkReader) error {
	chunks, _ := Pt.GetOriginalBuffer().(*AsyncBuffer)
	var scratch []byte
	logical := 0
	for _, p := range Pt.GetState().Pieces {
		pieceEnd := logical + p.Length
		for curr := logical; curr < pieceEnd; {
			if err := ctx.Err(); err != nil {
				return err
			}
			take := min(saveStep, pieceEnd-curr)
			if p.Buf == piecetable.Original {
				if scratch == nil {
					scratch = make([]byte, saveStep)
				}
				if n, ok := read(ctx, scratch, curr, take); ok {
					if _, err := w.Write(scratch[:n]); err != nil {
						return err
					}
					curr += n
					continue
				}
				if chunks != nil {
					chunks.Prefetch(p.Start+curr-logical, 2*saveStep)
				}
			}
			data, err := Pt.GetRange(curr, take)
			if err == piecetable.ErrLoading {
				// A released reader's stand-in never finishes loading.
				if _, gone := Pt.GetOriginalBuffer().(loadingBuffer); gone {
					return errTextUnavailable
				}
				time.Sleep(20 * time.Millisecond)
				continue
			}
			if err != nil {
				return err
			}
			if len(data) == 0 {
				return io.ErrUnexpectedEOF
			}
			if _, err := w.Write(data); err != nil {
				return err
			}
			curr += len(data)
		}
		logical = pieceEnd
	}
	return nil
}

// renameOverMapping makes the save's rename of the stage over path, the file
// m maps. Windows refuses to replace a file while a handle to it is open, but
// NTFS lets a rename replace one that is only mapped: the view keeps the old
// file's pages, and the editor goes on showing its text. Releasing the mapping
// before every rename instead put up a Loading screen for as long as the
// rename, the reopen and a virus scanner looking at the new file took —
// seconds on a large file.
//
// So the mapping is released only for a file system that does refuse the
// rename, and the rename is then tried once more. The release it made, if
// any, is returned for recoverMapping when the rename fails after all.
func (ev *EditorView) renameOverMapping(ctx *vtui.TaskContext, Pt *piecetable.PieceTable, m *MappedFile, path string, rename func() error) (*mappingRelease, error) {
	err := rename()
	if err == nil || !releaseMappingBeforeReplace || fileops.OperationMustNotRetry(err) ||
		m == nil || m.Size() == 0 || !m.describes(path) {
		return nil, err
	}
	release, releaseErr := ev.releaseMappingForSave(ctx, Pt, m, path, false)
	if releaseErr != nil {
		// The mapping is back where it was; the rename's own error is the
		// one worth reporting.
		vtui.DebugLog("EDITOR_MMAP: releasing the mapping after a refused rename: %v", releaseErr)
		return nil, err
	}
	return release, rename()
}

// errEditorClosed ends a save whose editor was closed while it waited for the
// UI thread.
var errEditorClosed = errors.New("the editor was closed")

// recoverMapping gives the piece table its text back after a save that
// released the mapping and then failed before replacing the file. path is the
// file that was mapped, which the failed replacement left as it was, so its
// bytes are still the ones the table's original pieces describe.
//
// It runs on the save goroutine, but not under the save's context: a save
// cancelled during the rename still has to get the text back.
//
// It returns what the editor should now hold: the reopened file and either a
// new mapping of it or, when the file cannot be mapped again, a chunk buffer
// over it. A file that is no longer the size it was is not the text the table
// describes; the table is then left with the loadingBuffer, and ok is false.
// It does not touch the piece table: adoptRecoveredBacking does, on the UI
// thread, together with the fields that name the backing, for the reason the
// release swaps it there.
func recoverMapping(v vfs.VFS, r *mappingRelease, path string) (f vfs.ReadAtCloser, m *MappedFile, buf *AsyncBuffer, ok bool) {
	if r == nil || r.copied {
		return nil, nil, nil, true
	}
	f, err := reopenReleasedFile(v, path)
	if err != nil {
		vtui.DebugLog("EDITOR_MMAP: reopening %s after a failed save: %v", path, err)
		return nil, nil, nil, false
	}
	if f.Size()-r.offset != int64(r.size) {
		vtui.DebugLog("EDITOR_MMAP: %s changed size during a failed save", path)
		_ = f.Close()
		return nil, nil, nil, false
	}
	if r.mapped {
		if m, mapErr := MapEditorFileWithOffset(v, f, r.offset); mapErr == nil {
			return f, m, nil, true
		}
	}
	return f, nil, NewAsyncBufferWithOffset(context.Background(), f, r.offset), true
}

// reopenReleasedFile opens a file whose mapping a save released, trying a few
// times: the piece table has nothing else to read, and on Windows the moment
// right after a rename is exactly when a virus scanner or an indexer holds the
// file for a while.
func reopenReleasedFile(v vfs.VFS, path string) (vfs.ReadAtCloser, error) {
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			time.Sleep(200 * time.Millisecond)
		}
		var f vfs.ReadAtCloser
		if f, err = v.Open(context.Background(), path); err == nil {
			return f, nil
		}
	}
	return nil, err
}

// adoptRecoveredBacking runs on the UI thread after a save that released the
// mapping failed, and points the piece table and the editor at what
// recoverMapping found. A copy needs nothing: the closed mapping stays in
// ev.Mapped, which keeps the line index on its lazy path, and the piece table
// reads the copy.
func (ev *EditorView) adoptRecoveredBacking(Pt *piecetable.PieceTable, r *mappingRelease, f vfs.ReadAtCloser, m *MappedFile, buf *AsyncBuffer, ok bool) {
	if r == nil || r.copied {
		return
	}
	if !ok {
		ev.noteTextLost()
		return
	}
	if m != nil {
		Pt.UpdateOriginalBuffer(piecetable.MemoryBuffer(m.Bytes()))
	} else {
		Pt.UpdateOriginalBuffer(buf)
	}
	ev.File = f
	ev.AsyncBuf = buf
	ev.Mapped = m
}

// noteTextLost records that the unchanged text went with a released mapping
// and could not be read back, and says so. The editor keeps what was typed,
// but has nothing to save it with: a save would write blanks where the rest
// of the file was.
func (ev *EditorView) noteTextLost() {
	ev.textLost = true
	vtui.ShowMessage(" Error ",
		"The file could not be read back after saving.\n"+
			"Close the editor and open the file again.",
		[]string{"&Ok"})
}
