package editor

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

// writeLogLines writes lines lines of about the length of a server log's,
// the kind of file that is opened large.
func writeLogLines(t *testing.T, lines int) string {
	t.Helper()
	line := []byte("2025-01-21 10:00:00.000 INFO  [inventory-worker-7] com.example.inventory.Sync - processed batch of 512 records in 37 ms\n")
	path := filepath.Join(t.TempDir(), "big.log")
	if err := os.WriteFile(path, bytes.Repeat(line, lines), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

// restoreToLastLine opens path mapped, asks for the position remembered at
// its last line, as reopening a file closed at its end does, and runs the UI
// tasks the scan posts until the editor is there. It answers how many tasks
// that took: in the application each one is followed by a frame.
func restoreToLastLine(t *testing.T, path string, lines int, wrap bool) int {
	t.Helper()
	ev := openMappedEditor(t, filepath.Dir(path), path)
	defer ev.Close()
	ev.ApplyRememberedWordWrap(wrap)
	ev.SetPosition(0, 0, 79, 24)
	ev.TargetLine = lines - 1
	ev.StartIndexing()

	tasks := 0
	deadline := time.After(60 * time.Second)
	for ev.TargetLine != -1 {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
			tasks++
		case <-deadline:
			t.Fatalf("the editor never reached line %d (index has %d lines)", lines-1, ev.Li.LineCount())
		}
	}
	if ev.CursorLine != lines-1 {
		t.Fatalf("the cursor is on line %d, want %d", ev.CursorLine, lines-1)
	}
	return tasks
}

// Reopening a large file at a line far down shows Loading until the scan has
// counted the lines before it. The scan handed its offsets to the UI thread
// after every 4 MB it read, and every hand-over is a frame drawn: on a 1 GB
// log that was some 250 frames of the Loading screen before the editor could
// show the line. Handing over by time instead keeps the count of frames to
// what the eye can follow, however large the file.
func TestMappedEditor_RestoringFarDownHandsOverInFewBatches(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	const lines = 400000 // 46 MB
	path := writeLogLines(t, lines)
	start := time.Now()
	tasks := restoreToLastLine(t, path, lines, false)
	// One hand-over per interval the scan took, the first one, the last
	// one, and the few tasks besides them that the scan posts.
	if allowed := int(time.Since(start)/indexBatchInterval) + 8; tasks > allowed {
		t.Errorf("restoring took %d UI tasks, want at most %d", tasks, allowed)
	}
}

// scanLineStarts does in one pass what the scan did in two, and has to agree
// with both, wherever the chunks happen to be cut.
func TestScanLineStarts_AgreesWithTheTwoScansItReplaces(t *testing.T) {
	long := bytes.Repeat([]byte("x"), maxWordWrapLineBytes)
	texts := [][]byte{
		nil,
		[]byte("\n"),
		[]byte("one\ntwo\n\nthree"),
		append(append([]byte("short\n"), long...), "\nafter\n"...),
		append(append([]byte("short\n"), long...), "y\nafter\n"...),
		[]byte("text\x00with a NUL\n"),
	}
	for ti, text := range texts {
		for _, cut := range []int{1, 2, 3, 7, 1000, len(text) + 1} {
			var got, want []int
			gotLen, wantLen := 0, 0
			gotUnsafe, wantUnsafe := false, false
			for base := 0; base < len(text); base += cut {
				data := text[base:min(base+cut, len(text))]
				var unsafe bool
				got, gotLen, unsafe = scanLineStarts(got, data, base, gotLen)
				gotUnsafe = gotUnsafe || unsafe
				want = piecetable.AppendNewlineOffsets(want, data, base)
				if !wantUnsafe {
					wantLen, unsafe = scanEditorWrapSafety(data, wantLen)
					wantUnsafe = unsafe
				}
			}
			if !slices.Equal(got, want) {
				t.Errorf("text %d cut every %d: line starts %v, want %v", ti, cut, got, want)
			}
			if gotUnsafe != wantUnsafe {
				t.Errorf("text %d cut every %d: unsafe %v, want %v", ti, cut, gotUnsafe, wantUnsafe)
			}
		}
	}
}

// The read-ahead hands back what a plain read of the same stretch would, in
// order, and gives way to the piece table where the file cannot be read.
func TestIndexReadAhead_ReadsWhatAPlainReadWould(t *testing.T) {
	text := bytes.Repeat([]byte("0123456789abcdef"), indexReadChunk/16*3+5)
	// The middle chunk has been typed over: reads touching it are refused.
	editedFrom, editedTo := indexReadChunk+10, indexReadChunk+20
	read := func(_ context.Context, dst []byte, offset, length int) (int, bool) {
		if offset < editedTo && offset+length > editedFrom {
			return 0, false
		}
		return copy(dst[:length], text[offset:offset+length]), true
	}
	ahead := newIndexReadAhead(context.Background(), read)
	defer ahead.close()

	var got []byte
	for pos := 0; pos < len(text); {
		take := min(indexReadChunk, len(text)-pos)
		data, ok := ahead.readAt(pos, take, len(text))
		if !ok {
			// What the scan does: read a little through the piece table.
			data = text[pos:min(pos+1000, len(text))]
		}
		got = append(got, data...)
		pos += len(data)
	}
	if !bytes.Equal(got, text) {
		t.Fatal("the read-ahead handed back other bytes than the file holds")
	}
}
