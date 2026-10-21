package fileops

import (
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

// FileOpTracker aggregates statistics from a running file operation.
// It is thread-safe and provides normalized progress data for the UI.
//
// A single-threaded operation has one file in work at a time, and the
// tracker's own methods report it. A multi-threaded copy (f4#1894, part 5)
// gives each worker a lane (see Lane): a lane has a file of its own in work
// and counts into the same totals, so several files can be in work at once
// without one worker's StartFile or FileDone taking over another's file.
type FileOpTracker struct {
	mu sync.RWMutex

	total     vfs.OpStats // Constant results from pre-scan
	processed vfs.OpStats // Accumulated results of finished items

	// files holds the file in work of every lane; files[0] is the tracker's
	// own. A lane whose file has no name has none in work.
	files  []trackedFile
	starts uint64 // StartFile calls so far, to order the files in work

	completedBytes int64 // Sum of sizes of fully copied files

	// base and lane are set on a lane only: it keeps no counts of its own
	// and reports into base.files[lane].
	base *FileOpTracker
	lane int
}

// trackedFile is the file one lane has in work.
type trackedFile struct {
	name      string
	bytes     int64
	size      int64
	sizeKnown bool
	// Some remote providers can report a percentage before the materialized
	// reader reveals its size (native exports, WebDAV without Content-Length).
	percent      int
	percentKnown bool
	// order is when the file was started; of several files in work the
	// progress shows the one that has been in work longest.
	order uint64
}

// percentDone is the progress of the file alone. A zero-byte stream may
// still spend significant time authenticating or committing remotely, so
// without a size or a provider percentage it stays at 0: only FileDone proves
// completion.
func (f *trackedFile) percentDone() int {
	if f.size > 0 {
		return int((f.bytes * 100) / f.size)
	}
	if f.percentKnown {
		return f.percent
	}
	return 0
}

func NewFileOpTracker(total vfs.OpStats) *FileOpTracker {
	return &FileOpTracker{
		total: total,
		files: make([]trackedFile, 1),
	}
}

// Lane returns a view of the tracker for one worker of a multi-threaded copy.
// The lane has its own current file; everything it finishes or skips counts
// into the tracker's totals, and GetProgress and GetStats on a lane answer for
// the whole tracker. A lane of a lane is a new lane of the same tracker.
func (t *FileOpTracker) Lane() *FileOpTracker {
	b, _ := t.owner()
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.files) == 0 {
		b.files = make([]trackedFile, 1) // lane 0 stays the tracker's own
	}
	b.files = append(b.files, trackedFile{})
	return &FileOpTracker{base: b, lane: len(b.files) - 1}
}

// owner returns the tracker that holds the counts and the index of t's file
// in it.
func (t *FileOpTracker) owner() (*FileOpTracker, int) {
	if t.base != nil {
		return t.base, t.lane
	}
	return t, 0
}

// fileLocked returns lane i's file, making room for it. The caller holds mu
// for writing.
func (t *FileOpTracker) fileLocked(i int) *trackedFile {
	for len(t.files) <= i {
		t.files = append(t.files, trackedFile{})
	}
	return &t.files[i]
}

// StartFile marks the beginning of a new file transfer.
func (t *FileOpTracker) StartFile(name string, size int64) {
	t.StartFileKnown(name, size, true)
}

// StartFileKnown distinguishes a genuine zero-byte file from a provider
// placeholder whose size will only be known after Open.
func (t *FileOpTracker) StartFileKnown(name string, size int64, sizeKnown bool) {
	b, i := t.owner()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.starts++
	*b.fileLocked(i) = trackedFile{
		name:      name,
		size:      size,
		sizeKnown: sizeKnown || size > 0,
		order:     b.starts,
	}
}

// SetCurrentSize supplies a size learned after Open. It preserves any earlier
// provider percentage, allowing an unknown-length network materialization to
// transition to byte-based progress without jumping backwards.
func (t *FileOpTracker) SetCurrentSize(size int64) {
	if size <= 0 {
		return
	}
	b, i := t.owner()
	b.mu.Lock()
	defer b.mu.Unlock()
	f := b.fileLocked(i)
	if f.name == "" || f.size > 0 {
		return
	}
	f.size = size
	f.sizeKnown = true
	if f.percentKnown {
		f.bytes = size * int64(f.percent) / 100
	}
}

// BytesBetweenPercents converts progress within one provider phase to bytes
// for speed accounting. It does not alter logical file progress, so a second
// Downloading/Uploading phase can report throughput without moving total
// progress backwards.
func (t *FileOpTracker) BytesBetweenPercents(previous, current int) int {
	if current <= previous {
		return 0
	}
	b, i := t.owner()
	b.mu.RLock()
	var size int64
	if i < len(b.files) {
		size = b.files[i].size
	}
	b.mu.RUnlock()
	if size <= 0 {
		return 0
	}
	delta := size*int64(current)/100 - size*int64(previous)/100
	if delta <= 0 {
		return 0
	}
	if delta > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	return int(delta)
}

// UpdateBytes records progress within the current file.
func (t *FileOpTracker) UpdateBytes(n int) {
	b, i := t.owner()
	b.mu.Lock()
	defer b.mu.Unlock()
	f := b.fileLocked(i)
	f.bytes += int64(n)
	// Safety: don't let current file progress exceed 100%
	if f.size > 0 && f.bytes > f.size {
		f.bytes = f.size
	}
}

// SetCurrentPercent replaces the staged byte count with provider-reported
// transfer progress. It returns the positive byte delta for speed accounting.
func (t *FileOpTracker) SetCurrentPercent(percent int) int {
	b, i := t.owner()
	b.mu.Lock()
	defer b.mu.Unlock()
	if percent < 0 {
		return 0
	}
	if percent > 100 {
		percent = 100
	}
	f := b.fileLocked(i)
	// Retries (for example a stale Digest nonce) may start another 0..99
	// sequence. UI and aggregate byte accounting must never move backwards.
	if f.percentKnown && percent < f.percent {
		percent = f.percent
	}
	f.percent = percent
	f.percentKnown = true
	if f.size <= 0 {
		return 0
	}
	previous := f.bytes
	target := f.size * int64(percent) / 100
	if target < previous {
		target = previous
	}
	f.bytes = target
	delta := f.bytes - previous
	if delta <= 0 {
		return 0
	}
	if delta > int64(^uint(0)>>1) {
		return int(^uint(0) >> 1)
	}
	return int(delta)
}

// FileDone completes the current file and updates global counters.
func (t *FileOpTracker) FileDone() {
	t.finishFile()
}

// FileSkipped records that a file was bypassed (e.g. user chose Skip)
func (t *FileOpTracker) FileSkipped() {
	t.finishFile()
}

func (t *FileOpTracker) finishFile() {
	b, i := t.owner()
	b.mu.Lock()
	defer b.mu.Unlock()
	f := b.fileLocked(i)
	b.processed.Files++
	b.completedBytes += f.size
	*f = trackedFile{}
}

// forgetFile drops the lane's file in work without counting it. A worker
// that gives its lane back after a failed file calls it, so the progress does
// not keep showing a file nobody is copying any more.
func (t *FileOpTracker) forgetFile() {
	b, i := t.owner()
	b.mu.Lock()
	defer b.mu.Unlock()
	if i < len(b.files) {
		b.files[i] = trackedFile{}
	}
}

// DirDone records a successfully created/processed directory.
func (t *FileOpTracker) DirDone() {
	b, _ := t.owner()
	b.mu.Lock()
	defer b.mu.Unlock()
	b.processed.Dirs++
}

// GetProgress returns data for both progress bars.
//
// With several files in work the current file is the one started first, and
// the total counts what every one of them has copied so far.
func (t *FileOpTracker) GetProgress() (filePct, totalPct int, currentName string) {
	b, _ := t.owner()
	b.mu.RLock()
	defer b.mu.RUnlock()

	var shown *trackedFile
	var inWorkBytes int64
	inWorkPct := int64(0)
	for k := range b.files {
		f := &b.files[k]
		inWorkBytes += f.bytes
		if f.name == "" {
			continue
		}
		inWorkPct += int64(f.percentDone())
		if shown == nil || f.order < shown.order {
			shown = f
		}
	}
	if shown == nil && len(b.files) > 0 {
		// Nothing named is in work: the tracker's own file still carries
		// whatever progress was reported without a StartFile.
		shown = &b.files[0]
	}

	// 1. Current File Percentage
	if shown != nil {
		currentName = shown.name
		filePct = shown.percentDone()
	}

	// 2. Total Percentage
	if b.total.Bytes > 0 && b.total.UnknownSizeFiles == 0 {
		currentTotalBytes := b.completedBytes + inWorkBytes
		totalPct = int((currentTotalBytes * 100) / b.total.Bytes)
	} else {
		// FALLBACK: If total volume is 0 (e.g. copying empty folders or 0-byte files),
		// calculate progress based on item count to avoid a "stuck" bar.
		totalItems := b.total.Files + b.total.Dirs
		processedItems := b.processed.Files + b.processed.Dirs
		if totalItems > 0 {
			progressUnits := processedItems*100 + inWorkPct
			totalPct = int(progressUnits / totalItems)
		} else {
			totalPct = 100
		}
	}

	// Safety clamp
	if totalPct > 100 {
		totalPct = 100
	}
	// Reading or writing the last byte is not the commit point: Close can still
	// fail while flushing a local file or waiting for a remote server response.
	// Reserve total 100% until FileDone records a confirmed close/commit.
	totalItems := b.total.Files + b.total.Dirs
	processedItems := b.processed.Files + b.processed.Dirs
	if totalPct >= 100 && totalItems > 0 && processedItems < totalItems {
		totalPct = 99
	}
	return
}

// GetStats returns the raw accumulated statistics.
func (t *FileOpTracker) GetStats() (processed vfs.OpStats, total vfs.OpStats) {
	b, _ := t.owner()
	b.mu.RLock()
	defer b.mu.RUnlock()
	processed = b.processed
	processed.Bytes = b.completedBytes
	for k := range b.files {
		processed.Bytes += b.files[k].bytes
	}
	return processed, b.total
}

// speedMeter turns the bytes an operation moves into the rate its progress
// shows: the bytes since the last sample over the time since it, sampled at
// most once a second. Every worker of a multi-threaded copy adds to it, and
// the progress reads it from whichever goroutine reports, so it locks.
type speedMeter struct {
	mu    sync.Mutex
	since time.Time
	bytes int64
	rate  float64
}

func newSpeedMeter(start time.Time) *speedMeter {
	return &speedMeter{since: start}
}

func (m *speedMeter) add(n int) {
	m.mu.Lock()
	m.bytes += int64(n)
	m.mu.Unlock()
}

// sample takes a new rate once a second has passed since the last one.
func (m *speedMeter) sample(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	dur := now.Sub(m.since).Seconds()
	if dur < 1.0 {
		return
	}
	m.rate = float64(m.bytes) / dur
	m.since = now
	m.bytes = 0
}

// current is the rate taken by the last sample, in bytes per second.
func (m *speedMeter) current() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rate
}
