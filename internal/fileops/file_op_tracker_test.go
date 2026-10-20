package fileops

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestFileOpTracker_SingleFile(t *testing.T) {
	// Total: 1 file, 1000 bytes
	total := vfs.OpStats{Files: 1, Bytes: 1000}
	tracker := NewFileOpTracker(total)

	tracker.StartFile("test.bin", 1000)
	tracker.UpdateBytes(500)

	filePct, totalPct, name := tracker.GetProgress()

	if name != "test.bin" {
		t.Errorf("Expected name test.bin, got %q", name)
	}
	if filePct != 50 {
		t.Errorf("Expected 50%% file progress, got %d", filePct)
	}
	if totalPct != 50 {
		t.Errorf("Expected 50%% total progress, got %d", totalPct)
	}

	tracker.UpdateBytes(500)
	tracker.FileDone()

	filePct, totalPct, _ = tracker.GetProgress()
	if filePct != 0 {
		t.Errorf("Expected 0%% file progress after FileDone, got %d", filePct)
	}
	if totalPct != 100 {
		t.Errorf("Expected 100%% total progress, got %d", totalPct)
	}
}

func TestFileOpTracker_MultiFile(t *testing.T) {
	// Total: 2 files, 200 bytes total (100 each)
	total := vfs.OpStats{Files: 2, Bytes: 200}
	tracker := NewFileOpTracker(total)

	// Finish first file
	tracker.StartFile("f1.txt", 100)
	tracker.UpdateBytes(100)
	tracker.FileDone()

	// Half of second file
	tracker.StartFile("f2.txt", 100)
	tracker.UpdateBytes(50)

	filePct, totalPct, _ := tracker.GetProgress()

	// File 2 is at 50%
	if filePct != 50 {
		t.Errorf("FilePct error: %d", filePct)
	}
	// Total is (100 + 50) / 200 = 75%
	if totalPct != 75 {
		t.Errorf("TotalPct error: expected 75, got %d", totalPct)
	}
}

func TestFileOpTracker_ZeroBytesFallback(t *testing.T) {
	// Scenario: 10 empty folders, 0 bytes total volume
	total := vfs.OpStats{Dirs: 10, Bytes: 0}
	tracker := NewFileOpTracker(total)

	for i := 0; i < 5; i++ {
		tracker.DirDone()
	}

	_, totalPct, _ := tracker.GetProgress()

	// Should use item count (5/10 = 50%)
	if totalPct != 50 {
		t.Errorf("Expected 50%% progress based on item count, got %d", totalPct)
	}
}

func TestFileOpTracker_ZeroByteFileWaitsForCommit(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1})
	tracker.StartFileKnown("empty-cloud-object", 0, true)
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 0 || totalPct != 0 {
		t.Fatalf("pre-commit zero-byte progress = %d/%d, want 0/0", filePct, totalPct)
	}
	tracker.SetCurrentPercent(50)
	filePct, totalPct, _ = tracker.GetProgress()
	if filePct != 50 || totalPct != 50 {
		t.Fatalf("provider zero-byte progress = %d/%d, want 50/50", filePct, totalPct)
	}
	tracker.FileDone()
	_, totalPct, _ = tracker.GetProgress()
	if totalPct != 100 {
		t.Fatalf("committed zero-byte total = %d, want 100", totalPct)
	}
}

func TestFileOpTracker_OverReporting(t *testing.T) {
	total := vfs.OpStats{Files: 1, Bytes: 100}
	tracker := NewFileOpTracker(total)

	tracker.StartFile("growth.log", 100)
	tracker.UpdateBytes(150) // More than announced

	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 100 || totalPct != 99 {
		t.Errorf("Over-reporting/commit reservation failed: file=%d, total=%d", filePct, totalPct)
	}
}

func TestFileOpTracker_EmptyJob(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{})
	_, totalPct, _ := tracker.GetProgress()
	if totalPct != 100 {
		t.Errorf("Empty job should report 100%%, got %d", totalPct)
	}
}

func TestFileOpTracker_SkippedFiles(t *testing.T) {
	// Total: 10 files, 1000 bytes
	total := vfs.OpStats{Files: 10, Bytes: 1000}
	tracker := NewFileOpTracker(total)

	// Process 2 files
	tracker.StartFile("f1", 100)
	tracker.UpdateBytes(100)
	tracker.FileDone()
	tracker.StartFile("f2", 100)
	tracker.UpdateBytes(100)
	tracker.FileDone()

	// Skip 1 file (announced size was 100)
	tracker.StartFile("f3", 100)
	tracker.FileSkipped()

	processed, _ := tracker.GetStats()
	if processed.Files != 3 {
		t.Errorf("Expected 3 files processed (including skipped), got %d", processed.Files)
	}

	_, totalPct, _ := tracker.GetProgress()
	// (100 + 100 + 100) / 1000 = 30%
	if totalPct != 30 {
		t.Errorf("Progress mismatch after skip: expected 30%%, got %d%%", totalPct)
	}
}
func TestFileOpTracker_ProcessedBytes(t *testing.T) {
	total := vfs.OpStats{Files: 1, Bytes: 1000}
	tracker := NewFileOpTracker(total)

	tracker.StartFile("f1", 1000)
	tracker.UpdateBytes(450)

	processed, _ := tracker.GetStats()
	if processed.Bytes != 450 {
		t.Errorf("GetStats should include in-progress bytes: expected 450, got %d", processed.Bytes)
	}

	tracker.UpdateBytes(50)
	tracker.FileDone()

	processed, _ = tracker.GetStats()
	if processed.Bytes != 1000 {
		t.Errorf("GetStats failed after FileDone: expected 1000, got %d", processed.Bytes)
	}
}

func TestFileOpTracker_UsesProviderCommitProgress(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 1000})
	tracker.StartFile("remote.bin", 1000)

	if delta := tracker.SetCurrentPercent(37); delta != 370 {
		t.Fatalf("37%% delta = %d, want 370", delta)
	}
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 37 || totalPct != 37 {
		t.Fatalf("provider progress = file %d%% total %d%%", filePct, totalPct)
	}

	if delta := tracker.SetCurrentPercent(100); delta != 630 {
		t.Fatalf("100%% delta = %d, want 630", delta)
	}
	tracker.FileDone()
	_, totalPct, _ = tracker.GetProgress()
	if totalPct != 100 {
		t.Fatalf("committed total = %d%%", totalPct)
	}
}

func TestFileOpTracker_ProviderRetryNeverMovesProgressBackwards(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 1000})
	tracker.StartFile("digest-upload.bin", 1000)
	if delta := tracker.SetCurrentPercent(72); delta != 720 {
		t.Fatalf("first delta = %d, want 720", delta)
	}
	if delta := tracker.SetCurrentPercent(3); delta != 0 {
		t.Fatalf("retry delta = %d, want 0", delta)
	}
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 72 || totalPct != 72 {
		t.Fatalf("progress moved backwards to file=%d total=%d", filePct, totalPct)
	}
}

func TestFileOpTracker_UnknownSizeUsesProviderPercentThenLearnedSize(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, UnknownSizeFiles: 1})
	tracker.StartFileKnown("native-export.bin", 0, false)
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 0 || totalPct != 0 {
		t.Fatalf("unknown-size initial progress = file %d total %d, want 0/0", filePct, totalPct)
	}
	if delta := tracker.SetCurrentPercent(41); delta != 0 {
		t.Fatalf("unknown-size byte delta = %d, want 0", delta)
	}
	filePct, totalPct, _ = tracker.GetProgress()
	if filePct != 41 {
		t.Fatalf("unknown-size file progress = %d, want 41", filePct)
	}
	if totalPct != 41 {
		t.Fatalf("unknown-size total progress = %d, want 41", totalPct)
	}
	tracker.SetCurrentSize(2000)
	processed, _ := tracker.GetStats()
	if processed.Bytes != 820 {
		t.Fatalf("learned-size processed bytes = %d, want 820", processed.Bytes)
	}
	if delta := tracker.SetCurrentPercent(50); delta != 180 {
		t.Fatalf("learned-size delta = %d, want 180", delta)
	}
}

func TestFileOpTracker_MixedKnownAndUnknownUsesItemProgress(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 2, Bytes: 1000, UnknownSizeFiles: 1})
	tracker.StartFileKnown("known.bin", 1000, true)
	tracker.SetCurrentPercent(100)
	tracker.FileDone()
	tracker.StartFileKnown("export.bin", 0, false)
	tracker.SetCurrentPercent(40)
	filePct, totalPct, _ := tracker.GetProgress()
	if filePct != 40 || totalPct != 70 {
		t.Fatalf("mixed progress = file %d total %d, want 40/70", filePct, totalPct)
	}
	processed, total := tracker.GetStats()
	if total.Bytes != 1000 || total.UnknownSizeFiles != 1 || processed.Bytes != 1000 {
		t.Fatalf("mixed stats processed=%#v total=%#v", processed, total)
	}
}

func TestGlobalAwareReporterResetsProviderProgressPerFile(t *testing.T) {
	reporter := &GlobalAwareReporter{tracker: NewFileOpTracker(vfs.OpStats{Files: 2, Bytes: 2})}
	reporter.providerProgress.Store(true)
	reporter.StartFile("next.bin", 1)
	if reporter.providerProgress.Load() {
		t.Fatal("provider progress from the previous file leaked into the next file")
	}
}

func TestGlobalAwareReporterClampsRetriesButResetsDistinctTransferPhase(t *testing.T) {
	capture := &mockReporter{}
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 1000})
	tracker.StartFileKnown("cross-cloud.bin", 1000, true)
	accounted := 0
	reporter := &GlobalAwareReporter{
		original:  capture,
		tracker:   tracker,
		getGlobal: func(string) (string, int, string) { return "", 0, "" },
		onBytes:   func(n int) { accounted += n },
	}
	reporter.UpdateTransfer("Downloading", "cross-cloud.bin", 72, "", 72, "")
	reporter.UpdateTransfer("Downloading", "cross-cloud.bin", 3, "", 3, "")
	if capture.lastCurrentPct != 72 {
		t.Fatalf("same-phase retry moved visible bar to %d", capture.lastCurrentPct)
	}
	reporter.UpdateTransfer("Uploading", "cross-cloud.bin", 0, "", 0, "")
	if capture.lastCurrentPct != 0 {
		t.Fatalf("new upload phase did not reset current bar: %d", capture.lastCurrentPct)
	}
	reporter.UpdateTransfer("Uploading", "cross-cloud.bin", 50, "", 50, "")
	if capture.lastCurrentPct != 50 {
		t.Fatalf("upload phase progress = %d, want 50", capture.lastCurrentPct)
	}
	if accounted != 1220 { // 72% download + 50% upload.
		t.Fatalf("phase throughput bytes = %d, want 1220", accounted)
	}
	_, totalPct, _ := tracker.GetProgress()
	if totalPct != 72 {
		t.Fatalf("logical total regressed during second phase: %d", totalPct)
	}
}

// A lane has its own file in work (f4#1894, part 5): two workers' files are
// counted side by side, finishing one leaves the other's progress alone, and
// the dialog shows the file that has been in work longest.
func TestFileOpTracker_LanesKeepTheirOwnFiles(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 3, Bytes: 600})
	a := tracker.Lane()
	b := tracker.Lane()

	a.StartFile("a.bin", 100)
	b.StartFile("b.bin", 300)
	a.UpdateBytes(50)
	b.UpdateBytes(150)

	filePct, totalPct, name := tracker.GetProgress()
	if name != "a.bin" || filePct != 50 || totalPct != 33 {
		t.Fatalf("two files in work: name=%q file=%d%% total=%d%%, want a.bin 50%% 33%%", name, filePct, totalPct)
	}
	if processed, _ := b.GetStats(); processed.Bytes != 200 || processed.Files != 0 {
		t.Fatalf("a lane's GetStats = %+v, want the tracker's 200 bytes and no file done", processed)
	}

	b.UpdateBytes(150)
	b.FileDone()
	filePct, totalPct, name = a.GetProgress()
	if name != "a.bin" || filePct != 50 || totalPct != 58 {
		t.Fatalf("after b.bin is done: name=%q file=%d%% total=%d%%, want a.bin 50%% 58%%", name, filePct, totalPct)
	}

	a.UpdateBytes(50)
	a.FileDone()
	tracker.StartFile("c.bin", 200)
	tracker.UpdateBytes(200)
	tracker.FileDone()
	processed, _ := tracker.GetStats()
	if processed.Files != 3 || processed.Bytes != 600 {
		t.Fatalf("all done: %+v, want 3 files and 600 bytes", processed)
	}
	if _, totalPct, name = tracker.GetProgress(); totalPct != 100 || name != "" {
		t.Fatalf("all done: total=%d%% name=%q, want 100%% and no file", totalPct, name)
	}
}

// Of several files in work the oldest is shown, and a lane of a lane is one
// more lane of the same tracker.
func TestFileOpTracker_LaneShowsTheOldestFileInWork(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 2, Dirs: 0})
	first := tracker.Lane()
	second := first.Lane()
	second.StartFile("late", 10)
	first.StartFile("later", 10)
	if _, _, name := tracker.GetProgress(); name != "late" {
		t.Fatalf("shown %q, want the file started first", name)
	}
	second.FileSkipped()
	if _, _, name := tracker.GetProgress(); name != "later" {
		t.Fatalf("shown %q after the first was skipped, want the other file in work", name)
	}
	if processed, _ := tracker.GetStats(); processed.Files != 1 {
		t.Fatalf("a skip on a lane of a lane counted %d files on the tracker, want 1", processed.Files)
	}
}

// forgetFile drops a lane's file without counting it: a worker giving its
// lane back after a failure must not leave a file showing that nobody copies.
func TestFileOpTracker_ForgetFile(t *testing.T) {
	tracker := NewFileOpTracker(vfs.OpStats{Files: 1, Bytes: 100})
	lane := tracker.Lane()
	lane.StartFile("broken", 100)
	lane.UpdateBytes(40)
	lane.forgetFile()
	processed, _ := tracker.GetStats()
	_, totalPct, name := tracker.GetProgress()
	if name != "" || processed.Bytes != 0 || processed.Files != 0 || totalPct != 0 {
		t.Fatalf("forgotten file still counted: name=%q stats=%+v total=%d%%", name, processed, totalPct)
	}
}

// Workers copying files at once through their lanes add up to exactly the
// work done, and what the dialog reads meanwhile never moves backwards. Run
// under -race this is also the check that the lanes share nothing unlocked.
func TestFileOpTracker_LanesUnderConcurrentWorkers(t *testing.T) {
	const workers, files, size, chunk = 8, 50, 1000, 100
	tracker := NewFileOpTracker(vfs.OpStats{Files: workers * files, Bytes: workers * files * size})

	stop := make(chan struct{})
	readerDone := make(chan error, 1)
	go func() {
		var last int64
		lastPct := 0
		for {
			select {
			case <-stop:
				readerDone <- nil
				return
			default:
			}
			_, totalPct, _ := tracker.GetProgress()
			processed, _ := tracker.GetStats()
			if processed.Bytes < last || totalPct < lastPct {
				readerDone <- fmt.Errorf("progress went back: bytes %d -> %d, total %d%% -> %d%%", last, processed.Bytes, lastPct, totalPct)
				return
			}
			last, lastPct = processed.Bytes, totalPct
		}
	}()

	var wg sync.WaitGroup
	for w := range workers {
		lane := tracker.Lane()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for f := range files {
				lane.StartFile(fmt.Sprintf("w%d-f%d", w, f), size)
				for range size / chunk {
					lane.UpdateBytes(chunk)
				}
				lane.FileDone()
			}
		}()
	}
	wg.Wait()
	close(stop)
	if err := <-readerDone; err != nil {
		t.Fatal(err)
	}
	processed, _ := tracker.GetStats()
	if processed.Files != workers*files || processed.Bytes != workers*files*size {
		t.Fatalf("processed %+v, want %d files and %d bytes", processed, workers*files, workers*files*size)
	}
	if _, totalPct, _ := tracker.GetProgress(); totalPct != 100 {
		t.Fatalf("total %d%% after every file is done, want 100%%", totalPct)
	}
}

// The speed is the bytes of every worker over the time since the last sample,
// as it was for one.
func TestSpeedMeterAddsEveryWorker(t *testing.T) {
	start := time.Unix(1000, 0)
	meter := newSpeedMeter(start)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 100 {
				meter.add(10)
			}
		}()
	}
	wg.Wait()
	meter.sample(start.Add(500 * time.Millisecond))
	if got := meter.current(); got != 0 {
		t.Fatalf("rate %v before a second passed, want no sample yet", got)
	}
	meter.sample(start.Add(2 * time.Second))
	if got := meter.current(); got != 4000 {
		t.Fatalf("rate %v, want 8000 bytes over 2 s = 4000", got)
	}
	meter.sample(start.Add(4 * time.Second))
	if got := meter.current(); got != 0 {
		t.Fatalf("rate %v after two idle seconds, want 0", got)
	}
}
