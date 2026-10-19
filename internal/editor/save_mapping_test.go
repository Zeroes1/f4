package editor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// f4#912: Windows will not replace a file while a view of it is mapped, so a
// save has to release the mapping first. These tests force that path on every
// platform; on Windows they also meet the real sharing violation.
func releaseMappingEverywhere(t *testing.T) {
	t.Helper()
	previous := releaseMappingBeforeReplace
	releaseMappingBeforeReplace = true
	t.Cleanup(func() { releaseMappingBeforeReplace = previous })
}

func editorText(t *testing.T, ev *EditorView) string {
	t.Helper()
	data, err := ev.Pt.Bytes()
	if err != nil {
		t.Fatalf("reading the buffer: %v", err)
	}
	return string(data)
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	return string(got)
}

// The reporter's steps: change a line, save, change it back, save again. The
// second save is a rename over a file the first save mapped again, which is
// what failed.
func TestMappedEditor_SavesTwiceKeepingTheMapping(t *testing.T) {
	releaseMappingEverywhere(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "highlight.ini")
	const original = "[Highlight_101]\nMask = *.go\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	ev := openMappedEditor(t, dir, path)
	defer ev.Close()

	header := len("[Highlight_101]")
	ev.Pt.Insert(header, []byte("  "))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()
	if ev.Modified {
		t.Fatal("the first save failed")
	}
	if got := readFileString(t, path); got != "[Highlight_101]  \nMask = *.go\n" {
		t.Fatalf("after the first save the file reads %q", got)
	}

	ev.Pt.Delete(header, 2)
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()
	if ev.Modified {
		t.Fatal("the second save failed")
	}
	if got := readFileString(t, path); got != original {
		t.Fatalf("after the second save the file reads %q, want %q", got, original)
	}
	if ev.Mapped == nil || ev.Mapped.Size() != len(original) {
		t.Error("the editor did not map the saved file again")
	}
	if got := editorText(t, ev); got != original {
		t.Errorf("the editor shows %q, want %q", got, original)
	}
	assertNoEditorTempSiblings(t, path)
}

// An edit that keeps the length is written into the file where it stands
// (OSVFS.PatchInPlace), which on Windows writes through the mapping's file.
func TestMappedEditor_SavesASameLengthEdit(t *testing.T) {
	releaseMappingEverywhere(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "same.txt")
	if err := os.WriteFile(path, []byte("alpha\nbravo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ev := openMappedEditor(t, dir, path)
	defer ev.Close()

	ev.Pt.Delete(6, 5)
	ev.Pt.Insert(6, []byte("BRAVO"))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()

	if ev.Modified {
		t.Fatal("the save failed")
	}
	if got := readFileString(t, path); got != "alpha\nBRAVO\n" {
		t.Fatalf("the file reads %q", got)
	}
	if got := editorText(t, ev); got != "alpha\nBRAVO\n" {
		t.Errorf("the editor shows %q", got)
	}
	assertNoEditorTempSiblings(t, path)
}

// A file with another name is rewritten in place, so the old content cannot
// be mapped back: the save keeps the text in memory instead.
func TestMappedEditor_SavesAHardLinkedFileInPlace(t *testing.T) {
	releaseMappingEverywhere(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	first, second := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt")
	if err := os.WriteFile(first, []byte("text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(first, second); err != nil {
		t.Skipf("no hard links here: %v", err)
	}
	ev := openMappedEditor(t, dir, second)
	defer ev.Close()

	ev.Pt.Insert(0, []byte("X"))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()

	if ev.Modified {
		t.Fatal("the save failed")
	}
	if got := readFileString(t, first); got != "Xtext\n" {
		t.Fatalf("the other name reads %q: the link was broken or the rewrite failed", got)
	}
	if got := editorText(t, ev); got != "Xtext\n" {
		t.Errorf("the editor shows %q", got)
	}
}

// renameFailingVFS is a local file system whose renames all fail before they
// touch anything, as a rename refused by a sharing violation does.
type renameFailingVFS struct {
	*vfs.OSVFS
	renames atomic.Int32
}

var errRenameRefused = errors.New("rename refused")

func (v *renameFailingVFS) Rename(context.Context, string, string) error {
	v.renames.Add(1)
	return errRenameRefused
}

// A save that released the mapping and then could not replace the file must
// leave the editor with its text, edits and undo intact: the old file is still
// there, so it is mapped again.
func TestMappedEditor_FailedRenameMapsTheFileAgain(t *testing.T) {
	releaseMappingEverywhere(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "keep.txt")
	const original = "first line\nsecond line\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	v := &renameFailingVFS{OSVFS: vfs.NewOSVFS(dir)}
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := MapEditorFile(v, f)
	if err != nil {
		t.Fatalf("MapEditorFile: %v", err)
	}
	ev := NewEditorViewIndexedLater(piecetable.New(m.Bytes()), v, path)
	ev.File = f
	ev.Mapped = m
	ev.Codepage = 65001
	defer ev.Close()

	before := ev.Pt.GetState()
	ev.Pt.Insert(0, []byte(">> "))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	// The failure is reported in a message box; leave it on screen.
	testutil.DrainPendingTasks()

	if v.renames.Load() == 0 {
		t.Fatal("precondition: the save never tried to rename")
	}
	if !ev.Modified {
		t.Fatal("a failed save left the editor marked saved")
	}
	if got := readFileString(t, path); got != original {
		t.Fatalf("the file on disk changed: %q", got)
	}
	if got := editorText(t, ev); got != ">> "+original {
		t.Fatalf("the editor shows %q, want the edited text", got)
	}
	if ev.Mapped == nil || ev.Mapped == m || ev.Mapped.Size() != len(original) {
		t.Error("the file was not mapped again")
	}
	// The original pieces still read the right bytes, so undoing works.
	ev.Pt.LoadState(before)
	if got := editorText(t, ev); got != original {
		t.Errorf("after undo the editor shows %q, want %q", got, original)
	}
	assertNoEditorTempSiblings(t, path)
}

// renameHookVFS is a local file system that calls before ahead of each rename,
// with the rename's number, and fails the rename when before does.
type renameHookVFS struct {
	*vfs.OSVFS
	renames atomic.Int32
	before  func(n int32) error
}

func (v *renameHookVFS) Rename(ctx context.Context, from, to string) error {
	if err := v.before(v.renames.Add(1)); err != nil {
		return err
	}
	return v.OSVFS.Rename(ctx, from, to)
}

func openHookedMappedEditor(t *testing.T, path, text string, before func(*EditorView, int32) error) (*EditorView, *renameHookVFS, *MappedFile) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	var ev *EditorView
	v := &renameHookVFS{OSVFS: vfs.NewOSVFS(filepath.Dir(path))}
	v.before = func(n int32) error { return before(ev, n) }
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := MapEditorFile(v, f)
	if err != nil {
		t.Fatalf("MapEditorFile: %v", err)
	}
	ev = NewEditorViewIndexedLater(piecetable.New(m.Bytes()), v, path)
	ev.File = f
	ev.Mapped = m
	ev.Codepage = 65001
	t.Cleanup(func() { ev.Close() })
	return ev, v, m
}

// A save renames the new text over a file that is still mapped, so the
// editor goes on showing the text while it does. Releasing the mapping first
// showed "Loading" for as long as the rename and the reopen took, seconds on
// a large file. On Windows this is the real rename over a mapped file.
func TestMappedEditor_SaveKeepsTheTextReadableThroughTheRename(t *testing.T) {
	releaseMappingEverywhere(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	path := filepath.Join(t.TempDir(), "shown.txt")
	const original = "first line\nsecond line\n"
	var readable atomic.Bool
	var m *MappedFile
	ev, v, m := openHookedMappedEditor(t, path, original, func(ev *EditorView, n int32) error {
		_, loading := ev.Pt.GetOriginalBuffer().(loadingBuffer)
		readable.Store(!loading && m.Bytes() != nil)
		return nil
	})

	ev.Pt.Insert(0, []byte(">> "))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()

	if ev.Modified {
		t.Fatal("the save failed")
	}
	if n := v.renames.Load(); n != 1 {
		t.Fatalf("the save renamed %d times, want once", n)
	}
	if !readable.Load() {
		t.Error("the mapping was released before the rename: the editor showed Loading")
	}
	if got := readFileString(t, path); got != ">> "+original {
		t.Fatalf("the file reads %q", got)
	}
	if ev.Mapped == nil || ev.Mapped == m {
		t.Error("the saved file was not mapped in place of the old one")
	}
	assertNoEditorTempSiblings(t, path)
}

// Where a file system does refuse to rename over a mapped file, the save
// releases the mapping and renames again.
func TestMappedEditor_RefusedRenameIsRetriedWithoutTheMapping(t *testing.T) {
	releaseMappingEverywhere(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	path := filepath.Join(t.TempDir(), "retried.txt")
	const original = "first line\nsecond line\n"
	var releasedForRetry atomic.Bool
	ev, v, _ := openHookedMappedEditor(t, path, original, func(ev *EditorView, n int32) error {
		if n == 1 {
			return errRenameRefused
		}
		_, loading := ev.Pt.GetOriginalBuffer().(loadingBuffer)
		releasedForRetry.Store(loading)
		return nil
	})

	ev.Pt.Insert(0, []byte(">> "))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()

	if ev.Modified {
		t.Fatal("the save failed")
	}
	if n := v.renames.Load(); n != 2 {
		t.Fatalf("the save renamed %d times, want twice", n)
	}
	if !releasedForRetry.Load() {
		t.Error("the second rename was tried with the mapping still held")
	}
	if got := readFileString(t, path); got != ">> "+original {
		t.Fatalf("the file reads %q", got)
	}
	if got := editorText(t, ev); got != ">> "+original {
		t.Errorf("the editor shows %q", got)
	}
	if ev.Mapped == nil {
		t.Error("the saved file was not mapped again")
	}
	assertNoEditorTempSiblings(t, path)
}

// retire is what makes unmapping safe while other goroutines read: it waits
// for the readers that may hold a window onto the mapping, and only for them.
func TestMappedFile_RetireWaitsForEarlierReadersOnly(t *testing.T) {
	m, _, _, _ := mapTestFile(t, "mapped text")

	earlier := m.pin(nil)
	retired := make(chan struct{})
	go func() {
		if !m.retire(0, 10*time.Second) {
			t.Error("retire timed out")
		}
		close(retired)
	}()

	// Wait for retire to have moved to the new generation.
	deadline := time.Now().Add(5 * time.Second)
	for m.gen.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("retire never started")
		}
		time.Sleep(time.Millisecond)
	}
	later := m.pin(nil)
	defer later()

	select {
	case <-retired:
		t.Fatal("retire returned while a reader from before it was still reading")
	case <-time.After(50 * time.Millisecond):
	}
	earlier()
	select {
	case <-retired:
	case <-time.After(5 * time.Second):
		t.Fatal("retire waited for a reader that started after it")
	}
	_ = m.Close()
}

// A search reading the mapping is cancelled rather than waited out: on a
// file of many gigabytes the save would otherwise stall for as long as the
// search runs.
func TestMappedFile_RetireCancelsSearches(t *testing.T) {
	m, _, _, _ := mapTestFile(t, "mapped text")

	ctx, cancel := context.WithCancel(context.Background())
	unpin := m.pin(cancel)
	stopped := make(chan struct{})
	go func() {
		// A search: it reads until it is told to stop.
		<-ctx.Done()
		unpin()
		close(stopped)
	}()

	if !m.retire(0, 5*time.Second) {
		t.Fatal("retire timed out instead of cancelling the search")
	}
	<-stopped
}

// A reader that cannot be cancelled and does not finish does not hang the
// save: retire gives up, and the save reports it.
func TestMappedFile_RetireGivesUpOnAStuckReader(t *testing.T) {
	m, _, _, _ := mapTestFile(t, "mapped text")
	unpin := m.pin(nil)
	defer unpin()

	start := time.Now()
	if m.retire(0, 50*time.Millisecond) {
		t.Fatal("retire reported every reader gone while one was still reading")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Errorf("retire took %v to give up", waited)
	}
}

// A save releases the mapping only for the file it describes, whatever the
// path that reaches it: Save As to another file needs no release and no copy
// of the text, and a second name for the same file does.
func TestMappedFile_DescribesTheFileItMapsOnly(t *testing.T) {
	m, path, _, _ := mapTestFile(t, "mapped text")
	if m.info == nil {
		t.Skip("this file system does not identify its files")
	}
	if !m.describes(path) {
		t.Error("the mapped file is not recognised by its own path")
	}
	other := filepath.Join(filepath.Dir(path), "other.txt")
	if err := os.WriteFile(other, []byte("mapped text"), 0600); err != nil {
		t.Fatal(err)
	}
	if m.describes(other) {
		t.Error("another file of the same content is taken for the mapped one")
	}
	alias := filepath.Join(filepath.Dir(path), "alias.txt")
	if err := os.Link(path, alias); err == nil && !m.describes(alias) {
		t.Error("a second name for the mapped file is not recognised")
	}
	if !m.describes(filepath.Join(filepath.Dir(path), "missing.txt")) {
		t.Error("a path that cannot be compared must be assumed to be the mapped file")
	}
}

// openFailingVFS fails every rename, and every open once a rename has been
// tried: a save that released the mapping then has no way to read the file
// back.
type openFailingVFS struct {
	*vfs.OSVFS
	renames atomic.Int32
}

var errOpenRefused = errors.New("open refused")

func (v *openFailingVFS) Open(ctx context.Context, path string) (vfs.ReadAtCloser, error) {
	if v.renames.Load() > 0 {
		return nil, errOpenRefused
	}
	return v.OSVFS.Open(ctx, path)
}

func (v *openFailingVFS) Rename(context.Context, string, string) error {
	v.renames.Add(1)
	return errRenameRefused
}

// When the text cannot be read back, the editor says so and refuses to save:
// a save would wait forever on the stand-in for the released mapping, or
// write blanks in place of the text it could not read.
func TestMappedEditor_TextThatCannotBeReadBackIsNotSaved(t *testing.T) {
	releaseMappingEverywhere(t)
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "gone.txt")
	const original = "first line\nsecond line\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	v := &openFailingVFS{OSVFS: vfs.NewOSVFS(dir)}
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	m, err := MapEditorFile(v, f)
	if err != nil {
		t.Fatalf("MapEditorFile: %v", err)
	}
	ev := NewEditorViewIndexedLater(piecetable.New(m.Bytes()), v, path)
	ev.File = f
	ev.Mapped = m
	ev.Codepage = 65001
	defer ev.Close()

	ev.Pt.Insert(0, []byte(">> "))
	ev.Modified = true
	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()
	if !ev.textLost {
		t.Fatal("the editor did not notice that its text could not be read back")
	}

	ev.SaveToFile(nil)
	waitEditorSave(t, ev)
	testutil.DrainPendingTasks()
	if got := readFileString(t, path); got != original {
		t.Fatalf("a save without the text changed the file to %q", got)
	}
	assertNoEditorTempSiblings(t, path)
}

// Without a mapping — the setting off, or a file that cannot be mapped — the
// scan still reads the local file in large pieces rather than through the
// chunk buffer, which fetched it 32 KB at a time and kept every byte.
func TestAsyncEditor_IndexReadsTheFileDirectly(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	const line = "a line of text\n"
	const lines = 200000 // ~3 MB
	if err := os.WriteFile(path, []byte(strings.Repeat(line, lines)), 0600); err != nil {
		t.Fatal(err)
	}
	v := vfs.NewOSVFS(dir)
	f, err := v.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	counter := &countingReadAtCloser{ReadAtCloser: f}
	buf := NewAsyncBuffer(context.Background(), counter)
	defer buf.Close()
	ev := NewEditorViewIndexedLater(piecetable.NewWithBuffer(buf), v, path)
	ev.File = counter
	ev.AsyncBuf = buf
	ev.Codepage = 65001
	defer ev.Close()

	ev.StartIndexing()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	for ev.Li.LineCount() < lines+1 {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline.C:
			t.Fatalf("timeout: indexed %d of %d lines", ev.Li.LineCount(), lines+1)
		}
	}

	calls, read := counter.counted()
	if want := int64(lines * len(line)); read < want {
		t.Errorf("the scan read %d bytes of a %d byte file", read, want)
	}
	if calls > 4 {
		t.Errorf("the file was read in %d calls; a direct scan needs one per 4 MB", calls)
	}
	buf.mu.Lock()
	loaded := len(buf.loaded)
	buf.mu.Unlock()
	// The wrap-safety probe reads the first few chunks through the buffer;
	// the scan must not have read the rest that way.
	if total := (lines*len(line) + buf.ChunkSize() - 1) / buf.ChunkSize(); loaded > total/10 {
		t.Errorf("the scan went through the chunk buffer: %d of %d chunks kept", loaded, total)
	}
	for i := 0; i < 5; i++ {
		if got := ev.Li.GetLineOffset(i); got != i*len(line) {
			t.Errorf("line %d starts at %d, want %d", i, got, i*len(line))
		}
	}
}

// Prefetch is the read-ahead: it must start fetches and nothing else, where
// Read assembled a copy of every window it was asked about.
func TestAsyncBuffer_PrefetchStartsFetchesWithoutCopying(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	content := strings.Repeat("0123456789abcdef", 64*1024) // 1 MB
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := vfs.NewOSVFS(dir).Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	buf := NewAsyncBuffer(context.Background(), f)
	defer buf.Close()

	buf.Prefetch(0, len(content))
	want := (len(content) + buf.ChunkSize() - 1) / buf.ChunkSize()
	deadline := time.Now().Add(5 * time.Second)
	for {
		buf.mu.Lock()
		loaded, fetching := len(buf.loaded), len(buf.fetching)
		buf.mu.Unlock()
		if loaded == want && fetching == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("%d of %d chunks loaded, %d still fetching", loaded, want, fetching)
		}
		time.Sleep(time.Millisecond)
	}
	testutil.DrainPendingTasks()

	if allocs := testing.AllocsPerRun(100, func() { buf.Prefetch(0, len(content)) }); allocs != 0 {
		t.Errorf("prefetching a loaded megabyte allocated %v times", allocs)
	}
	data, err := buf.Read(len(content)-16, 16)
	if err != nil || string(data) != "0123456789abcdef" {
		t.Errorf("Read after Prefetch = %q, %v", data, err)
	}
}

// A retire that gave up leaves its straggler reading, and the mapping in
// place. The save that is tried again has to wait for that reader as well:
// telling two generations apart by parity forgot it, and unmapped the file
// while it was still being read.
func TestMappedFile_RetryAfterATimeoutStillWaitsForTheStraggler(t *testing.T) {
	m, _, _, _ := mapTestFile(t, "mapped text")
	straggler := m.pin(nil)

	if m.retire(0, 20*time.Millisecond) {
		t.Fatal("precondition: the first retire should have timed out")
	}
	// A reader of the mapping that came back after the failed release.
	again := m.pin(nil)
	again()
	if m.retire(0, 20*time.Millisecond) {
		t.Fatal("the retried retire did not wait for the reader from before the first one")
	}
	straggler()
	if !m.retire(0, 5*time.Second) {
		t.Fatal("retire still waits with every reader gone")
	}
}

// A link made after the file was opened turns the save into a rewrite in
// place, on any system. Rewriting a mapped file truncates the pages under the
// mapping, and a rewrite that fails halfway leaves no old text to map again,
// so the text is copied first even where the mapping would not stop a rename.
func TestMappedEditor_LinkMadeAfterOpeningIsRewrittenFromACopy(t *testing.T) {
	previous := releaseMappingBeforeReplace
	releaseMappingBeforeReplace = false
	t.Cleanup(func() { releaseMappingBeforeReplace = previous })
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	dir := t.TempDir()
	path, other := filepath.Join(dir, "first.txt"), filepath.Join(dir, "second.txt")
	if err := os.WriteFile(path, []byte("text\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ev := openMappedEditor(t, dir, path)
	defer ev.Close()
	m := ev.Mapped
	if err := os.Link(path, other); err != nil {
		t.Skipf("no hard links here: %v", err)
	}

	// The release runs on a task goroutine and swaps on the UI thread, which
	// is this one.
	type outcome struct {
		release *mappingRelease
		err     error
	}
	done := make(chan outcome, 1)
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		r, err := ev.releaseMappingForSave(ctx, ev.Pt, m, path, true)
		done <- outcome{r, err}
	})
	var res outcome
	deadline := time.After(5 * time.Second)
wait:
	for {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case res = <-done:
			break wait
		case <-deadline:
			t.Fatal("the release did not finish")
		}
	}
	testutil.DrainPendingTasks()
	release, err := res.release, res.err
	if err != nil {
		t.Fatal(err)
	}
	if release == nil || !release.copied || m.isOriginalOf(ev.Pt) {
		t.Fatal("the piece table still reads the mapping of a file about to be rewritten")
	}
	if got := editorText(t, ev); got != "text\n" {
		t.Fatalf("the copy reads %q", got)
	}
}
