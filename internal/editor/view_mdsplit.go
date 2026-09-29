package editor

// The Markdown split preview (f4#1625, step 5): the editor keeps the left
// half of the workspace and a formatted view of the text it holds fills the
// right half, rebuilt shortly after typing stops and scrolled to where the
// cursor is. It is the same vtui Markdown viewer the F3 view and the
// Shift+F3 snapshot use; nothing is parsed here.

import (
	"path/filepath"
	"strings"
	"time"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// mdSplitDelay is how long the text must sit still before the preview is
// rebuilt: typing does not reparse the document per keystroke.
const mdSplitDelay = 300 * time.Millisecond

// mdSplitMaxSize is the largest text the split preview formats, the same
// limit as the F3 view (internal/app/markdown_view.go).
const mdSplitMaxSize = 4 << 20

type mdSplitState struct {
	view  *vtui.HelpView
	timer *time.Timer
}

// MarkdownSplitActive reports whether the split preview is on.
func (ev *EditorView) MarkdownSplitActive() bool { return ev.mdSplit != nil }

// ToggleMarkdownSplit switches the preview beside the editor on or off.
func (ev *EditorView) ToggleMarkdownSplit() {
	if ev.mdSplit != nil {
		if ev.mdSplit.timer != nil {
			ev.mdSplit.timer.Stop()
		}
		ev.mdSplit = nil
	} else {
		ev.mdSplit = &mdSplitState{}
		ev.refreshMarkdownSplit()
	}
	if ev.lastW > 0 && ev.lastH > 0 {
		ev.ResizeConsole(ev.lastW, ev.lastH)
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

// layoutMarkdownSplit narrows the editor to the left half and puts the
// preview in the right one. It runs at the end of ResizeConsole.
func (ev *EditorView) layoutMarkdownSplit(w int) {
	if ev.mdSplit == nil || w < 20 {
		return
	}
	x1, y1, x2, y2 := ev.GetPosition()
	mid := w / 2
	ev.SetPosition(x1, y1, mid-1, y2)
	if ev.mdSplit.view != nil {
		ev.mdSplit.view.SetPosition(mid, y1, x2, y2)
	}
}

// refreshMarkdownSplit rebuilds the preview from the buffer as it is now and
// scrolls it to the cursor's relative place in the text.
func (ev *EditorView) refreshMarkdownSplit() {
	st := ev.mdSplit
	if st == nil || ev.Pt == nil {
		return
	}
	text := ev.GetText()
	if len(text) > mdSplitMaxSize {
		st.view = nil
		return
	}
	name := filepath.Base(ev.FilePath)
	view := vtui.NewMarkdownView(name, strings.ReplaceAll(text, "\r\n", "\n"))
	view.Modal = false
	view.ShowClose = false
	view.SetTitle(" " + name + " ")
	if ev.lastW > 0 {
		_, y1, _, y2 := ev.GetPosition()
		view.SetPosition(ev.lastW/2, y1, ev.lastW-1, y2)
	}
	st.view = view
	ev.syncMarkdownSplitScroll()
}

// syncMarkdownSplitScroll scrolls the preview to the same relative place as
// the cursor, with the wheel the viewer already understands: it has no API
// for an absolute scroll position.
func (ev *EditorView) syncMarkdownSplitScroll() {
	st := ev.mdSplit
	if st == nil || st.view == nil || st.view.CurrentTopic() == nil || ev.Li == nil {
		return
	}
	total := ev.Li.LineCount()
	rows := len(st.view.CurrentTopic().Lines)
	if total <= 0 || rows <= 0 {
		return
	}
	target := ev.CursorLine * rows / total
	step := vtui.WheelLinesPerNotch()
	if step < 1 {
		step = 1
	}
	for i := 0; i < target/step; i++ {
		st.view.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType, WheelDirection: -1})
	}
}

// scheduleMarkdownSplit restarts the pause timer after a key, so the preview
// follows the text once typing stops.
func (ev *EditorView) scheduleMarkdownSplit() {
	st := ev.mdSplit
	if st == nil {
		return
	}
	if st.timer != nil {
		st.timer.Stop()
	}
	// Read on the calling goroutine, like scheduleIndexResume does.
	uiFrames := vtui.FrameManager
	st.timer = time.AfterFunc(mdSplitDelay, func() {
		uiFrames.PostTask(func() {
			if ev.IsDone() || ev.mdSplit != st {
				return
			}
			ev.refreshMarkdownSplit()
			uiFrames.Redraw()
		})
	})
}

func (ev *EditorView) showMarkdownSplit(scr *vtui.ScreenBuf) {
	if ev.mdSplit != nil && ev.mdSplit.view != nil {
		ev.mdSplit.view.Show(scr)
	}
}

// markdownSplitMouse hands a mouse event on the preview half to the preview.
func (ev *EditorView) markdownSplitMouse(e *vtinput.InputEvent) bool {
	if ev.mdSplit == nil || ev.mdSplit.view == nil || e.Type != vtinput.MouseEventType {
		return false
	}
	x1, y1, x2, y2 := ev.mdSplit.view.GetPosition()
	mx, my := int(e.MouseX), int(e.MouseY)
	if mx < x1 || mx > x2 || my < y1 || my > y2 {
		return false
	}
	return ev.mdSplit.view.ProcessMouse(e)
}
