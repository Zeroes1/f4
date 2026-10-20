package fileops

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/vfs"
)

// Building blocks of the multi-threaded copy engine (f4#1894, part 5):
//
//   - copyPool runs the files a single walker hands it on N workers and turns
//     the first failure or a cancel into one stop for all of them;
//   - dirScope tells the walker's folders when every file under them is done,
//     so a folder's attributes are set last, as in the single-threaded copy;
//   - questionGate lets only one question be on the screen at a time and lets
//     an "all" answer settle the questions that waited behind it.
//
// copyThreadsFor decides whether an operation uses them at all. With one
// thread, which is the default, recursiveCopy keeps its single-file path and
// none of this runs.

// copyThreadsFor returns how many files an operation may copy at once. The
// engine takes only a copy (not a move yet) between two local file systems; a
// remote VFS, an archive or a plugin one is not known to be safe from several
// goroutines, so anything else stays on one thread.
func copyThreadsFor(srcVfs, dstVfs vfs.VFS, isMove bool) int {
	n := config.ClampCopyThreads(config.App.CopyThreads)
	if n == 1 || isMove {
		return 1
	}
	if _, ok := srcVfs.(*vfs.OSVFS); !ok {
		return 1
	}
	if _, ok := dstVfs.(*vfs.OSVFS); !ok {
		return 1
	}
	return n
}

// copyPool runs file jobs on a fixed number of workers.
//
// The first job that fails records its error and cancels the context all the
// jobs run with; the others see the cancel, stop and clean up after
// themselves, and jobs not started yet are dropped. Wait reports the first
// error, or the parent's cancel if no job failed first.
//
// Go and Wait are for the one goroutine that walks the tree: Go may not be
// called after Wait, and Wait is called once.
type copyPool struct {
	ctx    context.Context
	cancel context.CancelCauseFunc
	jobs   chan func(ctx context.Context) error
	wg     sync.WaitGroup

	mu  sync.Mutex
	err error
}

func newCopyPool(parent context.Context, workers int) *copyPool {
	ctx, cancel := context.WithCancelCause(parent)
	p := &copyPool{
		ctx:    ctx,
		cancel: cancel,
		// Unbuffered: the walker waits until a worker is free, so it never
		// reads far ahead of the files being copied (a tree of millions of
		// names must not be queued in memory) and a failure is noticed by it
		// within one file.
		jobs: make(chan func(ctx context.Context) error),
	}
	for range max(workers, 1) {
		p.wg.Add(1)
		go p.work()
	}
	return p
}

func (p *copyPool) work() {
	defer p.wg.Done()
	for job := range p.jobs {
		if p.ctx.Err() != nil {
			continue // stopped: take the remaining jobs off the walker without running them
		}
		if err := job(p.ctx); err != nil {
			p.fail(err)
		}
	}
}

// fail records err if it is the first and stops every job. The error is
// stored before the cancel, so a job that fails only because of the cancel
// cannot take the first place.
func (p *copyPool) fail(err error) {
	p.mu.Lock()
	if p.err == nil {
		p.err = err
	}
	p.mu.Unlock()
	p.cancel(err)
}

// Stopped reports whether a job has failed or the parent context is done.
func (p *copyPool) Stopped() bool { return p.ctx.Err() != nil }

// Go hands job to a free worker, waiting for one. It returns false, and does
// not run job, once the pool has stopped: the walker should stop too and call
// Wait for the reason.
func (p *copyPool) Go(job func(ctx context.Context) error) bool {
	if p.ctx.Err() != nil {
		return false
	}
	select {
	case p.jobs <- job:
		return true
	case <-p.ctx.Done():
		return false
	}
}

// Wait lets the running jobs finish and returns the first error.
func (p *copyPool) Wait() error {
	close(p.jobs)
	p.wg.Wait()
	ctxErr := p.ctx.Err()
	p.cancel(nil)
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	return ctxErr
}

// dirScope counts what a folder of the walk is still waiting for: its files
// and its subfolders. When the count reaches zero, and only then, finish runs
// (a folder's permissions and times are set after everything in it, so a
// read-only source folder cannot stop its own files from being written), and
// the folder lets its parent know.
//
// finish does not run once the pool has stopped: a failed or cancelled copy
// leaves its folders as the single-threaded copy does, without attributes.
type dirScope struct {
	pool    *copyPool
	parent  *dirScope
	finish  func()
	pending atomic.Int64
}

// newDirScope opens a folder. The walker holds the first count and gives it
// back with done once it has handed over every entry of the folder.
func newDirScope(pool *copyPool, parent *dirScope, finish func()) *dirScope {
	s := &dirScope{pool: pool, parent: parent, finish: finish}
	s.pending.Store(1)
	if parent != nil {
		parent.add()
	}
	return s
}

func (s *dirScope) add() { s.pending.Add(1) }

func (s *dirScope) done() {
	if s.pending.Add(-1) != 0 {
		return
	}
	if s.finish != nil && !s.pool.Stopped() {
		s.finish()
	}
	if s.parent != nil {
		s.parent.done()
	}
}

// goFile runs a file of this folder on the pool. It reports false, with the
// file not run and the folder's count intact, when the pool has stopped.
func (s *dirScope) goFile(job func(ctx context.Context) error) bool {
	s.add()
	ok := s.pool.Go(func(ctx context.Context) error {
		defer s.done()
		return job(ctx)
	})
	if !ok {
		s.pending.Add(-1)
	}
	return ok
}

// questionGate lets one question at a time reach the user. A worker that
// meets a conflict takes the gate, and the others that meet one meanwhile
// wait for it instead of piling dialogs on the screen; their files that have
// no conflict go on copying.
type questionGate struct {
	slot chan struct{}
}

func newQuestionGate() *questionGate {
	return &questionGate{slot: make(chan struct{}, 1)}
}

// ask runs show, the dialog, once the gate is free. settled is asked again
// after the wait: an answer such as "overwrite all" given to the previous
// question may already decide this one, and then show does not run. It
// returns whether show ran, or the context's error when the operation was
// cancelled while the question waited.
func (g *questionGate) ask(ctx context.Context, settled func() bool, show func()) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	select {
	case g.slot <- struct{}{}:
	case <-ctx.Done():
		return false, ctx.Err()
	}
	defer func() { <-g.slot }()
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if settled != nil && settled() {
		return false, nil
	}
	show()
	return true, nil
}
