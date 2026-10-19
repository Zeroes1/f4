package editor

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

// A selection that is one occurrence is prompted as it is, so a regex like
// "a+" cannot widen it. The check read the whole buffer to look at the
// selection, which on a chunk buffer holding part of the file failed, and the
// prompt went to the wider match the check is there to prevent.
func TestReplace_SelectedMatchIsPromptedOnAChunkBuffer(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.SetDefaultPalette()
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)

	content := "xaaay\n" + strings.Repeat("filler line\n", 60000) // ~700 KB
	Pt, buf := lazyEditorBuffer(t, content)
	buf.Prewarm()
	if _, err := Pt.Bytes(); err != piecetable.ErrLoading {
		t.Fatalf("precondition: want a buffer that is not all loaded, got %v", err)
	}
	ev := NewEditorView(Pt, nil, "big.txt")
	t.Cleanup(func() { ev.Close() })
	ev.AsyncBuf = buf
	ev.SetPosition(0, 0, 80, 24)

	ev.selectFoundPattern(2, 1) // the middle "a"
	ev.Replace("a+", "b", true, false, true, false, false)
	pumpReplacePrompt(t, nil)

	if from, to := ev.GetSelectionRange(); from != 2 || to != 3 {
		t.Errorf("the prompt is at %d..%d, want the selected 2..3", from, to)
	}
}
