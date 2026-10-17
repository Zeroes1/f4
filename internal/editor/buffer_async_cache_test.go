package editor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// smallChunkBuffer opens content through an AsyncBuffer whose chunks are small
// enough that a test file spans hundreds of them, and so a cache of a few
// dozen of them is a small part of the file.
func smallChunkBuffer(t *testing.T, content string, wrap func(vfs.ReadAtCloser) vfs.ReadAtCloser) *AsyncBuffer {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := vfs.NewOSVFS(dir).Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	file := f
	if wrap != nil {
		file = wrap(f)
	}
	buf := NewAsyncBuffer(context.Background(), file)
	buf.chunkSize = 1024
	buf.limit = 1 // raised to the smallest the buffer allows: 32 chunks
	t.Cleanup(buf.Close)
	return buf
}

// readWhenLoaded reads from buf, waiting for the fetches the read starts.
func readWhenLoaded(t *testing.T, buf *AsyncBuffer, offset, length int) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		data, err := buf.Read(offset, length)
		if err == nil {
			return data
		}
		if err != piecetable.ErrLoading {
			t.Fatalf("Read(%d, %d): %v", offset, length, err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("Read(%d, %d) never finished loading", offset, length)
		}
		time.Sleep(time.Millisecond)
	}
}

// Reading a file through the buffer once used to leave all of it on the heap
// for as long as the editor stayed open. It keeps the chunks used last now,
// and fetches the others again when they are wanted.
func TestAsyncBuffer_KeepsOnlyTheChunksUsedLast(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	stop := pumpUITasks()
	defer stop()

	content := strings.Repeat("0123456789abcdef", 32*1024) // 512 KB
	buf := smallChunkBuffer(t, content, nil)
	limit := buf.cacheLimit()
	if limit >= len(content)/4 {
		t.Fatalf("precondition: a cache of %d bytes holds much of a %d byte file", limit, len(content))
	}

	for off := 0; off < len(content); off += 4096 {
		data := readWhenLoaded(t, buf, off, 4096)
		if string(data) != content[off:off+4096] {
			t.Fatalf("the buffer read the wrong bytes at %d", off)
		}
		if held := buf.LoadedBytes(); held > limit {
			t.Fatalf("after reading to %d the buffer holds %d bytes, over its limit of %d", off, held, limit)
		}
	}

	// The start of the file was evicted long ago; it comes back.
	buf.mu.Lock()
	_, kept := buf.loaded[0]
	buf.mu.Unlock()
	if kept {
		t.Error("the first chunk outlived a pass over the whole file")
	}
	if data := readWhenLoaded(t, buf, 10, 20); string(data) != content[10:30] {
		t.Errorf("an evicted chunk read back as %q", data)
	}
}

// What was used last stays: a chunk read over and over survives a pass over
// the rest of the file, where a buffer evicting the oldest fetch would have
// dropped it first.
func TestAsyncBuffer_EvictsTheLeastRecentlyUsed(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	stop := pumpUITasks()
	defer stop()

	content := strings.Repeat("x", 256*1024)
	buf := smallChunkBuffer(t, content, nil)
	readWhenLoaded(t, buf, 0, 10)
	for off := 1024; off < len(content); off += 1024 {
		readWhenLoaded(t, buf, off, 1024)
		readWhenLoaded(t, buf, 0, 10) // the line on screen, repainted
	}
	buf.mu.Lock()
	_, kept := buf.loaded[0]
	buf.mu.Unlock()
	if !kept {
		t.Error("the chunk in use was evicted")
	}
}

// concurrencyFile counts ReadAt calls, and how many of them ran at once.
type concurrencyFile struct {
	vfs.ReadAtCloser
	mu      sync.Mutex
	calls   int
	running int
	peak    int
}

func (c *concurrencyFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	c.mu.Lock()
	c.calls++
	c.running++
	c.peak = max(c.peak, c.running)
	c.mu.Unlock()
	time.Sleep(2 * time.Millisecond) // long enough for fetches to overlap
	n, err := c.ReadAtCloser.ReadAt(ctx, p, off)
	c.mu.Lock()
	c.running--
	c.mu.Unlock()
	return n, err
}

// A network file system pays a round trip per request. Neighbouring chunks
// are asked for in one request, and only a few requests run at once, where
// every 32 KB chunk had a request and a goroutine of its own.
func TestAsyncBuffer_FetchesNeighboursTogether(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	stop := pumpUITasks()
	defer stop()

	content := strings.Repeat("y", 64*1024)
	var file *concurrencyFile
	buf := smallChunkBuffer(t, content, func(f vfs.ReadAtCloser) vfs.ReadAtCloser {
		file = &concurrencyFile{ReadAtCloser: f}
		return file
	})
	buf.limit = 0 // all 64 chunks fit

	readWhenLoaded(t, buf, 0, len(content)) // 64 chunks
	file.mu.Lock()
	calls, peak := file.calls, file.peak
	file.mu.Unlock()
	if want := 64 / asyncMaxRun; calls != want {
		t.Errorf("64 chunks took %d requests, want %d", calls, want)
	}
	if peak > asyncMaxFetches {
		t.Errorf("%d requests ran at once, more than the %d allowed", peak, asyncMaxFetches)
	}
}

// Asking for the whole file at once — a piece table's Bytes — must not start
// a fetch for every chunk of it: they would be fetched only to evict each
// other.
func TestAsyncBuffer_AWholeFileReadFetchesOnlyWhatFits(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	content := strings.Repeat("z", 1024*1024)
	f := &stalledFile{size: int64(len(content)), release: make(chan struct{})}
	defer close(f.release)
	buf := NewAsyncBuffer(context.Background(), f)
	buf.chunkSize = 1024
	buf.limit = 1
	defer buf.Close()

	if _, err := buf.Read(0, len(content)); err != piecetable.ErrLoading {
		t.Fatalf("Read = %v, want ErrLoading", err)
	}
	buf.mu.Lock()
	fetching := len(buf.fetching)
	buf.mu.Unlock()
	if most := buf.cacheLimit() / 2 / buf.chunkSize; fetching > most {
		t.Errorf("one read started fetches for %d chunks, more than the %d the cache can spare", fetching, most)
	}
}

// A range larger than the cache could never be read through it: its chunks
// evicted each other before the last of them arrived, and every retry failed.
// It is read in one piece instead, and the cache stays within its limit.
func TestAsyncBuffer_ARangeLargerThanTheCacheIsReadWhole(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	stop := pumpUITasks()
	defer stop()

	content := strings.Repeat("0123456789abcdef", 16*1024) // 256 KB
	buf := smallChunkBuffer(t, content, nil)
	if len(content) <= buf.cacheLimit() {
		t.Fatal("precondition: the range has to be larger than the cache")
	}
	if got := readWhenLoaded(t, buf, 0, len(content)); string(got) != content {
		t.Fatal("the whole-file read returned the wrong bytes")
	}
	if held := buf.LoadedBytes(); held > buf.cacheLimit() {
		t.Errorf("the buffer holds %d bytes, over its limit of %d", held, buf.cacheLimit())
	}
}

// A read waiting for the rest of its range keeps the part it already has:
// left with their old stamps, those chunks were the next to be evicted while
// it waited, and had to be fetched again.
func TestAsyncBuffer_AWaitingReadKeepsWhatItHas(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	stop := pumpUITasks()
	defer stop()

	content := strings.Repeat("w", 128*1024) // 128 chunks; the cache holds 32
	buf := smallChunkBuffer(t, content, nil)
	k := buf.chunkSize
	readWhenLoaded(t, buf, 0, 8*k)     // chunks 0..7
	readWhenLoaded(t, buf, 32*k, 24*k) // 32..55, used after them
	if _, err := buf.Read(0, 16*k); err != piecetable.ErrLoading {
		t.Fatalf("precondition: chunks 8..15 should be missing, got %v", err)
	}
	readWhenLoaded(t, buf, 8*k, 8*k)  // 8..15 arrive
	readWhenLoaded(t, buf, 56*k, 8*k) // 56..63 push the cache over its limit

	buf.mu.Lock()
	_, kept := buf.loaded[0]
	buf.mu.Unlock()
	if !kept {
		t.Error("the start of the range being waited for was evicted")
	}
}

// gatedFile holds every ReadAt until it is let through, and reports the
// offset of each one as it starts.
type gatedFile struct {
	vfs.ReadAtCloser
	started chan int64
	gate    chan struct{}
}

func (g *gatedFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	g.started <- off
	select {
	case <-g.gate:
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	return g.ReadAtCloser.ReadAt(ctx, p, off)
}

// With every turn taken, the run asked for last goes first, and a run nobody
// has asked about for a while is dropped: holding PgDn through a remote file
// downloaded every page scrolled past before the one on screen.
func TestAsyncBuffer_FetchesWhatWasAskedForLastFirst(t *testing.T) {
	previous := asyncStaleAsk
	asyncStaleAsk = 200 * time.Millisecond
	t.Cleanup(func() { asyncStaleAsk = previous })
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	stop := pumpUITasks()
	defer stop()

	content := strings.Repeat("v", 512*1024)
	var file *gatedFile
	buf := smallChunkBuffer(t, content, func(f vfs.ReadAtCloser) vfs.ReadAtCloser {
		file = &gatedFile{ReadAtCloser: f, started: make(chan int64, 16), gate: make(chan struct{})}
		return file
	})
	t.Cleanup(buf.Close)
	k := buf.chunkSize
	next := func() int64 {
		t.Helper()
		select {
		case off := <-file.started:
			return off
		case <-time.After(5 * time.Second):
			t.Fatal("no fetch started")
			return -1
		}
	}

	for i := 0; i < asyncMaxFetches; i++ { // every turn taken
		buf.Prefetch(i*100*k, k)
		next()
	}
	buf.Prefetch(400*k, k) // scrolled past
	time.Sleep(10 * time.Millisecond)
	buf.Prefetch(450*k, k) // on screen
	file.gate <- struct{}{}
	if got := next(); got != int64(450*k) {
		t.Fatalf("the next fetch read offset %d, want the one asked for last, %d", got, 450*k)
	}

	time.Sleep(2 * asyncStaleAsk)
	file.gate <- struct{}{}
	select {
	case off := <-file.started:
		t.Fatalf("a run nobody asked about again was fetched anyway, at %d", off)
	case <-time.After(100 * time.Millisecond):
	}
	buf.mu.Lock()
	still := buf.fetching[400]
	buf.mu.Unlock()
	if still {
		t.Error("the dropped run's chunk is still marked as on its way")
	}
}
