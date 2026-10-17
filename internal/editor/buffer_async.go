package editor

import (
	"bytes"
	"context"
	"io"
	"sync"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// asyncChunkSize is the unit an AsyncBuffer fetches and keeps the file in.
// It was 32 KB, which over a network file system is one round trip per 32 KB
// of text: a remote file loaded at the speed of its latency, not of its
// bandwidth.
const asyncChunkSize = 256 << 10

// asyncMaxRun bounds how many neighbouring missing chunks one fetch reads in a
// single ReadAt: a reader asking for 4 MB gets one request for it rather than
// sixteen.
const asyncMaxRun = 16

// asyncMaxFetches bounds how many fetches run at once. Each used to have its
// own goroutine with nothing to stop them piling up: asking for a whole
// gigabyte started thirty thousand of them.
const asyncMaxFetches = 4

// asyncCacheLimit is how much of the file an AsyncBuffer keeps. It used to
// keep every chunk it ever fetched, so reading a large file through it once
// — a search, a save — left the whole file on the heap for as long as the
// editor stayed open. A var so the tests can make eviction happen.
var asyncCacheLimit = 64 << 20

// AsyncBuffer provides non-blocking access to a file, returning ErrLoading
// and triggering background fetches when data is missing. It keeps the
// chunks used most recently, up to asyncCacheLimit; anything older is
// fetched again when it is asked for again.
type AsyncBuffer struct {
	File       vfs.ReadAtCloser
	size       int
	fileOffset int64
	ctx        context.Context
	cancelCtx  context.CancelFunc

	mu        sync.Mutex
	loaded    map[int][]byte // Chunk index -> Data
	fetching  map[int]bool   // Chunk index -> is currently being fetched
	chunkSize int
	// lastUse is when each loaded chunk was last read, in ticks of clock;
	// the chunk with the smallest is the one evicted.
	lastUse     map[int]uint64
	clock       uint64
	loadedBytes int
	// limit is the most loadedBytes may hold; zero means asyncCacheLimit.
	limit int
	// slots holds one token per fetch that is running.
	slots chan struct{}
}

func NewAsyncBuffer(ctx context.Context, f vfs.ReadAtCloser) *AsyncBuffer {
	return NewAsyncBufferWithOffset(ctx, f, 0)
}

// NewAsyncBufferWithOffset presents the file from fileOffset onward. This is
// used for text files whose on-disk UTF-8 BOM is metadata rather than editor
// content; all buffer offsets remain logical offsets into the displayed text.
func NewAsyncBufferWithOffset(ctx context.Context, f vfs.ReadAtCloser, fileOffset int64) *AsyncBuffer {
	if fileOffset < 0 {
		fileOffset = 0
	}
	rawSize := int64(0)
	if f != nil {
		rawSize = f.Size()
	}
	size := rawSize - fileOffset
	if size < 0 {
		size = 0
	}
	bCtx, bCancel := context.WithCancel(ctx)
	return &AsyncBuffer{
		File:       f,
		size:       int(size),
		fileOffset: fileOffset,
		ctx:        bCtx,
		cancelCtx:  bCancel,
		loaded:     make(map[int][]byte),
		fetching:   make(map[int]bool),
		chunkSize:  asyncChunkSize,
	}
}

func (b *AsyncBuffer) Close() {
	if b.cancelCtx != nil {
		b.cancelCtx()
	}
}

func (b *AsyncBuffer) Size() int {
	return b.size
}

// FileOffset reports where in the file the buffer's contents begin: zero, or
// the length of a UTF-8 byte-order mark that is not part of the text.
func (b *AsyncBuffer) FileOffset() int64 {
	return b.fileOffset
}

// ChunkSize is the unit the buffer fetches and keeps the file in.
func (b *AsyncBuffer) ChunkSize() int {
	return b.chunkSize
}

// LoadedBytes reports how much of the file the buffer holds right now.
func (b *AsyncBuffer) LoadedBytes() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.loadedBytes
}

// cacheLimit is the most the buffer keeps, and never less than a few fetches'
// worth: a limit below what one reader asks for at once would evict the start
// of its range while the end was still arriving, and it would wait forever.
func (b *AsyncBuffer) cacheLimit() int {
	limit := b.limit
	if limit <= 0 {
		limit = asyncCacheLimit
	}
	return max(limit, 2*asyncMaxRun*b.chunkSize)
}

// Prefetch starts fetching whatever of [offset, offset+length) is neither
// loaded nor on its way, and returns at once. It is what a reader asks for to
// get ahead of itself: Read would do the same and then assemble a copy of the
// range only to have it thrown away.
func (b *AsyncBuffer) Prefetch(offset, length int) {
	if offset < 0 || offset >= b.size || length <= 0 {
		return
	}
	if offset+length > b.size {
		length = b.size - offset
	}
	startChunk := offset / b.chunkSize
	endChunk := (offset + length - 1) / b.chunkSize

	b.mu.Lock()
	defer b.mu.Unlock()
	b.fetchMissingLocked(startChunk, endChunk)
}

// fetchMissingLocked starts fetches for the chunks of [first, last] that are
// neither loaded nor on their way, neighbours together in one request, and no
// more of them than half the cache holds: a caller asking for the whole file
// at once — a piece table's Bytes — gets the start of it fetched, not every
// chunk of it fetched and evicted in turn. Loaded chunks in the range count
// as used.
func (b *AsyncBuffer) fetchMissingLocked(first, last int) {
	// Read the global here, on the goroutine that starts the fetch, rather
	// than inside it: the fetch outlives this call and would otherwise race
	// anything that reassigns vtui.FrameManager while it is still in flight.
	frames := vtui.FrameManager
	budget := b.cacheLimit() / 2 / b.chunkSize
	for i := first; i <= last && budget > 0; {
		if _, ok := b.loaded[i]; ok {
			b.touchLocked(i)
			i++
			continue
		}
		if b.fetching[i] {
			i++
			continue
		}
		run := 0
		for i+run <= last && run < asyncMaxRun && run < budget {
			if _, ok := b.loaded[i+run]; ok || b.fetching[i+run] {
				break
			}
			b.fetching[i+run] = true
			run++
		}
		go b.fetchRun(i, run, frames)
		budget -= run
		i += run
	}
}

// Prewarm synchronously loads the first chunk so the first render
// never sees ErrLoading, avoiding a brief [Loading...] flash.
func (b *AsyncBuffer) Prewarm() {
	if b.size == 0 {
		return
	}
	sz := b.chunkSize
	if sz > b.size {
		sz = b.size
	}
	data := make([]byte, sz)
	n, err := b.File.ReadAt(b.ctx, data, b.fileOffset)
	if b.ctx.Err() != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err == nil || err == io.EOF {
		b.storeLocked(0, data[:n])
	}
}

func (b *AsyncBuffer) Read(offset, length int) ([]byte, error) {
	if offset < 0 || offset >= b.size || length <= 0 {
		return nil, nil
	}
	if offset+length > b.size {
		length = b.size - offset
	}

	startChunk := offset / b.chunkSize
	endChunk := (offset + length - 1) / b.chunkSize

	b.mu.Lock()
	defer b.mu.Unlock()
	for i := startChunk; i <= endChunk; i++ {
		if _, ok := b.loaded[i]; !ok {
			b.fetchMissingLocked(i, endChunk)
			return nil, piecetable.ErrLoading
		}
	}

	res := make([]byte, 0, length)
	for i := startChunk; i <= endChunk; i++ {
		data := b.loaded[i]
		b.touchLocked(i)
		cStart := i * b.chunkSize
		takeStart := max(offset-cStart, 0)
		takeEnd := min(offset+length-cStart, len(data))
		if takeEnd > takeStart {
			res = append(res, data[takeStart:takeEnd]...)
		}
	}
	return res, nil
}

// touchLocked marks a loaded chunk as just used.
func (b *AsyncBuffer) touchLocked(idx int) {
	if b.lastUse == nil {
		b.lastUse = make(map[int]uint64)
	}
	b.clock++
	b.lastUse[idx] = b.clock
}

// storeLocked keeps a fetched chunk and evicts the least recently used ones
// until the buffer is back within its limit. The chunk just stored is never
// the one evicted: it is what a reader is waiting for.
func (b *AsyncBuffer) storeLocked(idx int, data []byte) {
	if old, ok := b.loaded[idx]; ok {
		b.loadedBytes -= len(old)
	}
	b.loaded[idx] = data
	b.loadedBytes += len(data)
	b.touchLocked(idx)

	limit := b.cacheLimit()
	for b.loadedBytes > limit && len(b.loaded) > 1 {
		victim, oldest := -1, uint64(0)
		for i := range b.loaded {
			if i == idx {
				continue
			}
			if use := b.lastUse[i]; victim < 0 || use < oldest {
				victim, oldest = i, use
			}
		}
		b.loadedBytes -= len(b.loaded[victim])
		delete(b.loaded, victim)
		delete(b.lastUse, victim)
	}
}

// fetchRun reads count chunks from first on in one request, once a slot is
// free, and keeps each as a slice of its own: a chunk sharing its array with
// its neighbours would keep all of them on the heap after it alone was
// evicted.
func (b *AsyncBuffer) fetchRun(first, count int, frames *vtui.FrameManagerType) {
	defer frames.PostTask(func() {
		frames.Redraw()
	})

	b.mu.Lock()
	if b.slots == nil {
		b.slots = make(chan struct{}, asyncMaxFetches)
	}
	slots := b.slots
	b.mu.Unlock()

	var data []byte
	var err error
	select {
	case slots <- struct{}{}:
		off := int64(first) * int64(b.chunkSize)
		data = make([]byte, min(int64(count)*int64(b.chunkSize), int64(b.size)-off))
		var n int
		n, err = b.File.ReadAt(b.ctx, data, b.fileOffset+off)
		data = data[:max(n, 0)]
		<-slots
	case <-b.ctx.Done():
		err = b.ctx.Err()
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	for i := first; i < first+count; i++ {
		delete(b.fetching, i)
	}
	if b.ctx.Err() != nil || (err != nil && err != io.EOF) {
		if err != nil && err != context.Canceled {
			// Left unloaded, so the next read asks again.
			vtui.DebugLog("AsyncBuffer: failed to fetch chunks %d..%d: %v", first, first+count-1, err)
		}
		return
	}
	for i := first; i < first+count; i++ {
		start := (i - first) * b.chunkSize
		want := min(b.chunkSize, b.size-i*b.chunkSize)
		if start+want > len(data) {
			// A short read: this chunk and the rest are fetched again when
			// they are asked for.
			break
		}
		b.storeLocked(i, bytes.Clone(data[start:start+want]))
	}
}
