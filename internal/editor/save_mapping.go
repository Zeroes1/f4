package editor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostmode"
	"github.com/unxed/vtui"
)

// loadingBuffer stands in for a mapping a save has released and not yet
// replaced. It has the mapping's size, so the piece table's offsets stay
// valid, and no bytes: every read reports ErrLoading, which the paint path
// draws as an empty line for the few milliseconds the rename takes.
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

// mappingRelease is what releaseMappingForSave did, kept so that a save which
// then fails can give the piece table its text back.
type mappingRelease struct {
	// copied: the mapped text was copied into memory, so the editor still
	// has all of it and there is nothing to recover.
	copied bool
	offset int64
	size   int
}

// describes reports whether path is the file that is mapped, or might be: a
// file that cannot be compared is assumed to be it, since releasing a mapping
// that did not need it costs a moment and keeping one that did fails the save.
// Comparing identities rather than paths is the point (f4#912): a SUBST drive
// or a symlink reaches the same file by another name.
func (m *MappedFile) describes(path string) bool {
	// Under a POSIX host the file information comes from the host's own
	// stat, which os.SameFile cannot compare.
	if m.info == nil || hostmode.Posix() {
		return true
	}
	info, err := os.Stat(path)
	if err != nil {
		return true
	}
	return os.SameFile(m.info, info)
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
// new text has been written out and before the file is renamed over or
// rewritten.
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
		// A table already given another buffer (a codepage reload, a
		// Replace All) does not read the mapping; it just goes.
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
	return &mappingRelease{copied: keepCopy, offset: m.FileOffset(), size: len(data)}, nil
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
	if m, mapErr := MapEditorFileWithOffset(v, f, r.offset); mapErr == nil {
		return f, m, nil, true
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
