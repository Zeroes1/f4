package editor

import (
	"context"
	"errors"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// MappedFile is a read-only memory map of a local file. The editor uses it as
// the piece table's original buffer: a mapping is one contiguous []byte, so
// PieceTable.View can hand out a window onto any part of it and a search runs
// over the file itself instead of over a copy assembled for the occasion.
//
// Nothing is read at map time. The kernel faults pages in as they are touched
// and can drop them again under memory pressure without swapping, because they
// are clean and file-backed — which is what lets the editor hold a file larger
// than the memory it is allowed to keep.
type MappedFile struct {
	data   []byte
	Mapped []byte
	// offset is how many bytes of the file data starts at. It is what a
	// reader has to add to a position in this buffer to get the position of
	// the same byte in the file.
	offset int64

	// gen and readers count the goroutines inside guardMapping, by the
	// generation they entered in. retire moves to a new generation and waits
	// only for the readers of the earlier ones: a reader that enters after
	// that reads whatever the piece table was given instead of the mapping,
	// so it has no window onto the mapping to lose and nothing to wait for.
	//
	// Every earlier generation, not just the last: a retire that timed out
	// leaves its straggler counted in its generation, and the save that is
	// retried next must wait for it too. Two generations told apart by
	// parity lost it, and unmapped the file under it.
	gen     atomic.Uint32
	readers map[uint32]int64 // guarded by mu

	// cancels stops the readers that can be stopped — searches — so that
	// retire does not wait out a pass over a file of many gigabytes.
	mu      sync.Mutex
	cancels map[uint64]func()
	nextID  uint64

	// info identifies the mapped file, so a save can tell whether the path
	// it writes is this file. Nil when the file could not say.
	info os.FileInfo
}

// pin records a reader of the mapping and returns the call that ends it.
// cancel, when not nil, is how retire stops that reader early.
func (m *MappedFile) pin(cancel func()) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.gen.Load()
	if m.readers == nil {
		m.readers = make(map[uint32]int64)
	}
	m.readers[g]++
	id := uint64(0)
	if cancel != nil {
		if m.cancels == nil {
			m.cancels = make(map[uint64]func())
		}
		id = m.nextID
		m.nextID++
		m.cancels[id] = cancel
	}
	return func() {
		m.mu.Lock()
		defer m.mu.Unlock()
		if cancel != nil {
			delete(m.cancels, id)
		}
		if m.readers[g]--; m.readers[g] == 0 {
			delete(m.readers, g)
		}
	}
}

// readersUpTo counts the readers that entered in generation old or before.
func (m *MappedFile) readersUpTo(old uint32) int64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := int64(0)
	for g, count := range m.readers {
		if g <= old {
			n += count
		}
	}
	return n
}

// retire waits until every reader that entered before this call has left,
// except held of them: the caller's own pin, when it has one. The caller must
// already have pointed the piece table away from the mapping, so the windows
// still alive are only the ones those readers took out earlier. Readers that
// can be cancelled are; the rest are waited for, up to timeout, and false
// means some were still reading when it ran out.
func (m *MappedFile) retire(held int64, timeout time.Duration) bool {
	m.mu.Lock()
	old := m.gen.Add(1) - 1
	cancels := make([]func(), 0, len(m.cancels))
	for _, cancel := range m.cancels {
		cancels = append(cancels, cancel)
	}
	m.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
	deadline := time.Now().Add(timeout)
	for m.readersUpTo(old) > held {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(time.Millisecond)
	}
	return true
}

// FileOffset reports where in the file the mapped contents begin: zero, or
// the length of a UTF-8 byte-order mark that is not part of the text.
func (m *MappedFile) FileOffset() int64 {
	if m == nil {
		return 0
	}
	return m.offset
}

// Bytes returns the mapped contents. The result aliases the mapping and must
// not be modified: it is mapped read-only, so a write would fault.
func (m *MappedFile) Bytes() []byte {
	if m == nil {
		return nil
	}
	return m.data
}

// Size reports the mapped length.
func (m *MappedFile) Size() int {
	if m == nil {
		return 0
	}
	return len(m.data)
}

// Close releases the mapping. Every window handed out by Bytes becomes invalid,
// so callers must be done reading before this runs.
func (m *MappedFile) Close() error {
	if m == nil || (m.data == nil && m.Mapped == nil) {
		return nil
	}
	data := m.Mapped
	if data == nil {
		data = m.data
	}
	m.data = nil
	m.Mapped = nil
	return unmapFileRegion(data)
}

// ErrNotMappable is the ordinary answer for a file the editor should open the
// usual way, not a failure worth reporting.
var ErrNotMappable = errors.New("file cannot be memory mapped")

// fileDescriptor is what a local vfs.ReadAtCloser exposes through the *os.File
// it embeds. A remote one does not implement it, which is exactly the test the
// editor needs: no descriptor, no mapping.
type fileDescriptor interface {
	Fd() uintptr
}

// CanMemoryMapFile reports whether a local file is safe to map while it is
// being edited. Multiply-linked files are not: saving them must preserve the
// shared file identity with an in-place rewrite, which is a write into the
// very file the mapping describes. Windows also refuses to replace a mapped
// file at all, through any path to it (f4#912); saveToFile releases the
// mapping before it replaces the file there, see releaseMappingForSave.
func CanMemoryMapFile(ctx context.Context, v vfs.VFS, path string) bool {
	if !fileops.IsLocalOSVFS(v) {
		return false
	}
	item, err := v.Stat(ctx, path)
	if err != nil {
		// Keep the previous best-effort mapping behavior when metadata is
		// temporarily unavailable; MapEditorFileWithOffset still validates
		// the backing file and can decline the mapping.
		return true
	}
	return !item.HasMetadata(vfs.MetadataNlink) || item.Nlink <= 1
}

// MapEditorFile maps an already opened local file, or answers ErrNotMappable
// when it should not be mapped at all: an empty file has nothing to map, a
// non-local backing has no descriptor to map, and a file too large for the
// address space cannot be described by one slice.
func MapEditorFile(v vfs.VFS, f vfs.ReadAtCloser) (*MappedFile, error) {
	return MapEditorFileWithOffset(v, f, 0)
}

// MapEditorFileWithOffset maps an already opened local file and exposes it
// from fileOffset onward. The complete mapping is retained separately so a
// logical slice that skips a UTF-8 BOM can still be unmapped from its true
// platform-specific base address.
func MapEditorFileWithOffset(v vfs.VFS, f vfs.ReadAtCloser, fileOffset int64) (*MappedFile, error) {
	if f == nil || !fileops.IsLocalOSVFS(v) {
		return nil, ErrNotMappable
	}
	size := f.Size()
	if size <= 0 || fileOffset < 0 || fileOffset >= size {
		return nil, ErrNotMappable
	}
	if int64(int(size)) != size {
		return nil, ErrNotMappable
	}
	fd, ok := f.(fileDescriptor)
	if !ok || fd.Fd() == 0 || fd.Fd() == ^uintptr(0) {
		return nil, ErrNotMappable
	}

	data, err := mapFileRegion(fd.Fd(), size)
	if err != nil {
		return nil, err
	}
	m := &MappedFile{data: data[fileOffset:], Mapped: data, offset: fileOffset}
	if st, ok := f.(interface{ Stat() (os.FileInfo, error) }); ok {
		if info, err := st.Stat(); err == nil {
			m.info = info
		}
	}
	return m, nil
}

// guardMappedFaults makes a fault on mapped memory recoverable for the calling
// goroutine, and must therefore be called by every goroutine that reads through
// a mapping. A file truncated after it was mapped leaves the pages past the new
// end with nothing behind them, and touching one raises SIGBUS — which Go turns
// into a process-wide crash unless the faulting goroutine has asked for a panic
// instead. The editor would rather lose the buffer than the session.
//
// The flag is per goroutine and is not inherited, so the returned function must
// be deferred by the same goroutine that set it.
func guardMappedFaults(what string, onFault func()) func() {
	return guardMappedFaultsPinned(what, onFault, nil, nil)
}

// guardMappedFaultsPinned is guardMappedFaults for a reader of m, which it
// counts as one for as long as the guard is armed (see MappedFile.retire).
func guardMappedFaultsPinned(what string, onFault func(), m *MappedFile, cancel func()) func() {
	unpin := func() {}
	if m != nil {
		unpin = m.pin(cancel)
	}
	previous := debug.SetPanicOnFault(true)
	return func() {
		debug.SetPanicOnFault(previous)
		unpin()
		r := recover()
		if r == nil {
			return
		}
		// A fault is the only panic worth swallowing here; anything else is a
		// bug that should keep travelling.
		if _, ok := r.(runtime_Error); !ok {
			panic(r)
		}
		vtui.DebugLog("EDITOR_MMAP: fault while %s: %v", what, r)
		if onFault != nil {
			onFault()
		}
	}
}

// guardMapping arms fault recovery for the calling goroutine, but only for an
// editor that actually reads through a mapping: everywhere else the zero value
// is a no-op, so an ordinary buffer keeps crashing honestly on a wild address
// instead of quietly swallowing it. It also counts the goroutine as a reader
// of the mapping, which is what lets a save release it safely.
//
// Use it as `defer ev.guardMapping("...")()`, which defers the returned
// function — the one that calls recover.
func (ev *EditorView) guardMapping(what string) func() {
	return ev.guardMappingFor(what, nil)
}

// guardMappingFor is guardMapping for a background pass that can be stopped:
// a save that has to release the mapping cancels it rather than waiting for
// it to finish.
func (ev *EditorView) guardMappingFor(what string, ctx *vtui.TaskContext) func() {
	if ev == nil {
		return func() {}
	}
	m := ev.Mapped
	if m == nil {
		return func() {}
	}
	var cancel func()
	if ctx != nil {
		cancel = ctx.Cancel
	}
	return guardMappedFaultsPinned(what, ev.noteMappingFault, m, cancel)
}

// noteMappingFault tells the user their file went out from under the editor.
// A mapping faults when the file it describes is truncated, so the buffer on
// screen no longer matches anything on disk and the only honest move is to say
// so; recovering the text would mean reading through the same broken mapping.
func (ev *EditorView) noteMappingFault() {
	vtui.FrameManager.PostTask(func() {
		if ev.mapFaulted {
			return
		}
		ev.mapFaulted = true
		vtui.ShowMessage(" Error ",
			"The file was truncated on disk while it was open here.\n"+
				"Close the editor and open it again to see its current contents.",
			[]string{"&Ok"})
	})
}

// runtime_Error matches runtime.Error without importing it for its own sake:
// a fault arrives as *runtime.Error (runtime.errorAddressString), while an
// ordinary panic does not implement RuntimeError.
type runtime_Error interface {
	error
	RuntimeError()
}
