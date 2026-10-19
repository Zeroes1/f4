package editor

import "context"

// indexReadAhead reads the file for the line scan one chunk ahead of it, so
// that copying a chunk out of the file and looking through the one before it
// for newlines happen at the same time rather than by turns. On a 1 GB log
// they took about as long as each other.
//
// What read hands back stays valid until the next call: the read started then
// goes into the other of its two buffers.
type indexReadAhead struct {
	ctx  context.Context
	read fileChunkReader
	bufs [2]*[]byte
	// cur is the buffer holding what read last handed back.
	cur int

	// The read running into the other buffer, if any, and what it asked for.
	pending           chan readAheadResult
	pendingAt, length int
}

type readAheadResult struct {
	n  int
	ok bool
}

// newIndexReadAhead borrows its two buffers from indexScanBuffers; close
// returns them.
func newIndexReadAhead(ctx context.Context, read fileChunkReader) *indexReadAhead {
	r := &indexReadAhead{ctx: ctx, read: read}
	for i := range r.bufs {
		b, _ := indexScanBuffers.Get().(*[]byte)
		if b == nil {
			s := make([]byte, indexReadChunk)
			b = &s
		}
		r.bufs[i] = b
	}
	return r
}

// readAt answers the length bytes of text at offset, read from the file, or
// false when that stretch has to be read through the piece table. It then
// starts reading the chunk after it, up to end.
func (r *indexReadAhead) readAt(offset, length, end int) ([]byte, bool) {
	var res readAheadResult
	if r.pending != nil && r.pendingAt == offset && r.length == length {
		res = <-r.pending
		r.pending = nil
		r.cur = 1 - r.cur
	} else {
		r.wait()
		n, ok := r.read(r.ctx, *r.bufs[r.cur], offset, length)
		res = readAheadResult{n, ok}
	}
	if !res.ok {
		return nil, false
	}
	if next := offset + res.n; next < end {
		r.start(next, min(indexReadChunk, end-next))
	}
	return (*r.bufs[r.cur])[:res.n], true
}

// start reads length bytes at offset into the buffer read is not handing out.
func (r *indexReadAhead) start(offset, length int) {
	done := make(chan readAheadResult, 1)
	r.pending, r.pendingAt, r.length = done, offset, length
	dst := *r.bufs[1-r.cur]
	go func() {
		n, ok := r.read(r.ctx, dst, offset, length)
		done <- readAheadResult{n, ok}
	}()
}

// wait lets a read that is no longer wanted finish, so its buffer can be
// written again.
func (r *indexReadAhead) wait() {
	if r.pending != nil {
		<-r.pending
		r.pending = nil
	}
}

func (r *indexReadAhead) close() {
	r.wait()
	for _, b := range r.bufs {
		indexScanBuffers.Put(b)
	}
}
