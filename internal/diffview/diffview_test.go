package diffview

import (
	"testing"

	"github.com/unxed/f4/internal/textdiff"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func newTestDiffView(t *testing.T, left, right []string) *DiffView {
	t.Helper()
	dv, err := NewDiffView("left.txt", "right.txt", left, right)
	if err != nil {
		t.Fatalf("NewDiffView: %v", err)
	}
	return dv
}

func TestNewDiffViewJumpsToFirstDifference(t *testing.T) {
	left := []string{"a", "b", "same", "c"}
	right := []string{"a", "x", "same", "c"}
	dv := newTestDiffView(t, left, right)

	if len(dv.rows) == 0 {
		t.Fatal("expected rows to be populated")
	}
	if dv.rows[dv.cursor].Left.Kind == textdiff.RowEqual {
		t.Fatalf("cursor row %d should not be RowEqual, got %#v", dv.cursor, dv.rows[dv.cursor])
	}
	if dv.topPos != dv.cursor {
		t.Fatalf("expected topPos to start at the first difference (%d), got %d", dv.cursor, dv.topPos)
	}
}

func TestNewDiffViewIdenticalFilesOpenAtTop(t *testing.T) {
	lines := []string{"a", "b", "c"}
	dv := newTestDiffView(t, lines, lines)
	if dv.topPos != 0 || dv.cursor != 0 {
		t.Fatalf("expected identical files to open at the top, got topPos=%d cursor=%d", dv.topPos, dv.cursor)
	}
	for _, r := range dv.rows {
		if r.Left.Kind != textdiff.RowEqual {
			t.Fatalf("expected only RowEqual rows for identical inputs, got %#v", r)
		}
	}
}

func TestDiffViewTooLarge(t *testing.T) {
	big := make([]string, textdiff.MaxLines)
	for i := range big {
		big[i] = "x"
	}
	if _, err := NewDiffView("a", "b", big, []string{"y"}); err != textdiff.ErrTooLarge {
		t.Fatalf("expected ErrTooLarge, got %v", err)
	}
}

// key sends a synthetic key-down event through ProcessKey, the same way a
// real terminal input event would arrive.
func key(dv *DiffView, vk uint16, ctrl bool) bool {
	state := vtinput.ControlKeyState(0)
	if ctrl {
		state = vtinput.LeftCtrlPressed
	}
	return dv.ProcessKey(&vtinput.InputEvent{
		Type:            vtinput.KeyEventType,
		KeyDown:         true,
		VirtualKeyCode:  vk,
		ControlKeyState: state,
	})
}

func TestDiffViewEscapeCloses(t *testing.T) {
	dv := newTestDiffView(t, []string{"a"}, []string{"b"})
	if dv.IsDone() {
		t.Fatal("expected the view not to be done before Escape")
	}
	if !key(dv, vtinput.VK_ESCAPE, false) {
		t.Fatal("expected Escape to be handled")
	}
	if !dv.IsDone() {
		t.Fatal("expected Escape to close the view")
	}
}

func TestDiffViewScrollClampsToContent(t *testing.T) {
	var left, right []string
	for i := 0; i < 5; i++ {
		left = append(left, "line")
		right = append(right, "line")
	}
	dv := newTestDiffView(t, left, right)
	dv.SetPosition(0, 0, 20, 3) // 4 rows tall -> 2 content rows (minus 2 border rows)

	// Scrolling up past the top must clamp at 0.
	key(dv, vtinput.VK_UP, false)
	key(dv, vtinput.VK_UP, false)
	if dv.topPos != 0 {
		t.Fatalf("expected topPos clamped to 0, got %d", dv.topPos)
	}

	// Scrolling down past the end must clamp at len(rows)-viewHeight.
	for i := 0; i < 20; i++ {
		key(dv, vtinput.VK_DOWN, false)
	}
	maxTop := len(dv.rows) - dv.viewHeight()
	if maxTop < 0 {
		maxTop = 0
	}
	if dv.topPos != maxTop {
		t.Fatalf("expected topPos clamped to %d, got %d", maxTop, dv.topPos)
	}
}

func TestDiffViewJumpToDifferenceWraps(t *testing.T) {
	left := []string{"a", "b1", "c", "d", "e2"}
	right := []string{"a", "b2", "c", "d", "e1"}
	dv := newTestDiffView(t, left, right)
	dv.SetPosition(0, 0, 20, 6)

	first := dv.cursor
	key(dv, vtinput.VK_DOWN, true) // Ctrl+Down: next difference
	second := dv.cursor
	if second == first {
		t.Fatalf("expected cursor to move to a different row, stayed at %d", first)
	}
	if dv.rows[second].Left.Kind == textdiff.RowEqual {
		t.Fatalf("row %d should be a difference, got %#v", second, dv.rows[second])
	}

	key(dv, vtinput.VK_UP, true) // Ctrl+Up: back to the previous difference
	if dv.cursor != first {
		t.Fatalf("expected Ctrl+Up to return to row %d, got %d", first, dv.cursor)
	}
}

// TestDiffViewShowDoesNotPanic exercises the full paint path against a real
// (silent) ScreenBuf, the way internal/viewer's own tests do -- a plain smoke
// test for slice bounds and nil derefs that the logic-only tests above
// cannot see, since they never call Show.
func TestDiffViewShowDoesNotPanic(t *testing.T) {
	theme.SetDefaultF4Palette()

	left := []string{"a", "b", "c", "d", "e", "f"}
	right := []string{"a", "x", "c", "y", "z", "f"}
	dv := newTestDiffView(t, left, right)
	dv.SetPosition(0, 0, 39, 9)

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(40, 10)

	dv.Show(scr)
	// Scroll around and repaint, including past both ends, to exercise the
	// clamping paths inside Show itself.
	dv.topPos = -5
	dv.Show(scr)
	dv.topPos = len(dv.rows) + 5
	dv.Show(scr)
}

// TestDiffViewShowZeroSizeDoesNotPanic covers the frame being painted before
// ResizeConsole/SetPosition ever ran (X1..Y2 all zero), which is exactly the
// state NewDiffView leaves a view in.
func TestDiffViewShowZeroSizeDoesNotPanic(t *testing.T) {
	theme.SetDefaultF4Palette()
	dv := newTestDiffView(t, []string{"a"}, []string{"b"})
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(1, 1)
	dv.Show(scr)
}
