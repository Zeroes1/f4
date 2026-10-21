package fileops

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
)

func setCopyThreads(t *testing.T, n int) {
	t.Helper()
	old := config.App.CopyThreads
	config.App.CopyThreads = n
	t.Cleanup(func() { config.App.CopyThreads = old })
}

// One thread is the single-file path whatever the operation is, and so is
// anything that is not a copy between two local file systems.
func TestCopyThreadsFor(t *testing.T) {
	dir := t.TempDir()
	local := vfs.NewOSVFS(dir)
	other := vfs.NewOSVFS(t.TempDir())
	// Anything that is not an *OSVFS: a plugin, an archive or a remote file system.
	remote := &struct{ vfs.VFS }{VFS: local}

	for _, tc := range []struct {
		name    string
		setting int
		src     vfs.VFS
		dst     vfs.VFS
		move    bool
		want    int
	}{
		{"default is one thread", 1, local, other, false, 1},
		{"zero means one", 0, local, other, false, 1},
		{"negative means one", -2, local, other, false, 1},
		{"local copy uses the setting", 4, local, other, false, 4},
		{"the setting is capped", 99, local, other, false, config.MaxCopyThreads},
		{"a move stays on one thread", 4, local, other, true, 1},
		{"a remote source stays on one thread", 4, remote, other, false, 1},
		{"a remote destination stays on one thread", 4, local, remote, false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			setCopyThreads(t, tc.setting)
			if got := copyThreadsFor(tc.src, tc.dst, tc.move); got != tc.want {
				t.Fatalf("copyThreadsFor = %d, want %d", got, tc.want)
			}
		})
	}
}

// Every file the walker hands over runs, never more than the workers at once,
// and Wait returns only after all of them are done.
func TestCopyPoolRunsEveryJobWithinTheWorkerLimit(t *testing.T) {
	const workers, jobs = 4, 200
	pool := newCopyPool(context.Background(), workers)
	var running, peak, done atomic.Int32
	for range jobs {
		ok := pool.Go(func(context.Context) error {
			n := running.Add(1)
			for {
				p := peak.Load()
				if n <= p || peak.CompareAndSwap(p, n) {
					break
				}
			}
			time.Sleep(200 * time.Microsecond)
			running.Add(-1)
			done.Add(1)
			return nil
		})
		if !ok {
			t.Fatal("Go refused a job on a healthy pool")
		}
	}
	if err := pool.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}
	if done.Load() != jobs {
		t.Fatalf("%d of %d jobs ran", done.Load(), jobs)
	}
	if peak.Load() > workers {
		t.Fatalf("%d jobs ran at once, more than %d workers", peak.Load(), workers)
	}
	if peak.Load() < 2 {
		t.Fatalf("jobs never overlapped (peak %d): the pool is not parallel", peak.Load())
	}
}

// The first failure is the one Wait reports; the others, stopped by the
// cancel, do not replace it, and the walker is told to stop.
func TestCopyPoolFirstErrorStopsTheOthers(t *testing.T) {
	pool := newCopyPool(context.Background(), 4)
	boom := errors.New("disk full")
	started := make(chan struct{}, 3)
	var cleaned atomic.Int32

	for range 3 {
		if !pool.Go(func(ctx context.Context) error {
			started <- struct{}{}
			<-ctx.Done() // a long copy, stopped by the failure of another
			cleaned.Add(1)
			return ctx.Err()
		}) {
			t.Fatal("Go refused a job on a healthy pool")
		}
	}
	for range 3 {
		<-started
	}
	if !pool.Go(func(context.Context) error { return boom }) {
		t.Fatal("Go refused the failing job")
	}
	if err := pool.Wait(); !errors.Is(err, boom) {
		t.Fatalf("Wait = %v, want the first error %v", err, boom)
	}
	if cleaned.Load() != 3 {
		t.Fatalf("%d of 3 running jobs saw the stop", cleaned.Load())
	}
}

func TestCopyPoolRefusesJobsAfterAFailure(t *testing.T) {
	pool := newCopyPool(context.Background(), 2)
	boom := errors.New("read error")
	if !pool.Go(func(context.Context) error { return boom }) {
		t.Fatal("Go refused the first job")
	}
	deadline := time.Now().Add(5 * time.Second)
	for !pool.Stopped() {
		if time.Now().After(deadline) {
			t.Fatal("the pool did not stop after a failed job")
		}
		time.Sleep(time.Millisecond)
	}
	var ran atomic.Bool
	if pool.Go(func(context.Context) error { ran.Store(true); return nil }) {
		t.Fatal("Go accepted a job on a stopped pool")
	}
	if err := pool.Wait(); !errors.Is(err, boom) {
		t.Fatalf("Wait = %v, want %v", err, boom)
	}
	if ran.Load() {
		t.Fatal("a job ran after the pool stopped")
	}
}

// A cancel of the whole operation stops the jobs and comes out of Wait even
// though no job failed on its own.
func TestCopyPoolParentCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	pool := newCopyPool(parent, 2)
	started := make(chan struct{})
	if !pool.Go(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}) {
		t.Fatal("Go refused a job")
	}
	<-started
	cancel()
	if err := pool.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait = %v, want context.Canceled", err)
	}
	if pool.Go(func(context.Context) error { return nil }) {
		// Wait closed the jobs channel, so this must not be reached; the
		// guard is the stopped context, checked before the send.
		t.Fatal("Go accepted a job after the parent was cancelled")
	}
}

func TestCopyPoolCancelledBeforeAnyJob(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	pool := newCopyPool(parent, 3)
	if pool.Go(func(context.Context) error { return nil }) {
		t.Fatal("Go accepted a job on a cancelled pool")
	}
	if err := pool.Wait(); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait = %v, want context.Canceled", err)
	}
}

// fakeDir and walkFake mimic the walker: it opens a folder scope per folder, hands the
// files to the pool and gives the scope back once the folder is listed.
type fakeDir struct {
	name  string
	files int
	dirs  []fakeDir
}

func walkFake(pool *copyPool, parent *dirScope, d fakeDir, file func(dir string, i int), finished func(dir string)) {
	scope := newDirScope(pool, parent, func() { finished(d.name) })
	for i := range d.files {
		if !scope.goFile(func(context.Context) error {
			file(d.name, i)
			return nil
		}) {
			return
		}
	}
	for _, sub := range d.dirs {
		walkFake(pool, scope, sub, file, finished)
	}
	scope.done()
}

// A folder is finished after every file of it and of every folder below it,
// and a folder below is finished before the one above.
func TestDirScopeFinishesAFolderAfterEverythingInIt(t *testing.T) {
	tree := fakeDir{name: "root", files: 5, dirs: []fakeDir{
		{name: "a", files: 20, dirs: []fakeDir{{name: "a/x", files: 30}, {name: "a/empty"}}},
		{name: "b", files: 0},
		{name: "c", files: 40, dirs: []fakeDir{{name: "c/y", files: 10, dirs: []fakeDir{{name: "c/y/z", files: 10}}}}},
	}}
	copied := map[string]int{
		"root": 5, "a": 20, "a/x": 30, "a/empty": 0, "b": 0, "c": 40, "c/y": 10, "c/y/z": 10,
	}
	subtree := map[string][]string{
		"root":    {"root", "a", "a/x", "a/empty", "b", "c", "c/y", "c/y/z"},
		"a":       {"a", "a/x", "a/empty"},
		"c":       {"c", "c/y", "c/y/z"},
		"c/y":     {"c/y", "c/y/z"},
		"a/x":     {"a/x"},
		"a/empty": {"a/empty"},
		"b":       {"b"},
		"c/y/z":   {"c/y/z"},
	}

	pool := newCopyPool(context.Background(), 8)
	var mu sync.Mutex
	count := map[string]int{}
	var order []string
	walkFake(pool,
		nil, tree,
		func(dir string, _ int) {
			time.Sleep(50 * time.Microsecond)
			mu.Lock()
			count[dir]++
			mu.Unlock()
		},
		func(dir string) {
			mu.Lock()
			defer mu.Unlock()
			for _, d := range subtree[dir] {
				if count[d] != copied[d] {
					t.Errorf("folder %q finished with %d of %d files of %q copied", dir, count[d], copied[d], d)
				}
			}
			order = append(order, dir)
		})
	if err := pool.Wait(); err != nil {
		t.Fatalf("Wait = %v", err)
	}

	if len(order) != len(copied) {
		t.Fatalf("%d folders finished, want %d: %v", len(order), len(copied), order)
	}
	pos := map[string]int{}
	for i, d := range order {
		if _, dup := pos[d]; dup {
			t.Errorf("folder %q finished twice", d)
		}
		pos[d] = i
	}
	for _, pair := range [][2]string{{"a/x", "a"}, {"a/empty", "a"}, {"a", "root"}, {"c/y/z", "c/y"}, {"c/y", "c"}, {"c", "root"}, {"b", "root"}} {
		if pos[pair[0]] >= pos[pair[1]] {
			t.Errorf("%q finished after %q, its parent: %v", pair[0], pair[1], order)
		}
	}
}

// A folder of a failed copy is left without its attributes, as it is when the
// single-threaded copy stops.
func TestDirScopeDoesNotFinishOnAStoppedPool(t *testing.T) {
	pool := newCopyPool(context.Background(), 2)
	var finished atomic.Bool
	scope := newDirScope(pool, nil, func() { finished.Store(true) })
	boom := errors.New("cannot write")
	if !scope.goFile(func(context.Context) error { return boom }) {
		t.Fatal("goFile refused a job on a healthy pool")
	}
	deadline := time.Now().Add(5 * time.Second)
	for !pool.Stopped() {
		if time.Now().After(deadline) {
			t.Fatal("the pool did not stop")
		}
		time.Sleep(time.Millisecond)
	}
	if scope.goFile(func(context.Context) error { return nil }) {
		t.Fatal("goFile accepted a file on a stopped pool")
	}
	scope.done()
	if err := pool.Wait(); !errors.Is(err, boom) {
		t.Fatalf("Wait = %v, want %v", err, boom)
	}
	if finished.Load() {
		t.Fatal("a folder of a failed copy was finished")
	}
}

// Questions come one at a time however many workers meet a conflict at once.
func TestQuestionGateShowsOneQuestionAtATime(t *testing.T) {
	gate := newQuestionGate()
	var open, peak, shown atomic.Int32
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			asked, err := gate.ask(context.Background(), nil, func() {
				n := open.Add(1)
				if n > peak.Load() {
					peak.Store(n)
				}
				time.Sleep(time.Millisecond)
				shown.Add(1)
				open.Add(-1)
			})
			if err != nil || !asked {
				t.Errorf("ask = %v, %v", asked, err)
			}
		}()
	}
	wg.Wait()
	if peak.Load() != 1 {
		t.Fatalf("%d questions were open together", peak.Load())
	}
	if shown.Load() != 12 {
		t.Fatalf("%d of 12 questions were shown", shown.Load())
	}
}

// "Overwrite all" answered to the first question settles the ones that waited
// behind it: they are not shown again.
func TestQuestionGateAnswerForAllSettlesTheWaitingOnes(t *testing.T) {
	gate := newQuestionGate()
	var mu sync.Mutex
	overwriteAll := false
	settled := func() bool {
		mu.Lock()
		defer mu.Unlock()
		return overwriteAll
	}
	var shown atomic.Int32
	first := make(chan struct{})
	release := make(chan struct{})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = gate.ask(context.Background(), settled, func() {
			shown.Add(1)
			close(first)
			<-release // the user is still deciding
			mu.Lock()
			overwriteAll = true
			mu.Unlock()
		})
	}()
	<-first

	var skipped atomic.Int32
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			asked, err := gate.ask(context.Background(), settled, func() { shown.Add(1) })
			if err != nil {
				t.Errorf("ask = %v", err)
			}
			if !asked {
				skipped.Add(1)
			}
		}()
	}
	time.Sleep(20 * time.Millisecond) // let the others reach the gate
	close(release)
	wg.Wait()

	if shown.Load() != 1 {
		t.Fatalf("%d questions were shown, want 1", shown.Load())
	}
	if skipped.Load() != 8 {
		t.Fatalf("%d of 8 waiting workers were settled by the answer", skipped.Load())
	}
}

// A cancel of the operation frees the workers waiting for the gate.
func TestQuestionGateCancelFreesTheWaiting(t *testing.T) {
	gate := newQuestionGate()
	hold := make(chan struct{})
	in := make(chan struct{})
	go func() {
		_, _ = gate.ask(context.Background(), nil, func() { close(in); <-hold })
	}()
	<-in

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		asked, err := gate.ask(ctx, nil, func() { t.Error("a cancelled question was shown") })
		if asked {
			t.Error("a cancelled question reported as asked")
		}
		result <- err
	}()
	time.Sleep(10 * time.Millisecond)
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ask = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a waiting question did not stop on cancel")
	}
	close(hold)
}

func TestQuestionGateRefusesOnCancelledContext(t *testing.T) {
	gate := newQuestionGate()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	asked, err := gate.ask(ctx, nil, func() { t.Error("question shown on a cancelled operation") })
	if asked || !errors.Is(err, context.Canceled) {
		t.Fatalf("ask = %v, %v; want false, context.Canceled", asked, err)
	}
}
