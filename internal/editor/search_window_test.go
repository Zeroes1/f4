package editor

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/textsearch"
	"github.com/unxed/vtui"
)

// The windowed search has to give the answer the whole-buffer search gives,
// whichever window seam a match falls on. Tiny windows put one under almost
// every match; the alphabet has the Kelvin sign in it, which folds to a "k"
// two bytes shorter, so folded matches differ in length from the pattern.
func TestFindLiteralWindowed_AgreesWithAWholeBufferSearch(t *testing.T) {
	previous := findAllWindow
	findAllWindow = 7
	t.Cleanup(func() { findAllWindow = previous })

	// #nosec G404 -- a fixed seed, for test input that is the same every run.
	rng := rand.New(rand.NewSource(912))
	alphabet := []string{"a", "b", "k", "K", "\u212a", "\n", "ab"}
	patterns := []string{"a", "ab", "aba", "k", "kk", "Ka", "\u212aa", "b\na", "abab"}
	decline := func(context.Context, []byte, int, int) (int, bool) { return 0, false }
	ev := &EditorView{}

	for round := 0; round < 200; round++ {
		var b strings.Builder
		for b.Len() < 40+rng.Intn(200) {
			b.WriteString(alphabet[rng.Intn(len(alphabet))])
		}
		text := b.String()
		// Edited, so the table is several pieces and cannot be viewed whole.
		Pt := piecetable.New([]byte(text[:len(text)/2]))
		Pt.Insert(Pt.Size(), []byte(text[len(text)/2:]))
		data, err := Pt.Bytes()
		if err != nil {
			t.Fatal(err)
		}

		for _, pattern := range patterns {
			for _, caseSensitive := range []bool{true, false} {
				for _, reverse := range []bool{false, true} {
					for _, next := range []bool{false, true} {
						start := rng.Intn(len(data) + 1)
						wantOff, wantLen, _ := textsearch.FindMatch(data, pattern, caseSensitive, reverse, false, false, next, start)
						gotOff, gotLen, err := ev.findLiteralWindowed(context.Background(), decline, Pt,
							pattern, caseSensitive, reverse, next, start)
						if err != nil {
							t.Fatal(err)
						}
						if gotOff != wantOff || (wantOff >= 0 && gotLen != wantLen) {
							t.Fatalf("%q in %q (case %v, reverse %v, next %v, from %d): got %d+%d, want %d+%d",
								pattern, data, caseSensitive, reverse, next, start, gotOff, gotLen, wantOff, wantLen)
						}
					}
				}
			}
		}
	}
}

// A search of a file read through the chunk buffer used to copy the whole
// file onto the heap before comparing a byte of it; the needle on the next
// line cost the whole file. It reads windows now and keeps none of them.
func TestSearch_ChunkedFileIsNotCopiedWhole(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	testutil.DrainPendingTasks()

	content, needle, needleOff := bigSearchCorpus()
	Pt, buf := lazyEditorBuffer(t, content)
	ev := NewEditorViewIndexedLater(Pt, nil, "big.txt")
	ev.AsyncBuf = buf
	ev.File = buf.File
	defer ev.Close()
	ev.SetPosition(0, 0, 80, 24)

	ev.Search(strings.ToLower(needle), false, false, false, false, false)
	deadline := time.After(10 * time.Second)
	for !ev.SelActive {
		select {
		case task := <-vtui.FrameManager.TaskChan:
			task()
		case <-deadline:
			t.Fatal("the search did not finish")
		}
	}
	if ev.SelAnchorOffset != needleOff {
		t.Errorf("found at %d, want %d", ev.SelAnchorOffset, needleOff)
	}
	ev.searchSnapMu.Lock()
	snapshot := ev.searchSnapshot
	ev.searchSnapMu.Unlock()
	if snapshot != nil {
		t.Errorf("the search kept a %d byte copy of the file", len(snapshot))
	}
}
