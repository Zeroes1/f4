package fileops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/unxed/f4/vfs"
)

// newWorkerTestState is the operation state ExecuteFileOp builds, with a
// tracker over total and its progress wrapper, minus the dialog.
func newWorkerTestState(total vfs.OpStats, opts FileOpOptions, report *opReport) *FileOpState {
	tracker := NewFileOpTracker(total)
	wrap := NewGlobalAwareReporter(&DummyReporter{}, func(string) (string, int, string) { return "", 0, "" }, tracker, nil)
	state := &FileOpState{
		Tracker:     tracker,
		StartFile:   wrap.StartFileKnown,
		SetFileSize: wrap.SetCurrentSize,
		OnBytes:     wrap.UpdateBytes,
		Buffer:      make([]byte, 128*1024),
		Report:      report,
		reporter:    wrap,
	}
	opts.applyTo(state, false)
	return state
}

// Workers of one operation share its answers, counters, log and caches
// (f4#1894, part 5): what each of them records adds up exactly, every log
// line is whole, and each worker holding a view has a buffer nobody else
// writes into. Under -race this is the check that nothing shared is touched
// without the operation's lock.
func TestFileOpStateWorkersShareTheOperation(t *testing.T) {
	const workers, rounds = 8, 40
	report := openOpReport("Copy", FileOpOptions{IgnoreReadErrors: true, IgnoreWriteErrors: true}, "/src", []string{"tree"}, "/dst")
	if report.file == nil {
		t.Fatalf("no operation log: %v", report.logErr)
	}
	logPath := report.path
	t.Cleanup(func() { _ = os.Remove(logPath) })
	state := newWorkerTestState(vfs.OpStats{Files: workers * rounds * 2}, FileOpOptions{IgnoreReadErrors: true, IgnoreWriteErrors: true}, report)

	dest := t.TempDir()
	dstVfs := vfs.NewOSVFS(dest)
	folders := []string{filepath.Join(dest, "one"), filepath.Join(dest, "two")}
	for _, f := range folders {
		if err := os.Mkdir(f, 0o750); err != nil {
			t.Fatal(err)
		}
	}

	var held sync.Map // first byte of each buffer in use -> worker
	var views sync.Map
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range rounds {
				v := state.acquireWorker()
				views.Store(v, true)
				if v == state || v.root != state {
					errs <- fmt.Errorf("worker %d got a view that is not of the operation", w)
					return
				}
				if other, busy := held.LoadOrStore(&v.Buffer[0], w); busy {
					errs <- fmt.Errorf("workers %d and %v hold the same buffer", w, other)
					return
				}
				name := fmt.Sprintf("w%d-%d", w, i)
				v.StartFile(name, 10, true, 1, 0)
				v.skipItem("/src/"+name, "/dst/"+name)
				v.StartFile(name+"b", 10, true, 1, 0)
				if !v.tolerateRead("/src/"+name+"b", errors.New("bad sector")) {
					errs <- fmt.Errorf("worker %d: read error not tolerated", w)
					return
				}
				v.fileCopied("/src/"+name+"c", "/dst/"+name+"c")
				v.note("RETRY    %s", name)
				parentRights(context.Background(), v, dstVfs, folders[i%2])
				parentOwner(context.Background(), v, dstVfs, folders[(i+1)%2])
				if w == 3 && i == rounds/2 {
					v.rememberOverwriteAll()
				}
				_, _ = v.existingFilesChoice()
				_ = v.leftBehind()
				held.Delete(&v.Buffer[0])
				state.releaseWorker(v)
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	n := 0
	views.Range(func(any, any) bool { n++; return true })
	if n > workers {
		t.Errorf("%d views made for %d workers, want them reused", n, workers)
	}
	want := workers * rounds
	if state.SkippedCount != want || state.FailedCount != want {
		t.Errorf("skipped=%d failed=%d, want %d each", state.SkippedCount, state.FailedCount, want)
	}
	if overwrite, skip := state.existingFilesChoice(); !overwrite || skip {
		t.Errorf("remembered answer = overwrite %v skip %v, want overwrite all", overwrite, skip)
	}
	if report.copied != want || report.skipped != want || report.failed != want {
		t.Errorf("report copied=%d skipped=%d failed=%d, want %d each", report.copied, report.skipped, report.failed, want)
	}
	if len(state.parentRights) != 2 || len(state.parentOwners) != 2 {
		t.Errorf("caches hold %d rights and %d owners, want both folders once", len(state.parentRights), len(state.parentOwners))
	}
	if processed, _ := state.Tracker.GetStats(); processed.Files != int64(2*want) || processed.Bytes != int64(2*want*10) {
		t.Errorf("tracker %+v, want %d files and %d bytes", processed, 2*want, 2*want*10)
	}

	report.close(nil)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	line := regexp.MustCompile(`^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d  (OK|SKIPPED|FAILED|RETRY) +\S`)
	counts := map[string]int{}
	for _, l := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		if !strings.Contains(l, "/src/w") && !strings.Contains(l, "RETRY") {
			continue // the header and the footer
		}
		m := line.FindStringSubmatch(l)
		if m == nil {
			t.Fatalf("log line is not whole: %q", l)
		}
		counts[m[1]]++
	}
	for _, kind := range []string{"OK", "SKIPPED", "FAILED", "RETRY"} {
		if counts[kind] != want {
			t.Errorf("%d %s lines in the log, want %d", counts[kind], kind, want)
		}
	}
}

// The real copy of a file runs on worker views at once: several subtrees
// copied in parallel through recursiveCopy, with existing files skipped,
// permissions inherited from the folders and every byte and file counted,
// end up exactly as the single-threaded copy would leave them.
func TestRecursiveCopyOnWorkerViewsAtOnce(t *testing.T) {
	const subtrees, files = 6, 12
	srcRoot, dstRoot := t.TempDir(), t.TempDir()
	srcVfs, dstVfs := vfs.NewOSVFS(srcRoot), vfs.NewOSVFS(dstRoot)
	var preexisting, totalBytes int64
	for s := range subtrees {
		dir := filepath.Join(srcRoot, fmt.Sprintf("tree%d", s), "inner")
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
		for f := range files {
			data := bytes.Repeat([]byte{byte('a' + f)}, 1000*(f+1)+s)
			totalBytes += int64(len(data))
			if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.bin", f)), data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		// One file of every subtree is already there and must be kept.
		kept := filepath.Join(dstRoot, fmt.Sprintf("tree%d", s), "inner")
		if err := os.MkdirAll(kept, 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(kept, "f0.bin"), []byte("mine"), 0o600); err != nil {
			t.Fatal(err)
		}
		preexisting++
	}
	stats, err := vfs.CalculateStats(context.Background(), srcVfs, srcRoot, func() []string {
		var names []string
		for s := range subtrees {
			names = append(names, fmt.Sprintf("tree%d", s))
		}
		return names
	}(), nil)
	if err != nil {
		t.Fatal(err)
	}
	state := newWorkerTestState(stats, FileOpOptions{ExistingFiles: ExistingFilesSkip, AccessRights: AccessRightsInherit}, nil)

	var wg sync.WaitGroup
	errs := make(chan error, subtrees)
	for s := range subtrees {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := state.acquireWorker()
			defer state.releaseWorker(v)
			name := fmt.Sprintf("tree%d", s)
			if err := recursiveCopy(context.Background(), srcVfs, filepath.Join(srcRoot, name), dstVfs, filepath.Join(dstRoot, name), v, 0); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	for s := range subtrees {
		for f := range files {
			rel := filepath.Join(fmt.Sprintf("tree%d", s), "inner", fmt.Sprintf("f%d.bin", f))
			got, err := os.ReadFile(filepath.Join(dstRoot, rel))
			if err != nil {
				t.Fatal(err)
			}
			if f == 0 {
				if string(got) != "mine" {
					t.Errorf("%s was overwritten, want the existing file kept", rel)
				}
				continue
			}
			want, _ := os.ReadFile(filepath.Join(srcRoot, rel))
			if !bytes.Equal(got, want) {
				t.Errorf("%s differs from its source", rel)
			}
		}
	}
	if int64(state.SkippedCount) != preexisting {
		t.Errorf("skipped %d, want the %d existing files", state.SkippedCount, preexisting)
	}
	processed, total := state.Tracker.GetStats()
	if processed.Files != total.Files || processed.Dirs != total.Dirs || processed.Bytes != totalBytes {
		t.Errorf("progress %+v of %+v, want every file, folder and byte (%d) counted once", processed, total, totalBytes)
	}
	if _, pct, name := state.Tracker.GetProgress(); pct != 100 || name != "" {
		t.Errorf("progress at the end: %d%% showing %q, want 100%% and no file", pct, name)
	}
}

// The server-to-server choice is the operation's: a direction found by one
// file is not undone by another that found none, and once the saved password
// was needed it stays needed.
func TestFileOpStateS2SChoiceIsShared(t *testing.T) {
	state := &FileOpState{}
	v := state.acquireWorker()
	v.noteS2S(2, true)
	v.disableS2SIfUnprobed()
	state.noteS2S(2, false)
	if dir, password := v.s2sState(); dir != 2 || !password {
		t.Fatalf("s2s = %d, %v; want pull with the password", dir, password)
	}
	fresh := &FileOpState{}
	fresh.acquireWorker().disableS2SIfUnprobed()
	if fresh.S2SDir != 3 {
		t.Fatalf("S2SDir = %d after a file found no way, want 3 (disabled)", fresh.S2SDir)
	}
}

// A view without the progress wrapper (a state built by hand) still counts
// into the operation's tracker through a lane, and a view given back drops a
// file its worker left half done.
func TestFileOpStateWorkerWithoutWrapper(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 100})
	state := &FileOpState{Tracker: tracker, Buffer: make([]byte, 1024)}
	v := state.acquireWorker()
	if v.Tracker == tracker || v.StartFile != nil || len(v.Buffer) != 1024 {
		t.Fatalf("view: own tracker %v, StartFile %v, buffer %d; want a lane, no callback, the operation's buffer size",
			v.Tracker != tracker, v.StartFile != nil, len(v.Buffer))
	}
	v.Tracker.StartFile("half", 100)
	v.Tracker.UpdateBytes(50)
	state.releaseWorker(v)
	if _, _, name := tracker.GetProgress(); name != "" {
		t.Fatalf("given back view still shows %q", name)
	}
	if again := state.acquireWorker(); again != v {
		t.Fatal("a view given back was not reused")
	}
	state.releaseWorker(state) // the operation's own state is not a view
	if len(state.idle) != 0 {
		t.Fatal("the operation's own state was taken as an idle view")
	}
}
