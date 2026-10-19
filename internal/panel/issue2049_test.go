package panel

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/terminal/far2ldnd"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// issue2049Fixture is a frame whose left panel shows a folder holding the
// folder "sub" and the file "f.txt", and a file elsewhere to drop on it.
// Drops are recorded instead of copied.
type issue2049Fixture struct {
	pf        *PanelsFrame
	fsp       *FileSystemPanel
	dir       string
	source    string
	drops     chan string
	subX      int
	subY      int
	fileX     int
	fileY     int
	subIndex  int
	fileIndex int
}

func newIssue2049Fixture(t *testing.T) *issue2049Fixture {
	t.Helper()
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()

	f := &issue2049Fixture{dir: t.TempDir(), drops: make(chan string, 4)}
	if err := os.Mkdir(filepath.Join(f.dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, "f.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.source = filepath.Join(t.TempDir(), "dropped.txt")
	if err := os.WriteFile(f.source, []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}

	old := executeDropFileOp
	executeDropFileOp = func(_, _ vfs.VFS, _ []string, dstDir string, _ bool, _ int, _ func()) {
		f.drops <- dstDir
	}
	t.Cleanup(func() { executeDropFileOp = old })
	oldPolicy := config.App.DropOnFolder
	t.Cleanup(func() { config.App.DropOnFolder = oldPolicy })

	f.pf = NewPanelsFrame()
	t.Cleanup(f.pf.Close)
	f.pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(f.pf)
	f.pf.ShowPanels = true
	f.fsp = f.pf.Panels[0].(*FileSystemPanel)
	f.fsp.Vfs = vfs.NewOSVFS(f.dir)
	f.fsp.ReadDirectory()
	waitForDirectoryLoads(t)
	testutil.DrainUITasks()
	f.pf.Show(scr)

	f.subIndex, f.fileIndex = -1, -1
	f.fsp.visibleNameCells(func(e *FileEntry, x, y, _ int) {
		switch e.Name {
		case "sub":
			f.subX, f.subY = x, y
		case "f.txt":
			f.fileX, f.fileY = x, y
		}
	})
	for i, e := range f.fsp.Entries {
		switch e.Name {
		case "sub":
			f.subIndex = i
		case "f.txt":
			f.fileIndex = i
		}
	}
	if f.subIndex < 0 || f.fileIndex < 0 || f.subY == 0 || f.fileY == 0 {
		t.Fatalf("fixture: sub at %d (%d,%d), f.txt at %d (%d,%d)", f.subIndex, f.subX, f.subY, f.fileIndex, f.fileX, f.fileY)
	}
	return f
}

func (f *issue2049Fixture) drag(phase vtui.DragPhase, x, y int, tracked bool) vtui.DropAction {
	return f.pf.HandleDrag(&vtui.DragEvent{
		Phase: phase, X: x, Y: y, Tracked: tracked,
		Allowed: vtui.DropCopy, Suggested: vtui.DropCopy,
		Payload: vtui.DragPayload{Paths: []string{f.source}},
	})
}

func (f *issue2049Fixture) droppedInto(t *testing.T) string {
	t.Helper()
	select {
	case dir := <-f.drops:
		return dir
	case <-time.After(5 * time.Second):
		t.Fatal("the drop never started a transfer")
		return ""
	}
}

// Where the backend follows the drag, the folder row under the pointer is
// highlighted, and anything else in the panel highlights the panel itself;
// the drop goes where the highlight was, and the highlight goes with it.
func TestIssue2049_TrackedDragHighlightsTarget(t *testing.T) {
	f := newIssue2049Fixture(t)

	f.drag(vtui.DragEnter, f.subX, f.subY, true)
	if f.fsp.drop != (dropHighlight{active: true, entry: f.subIndex}) {
		t.Fatalf("over the folder row: highlight %+v, want the row", f.fsp.drop)
	}
	f.drag(vtui.DragOver, f.fileX, f.fileY, true)
	if f.fsp.drop != (dropHighlight{active: true, entry: -1}) {
		t.Fatalf("over a file: highlight %+v, want the panel", f.fsp.drop)
	}
	f.drag(vtui.DragLeave, 0, 0, true)
	if f.fsp.drop.active {
		t.Fatal("leaving the window must clear the highlight")
	}

	f.drag(vtui.DragEnter, f.subX, f.subY, true)
	f.drag(vtui.DragDrop, f.subX, f.subY, true)
	if got, want := f.droppedInto(t), filepath.Join(f.dir, "sub"); got != want {
		t.Fatalf("tracked drop over the highlighted folder went to %q, want %q", got, want)
	}
	if f.fsp.drop.active {
		t.Fatal("the drop must clear the highlight")
	}
}

// Where the backend only reports the drop, nothing was highlighted, so
// nothing is: and a drop over a folder row goes where DropOnFolder says -
// the open folder by default.
func TestIssue2049_UntrackedDropFollowsDropOnFolder(t *testing.T) {
	f := newIssue2049Fixture(t)
	sub := filepath.Join(f.dir, "sub")

	cases := []struct {
		policy string
		want   string
	}{
		{"", f.dir}, // the default
		{config.DropOnFolderCurrent, f.dir},
		{config.DropOnFolderFolder, sub},
	}
	for _, c := range cases {
		config.App.DropOnFolder = c.policy
		f.drag(vtui.DragEnter, f.subX, f.subY, false)
		if f.fsp.drop.active {
			t.Fatalf("policy %q: an untracked gesture must not highlight anything", c.policy)
		}
		f.drag(vtui.DragDrop, f.subX, f.subY, false)
		if got := f.droppedInto(t); got != c.want {
			t.Errorf("policy %q: drop went to %q, want %q", c.policy, got, c.want)
		}
	}

	// Over a file the folder is the panel's whatever the policy.
	config.App.DropOnFolder = config.DropOnFolderFolder
	f.drag(vtui.DragDrop, f.fileX, f.fileY, false)
	if got := f.droppedInto(t); got != f.dir {
		t.Errorf("drop over a file went to %q, want the open folder", got)
	}
}

// With DropOnFolder = ask, a drop over a folder row asks which folder, and
// goes where the answer says - or nowhere.
func TestIssue2049_AskWhichFolder(t *testing.T) {
	f := newIssue2049Fixture(t)
	config.App.DropOnFolder = config.DropOnFolderAsk

	answer := func(code int) {
		t.Helper()
		w, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
		if !ok || w.OnResult == nil {
			t.Fatalf("no question on screen, top frame is %T", vtui.FrameManager.GetTopFrame())
		}
		w.OnResult(code)
		vtui.FrameManager.RemoveFrame(w)
	}

	f.drag(vtui.DragDrop, f.subX, f.subY, false)
	answer(0)
	if got, want := f.droppedInto(t), filepath.Join(f.dir, "sub"); got != want {
		t.Fatalf("\"into the folder\" went to %q, want %q", got, want)
	}

	f.drag(vtui.DragDrop, f.subX, f.subY, false)
	answer(1)
	if got := f.droppedInto(t); got != f.dir {
		t.Fatalf("\"into the panel\" went to %q, want %q", got, f.dir)
	}

	f.drag(vtui.DragDrop, f.subX, f.subY, false)
	answer(2)
	select {
	case dir := <-f.drops:
		t.Fatalf("a cancelled drop still went to %q", dir)
	case <-time.After(100 * time.Millisecond):
	}
}

// The highlight is drawn: the folder row in the cursor's colours, the panel
// frame when the target is the open folder.
func TestIssue2049_HighlightIsDrawn(t *testing.T) {
	f := newIssue2049Fixture(t)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	cursor := vtui.Palette[theme.ColPanelCursor]

	f.fsp.drop = dropHighlight{active: true, entry: f.subIndex}
	f.fsp.SetFocus(false)
	f.fsp.Show(scr)
	if got := scr.GetCell(f.subX, f.subY).Attributes; got != cursor {
		t.Fatalf("highlighted folder row attr %#x, want the cursor's %#x", got, cursor)
	}

	f.fsp.drop = dropHighlight{active: true, entry: -1}
	f.fsp.Show(scr)
	x1, y1, _, _ := f.fsp.GetPosition()
	if got := scr.GetCell(x1, y1).Attributes; got != cursor {
		t.Fatalf("highlighted panel frame attr %#x, want the cursor's %#x", got, cursor)
	}
	if got := scr.GetCell(f.subX, f.subY).Attributes; got == cursor && !f.fsp.IsFocused() {
		t.Fatal("with the panel as the target the folder row must not stay highlighted")
	}
}

// With several workspaces the drag goes to the panels on screen, not to the
// workspace created last: that one used to answer every drag, so the
// highlight was set on panels nobody saw and drops were laid out against
// them (#2049).
func TestIssue2049_DragGoesToWorkspaceOnScreen(t *testing.T) {
	t.Cleanup(swapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	theme.SetDefaultF4Palette()
	t.Cleanup(func() { vtui.SetDropTarget(nil) })

	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	open := func() (*PanelsFrame, int) {
		pf := NewPanelsFrame()
		t.Cleanup(pf.Close)
		vtui.FrameManager.AddScreen(pf)
		pf.ResizeConsole(80, 25)
		pf.ShowPanels = true
		fsp := pf.Panels[0].(*FileSystemPanel)
		fsp.Vfs = vfs.NewOSVFS(dir)
		fsp.ReadDirectory()
		waitForDirectoryLoads(t)
		testutil.DrainUITasks()
		pf.Show(scr)
		return pf, vtui.FrameManager.ActiveIdx
	}
	first, firstIdx := open()
	second, _ := open() // created last: it used to be the only target

	var x, y int
	first.Panels[0].(*FileSystemPanel).visibleNameCells(func(e *FileEntry, ex, ey, _ int) {
		if e.Name == "sub" {
			x, y = ex, ey
		}
	})
	target := vtui.CurrentDropTarget()
	enter := func() {
		target.HandleDrag(&vtui.DragEvent{Phase: vtui.DragEnter, X: x, Y: y, Tracked: true,
			Allowed: vtui.DropCopy, Payload: vtui.DragPayload{Paths: []string{filepath.Join(dir, "x")}}})
	}
	highlighted := func(pf *PanelsFrame) bool { return pf.Panels[0].(*FileSystemPanel).drop.active }

	vtui.FrameManager.SwitchScreen(firstIdx)
	enter()
	if !highlighted(first) || highlighted(second) {
		t.Fatalf("drag over the first workspace on screen: first=%v second=%v", highlighted(first), highlighted(second))
	}

	// The user switches workspaces mid-drag: the one left behind lets go.
	switched := false
	for i, s := range vtui.FrameManager.Screens {
		for _, f := range s.Frames {
			if f == second && !switched {
				vtui.FrameManager.SwitchScreen(i)
				switched = true
			}
		}
	}
	if !switched || FindPanelsFrameAnyScreen() != second {
		t.Fatal("fixture: could not switch to the second workspace")
	}
	enter()
	if highlighted(first) || !highlighted(second) {
		t.Fatalf("after switching: first=%v second=%v", highlighted(first), highlighted(second))
	}
}

// issue2049Offer records where a terminal's offer went and how it was
// closed, without copying anything.
type issue2049Offer struct {
	copies chan string
	closed chan uint8
}

func recordTerminalOffers(t *testing.T) *issue2049Offer {
	t.Helper()
	o := &issue2049Offer{copies: make(chan string, 4), closed: make(chan uint8, 4)}
	old := executeNestedFileOp
	executeNestedFileOp = func(_, _ vfs.VFS, _ string, _ []string, dstDir string, _ bool, _ int, onComplete func()) {
		o.copies <- dstDir
		onComplete()
	}
	t.Cleanup(func() { executeNestedFileOp = old })
	return o
}

func (o *issue2049Offer) closeOffer(reason uint8) { o.closed <- reason }

func (o *issue2049Offer) wantCopy(t *testing.T, dir string, reason uint8) {
	t.Helper()
	select {
	case got := <-o.copies:
		if got != dir {
			t.Fatalf("the offer was copied into %q, want %q", got, dir)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the offer was never copied")
	}
	o.wantClosed(t, reason)
}

func (o *issue2049Offer) wantClosed(t *testing.T, reason uint8) {
	t.Helper()
	select {
	case got := <-o.closed:
		if got != reason {
			t.Fatalf("the offer was closed with %d, want %d", got, reason)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the offer was never closed")
	}
}

// A drop from far2l carries only where it landed: nothing was highlighted,
// so a drop over a folder row follows DropOnFolder like any untracked drop,
// and the offer is closed with what became of the copy (#2049).
func TestIssue2049_TerminalOfferFollowsDropOnFolder(t *testing.T) {
	f := newIssue2049Fixture(t)
	o := recordTerminalOffers(t)
	src := vfs.NewOSVFS(filepath.Dir(f.source))
	names := []string{filepath.Base(f.source)}
	notCancelled := func() bool { return false }
	overSub := far2ldnd.Event{X: testutil.Int16(f.subX), Y: testutil.Int16(f.subY)}

	config.App.DropOnFolder = ""
	f.pf.dropTerminalOffer(overSub, src, names, notCancelled, o.closeOffer)
	o.wantCopy(t, f.dir, far2ldnd.CloseProcessed)

	config.App.DropOnFolder = config.DropOnFolderFolder
	f.pf.dropTerminalOffer(overSub, src, names, func() bool { return true }, o.closeOffer)
	o.wantCopy(t, filepath.Join(f.dir, "sub"), far2ldnd.CloseCancelled)

	// Declining the question closes the offer as cancelled, copying nothing.
	config.App.DropOnFolder = config.DropOnFolderAsk
	f.pf.dropTerminalOffer(overSub, src, names, notCancelled, o.closeOffer)
	w, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok || w.OnResult == nil {
		t.Fatalf("no question on screen, top frame is %T", vtui.FrameManager.GetTopFrame())
	}
	w.OnResult(2)
	vtui.FrameManager.RemoveFrame(w)
	o.wantClosed(t, far2ldnd.CloseCancelled)
	select {
	case dir := <-o.copies:
		t.Fatalf("a declined offer was still copied into %q", dir)
	default:
	}
}

// Where the terminal does not know the position, the offer goes to the
// active panel; with no panel to take it, it is rejected.
func TestIssue2049_TerminalOfferWithoutPosition(t *testing.T) {
	f := newIssue2049Fixture(t)
	o := recordTerminalOffers(t)
	src := vfs.NewOSVFS(filepath.Dir(f.source))
	names := []string{filepath.Base(f.source)}
	unknown := far2ldnd.Event{X: -1, Y: -1}

	f.pf.ActiveIdx = 0
	f.pf.dropTerminalOffer(unknown, src, names, func() bool { return false }, o.closeOffer)
	o.wantCopy(t, f.dir, far2ldnd.CloseProcessed)

	f.fsp.Vfs = issue2049ReadOnlyVFS{f.fsp.Vfs}
	f.pf.dropTerminalOffer(unknown, src, names, func() bool { return false }, o.closeOffer)
	o.wantClosed(t, far2ldnd.CloseRejected)
}

type issue2049ReadOnlyVFS struct{ vfs.VFS }

func (issue2049ReadOnlyVFS) IsReadOnly() bool { return true }
