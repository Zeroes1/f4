package git

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// hunkLineRow is one line of HunkView's list: a hunk's "@@" line (line
// -1) or one of its body lines (the index into diffHunk.lines). Every row
// of a hunk points at that same *diffHunk and reads the picks from there,
// so IsSelected -- vtui.Table's SelectableRow hook, the one a file panel's
// marked files use -- repaints the moment a line or the hunk is toggled,
// with no per-row bookkeeping: a "+"/"-" row is painted as "selected" when
// that line is picked, the "@@" and context rows when the whole hunk is.
type hunkLineRow struct {
	hunk *diffHunk
	line int
}

func (r hunkLineRow) header() bool { return r.line < 0 }

// changeLine reports whether the row is a "+" or "-" line, the rows that
// can be picked one by one.
func (r hunkLineRow) changeLine() bool {
	return !r.header() && isChangeLine(r.hunk.lines[r.line])
}

func (r hunkLineRow) GetCellText(int) string {
	if r.header() {
		return r.hunk.headerLine()
	}
	return displayDiffLine(r.hunk.lines[r.line])
}

func (r hunkLineRow) IsSelected() bool {
	if r.changeLine() {
		return r.hunk.picked[r.line]
	}
	return r.hunk.allPicked()
}

// displayDiffLine makes one raw diff line fit a table cell: a tab becomes
// four spaces and a CRLF file's trailing '\r' is dropped. Only the shown
// text changes -- diffHunk.lines keeps the original bytes for the patch.
func displayDiffLine(line string) string {
	return strings.ReplaceAll(strings.TrimSuffix(line, "\r"), "\t", "    ")
}

// HunkView is F4 on the git status panel: the unstaged changes of one file
// as a list of hunks, `git add -p` style. Insert or Space on a hunk's "@@"
// line or on one of its context lines picks or drops the whole hunk (and
// moves on to the next hunk); on a "+" or "-" line it picks or drops just
// that line (and moves one line down), which is how a hunk is split -- the
// line-level staging GUI clients offer, `git add -p`'s "s" and "e" in one.
// Enter or F2 stages what is picked with `git apply --cached` and closes,
// Esc or F10 closes without staging anything.
//
// Shift+F4 opens the same view over the file's staged changes (a patch
// with staged set, `git reset -p` style): the keys are the same, and Enter
// or F2 takes the picked hunks out of the index with
// `git apply --cached -R`. Only the texts differ between the two.
//
// Like LogView and BranchView it is its own full-screen vtui.Frame pushed
// over the panels, not a panel of its own: it belongs to one file of the
// status list, and closing it returns to that list exactly as it was --
// reloaded, if something was staged.
type HunkView struct {
	vtui.BaseFrame

	frame *vtui.BorderedFrame
	table *vtui.Table

	dir   string
	path  string
	patch *filePatch

	// onStaged runs after a successful apply, before the view closes; the
	// status panel reloads itself there.
	onStaged func()
}

// newHunkView builds a HunkView for an already loaded patch.
func newHunkView(dir, path string, patch *filePatch, onStaged func()) *HunkView {
	frame := vtui.NewBorderedFrame(0, 0, 1, 1, vtui.SingleBox, "")
	frame.ColorBoxIdx = theme.ColPanelBox
	frame.ColorTitleIdx = theme.ColPanelTitle
	frame.ColorBackgroundIdx = theme.ColPanelText

	// One column and no header: the rows are diff text, not records.
	// QuickSearch stays off -- it would take Space, the toggle key here,
	// and a search that hides lines would cut hunks apart.
	table := vtui.NewTable(0, 0, 1, 1, []vtui.TableColumn{{Title: path, MinWidth: 1}})
	table.ShowHeader = false
	table.ShowSeparators = false
	table.ColorBoxIdx = theme.ColPanelBox
	table.ColorTitleIdx = theme.ColPanelColumnTitle
	table.ColorTextIdx = theme.ColPanelText
	table.ColorItemSelectTextIdx = theme.ColPanelSelectedText
	table.ColorSelectedTextIdx = theme.ColPanelCursor
	table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
	table.SetFocus(true)

	var rows []vtui.TableRow
	for _, h := range patch.hunks {
		rows = append(rows, hunkLineRow{hunk: h, line: -1})
		for i := range h.lines {
			rows = append(rows, hunkLineRow{hunk: h, line: i})
		}
	}
	table.SetRows(rows)

	return &HunkView{frame: frame, table: table, dir: dir, path: path, patch: patch, onStaged: onStaged}
}

// GetType identifies HunkView on the frame stack, the next free
// vtui.TypeUser+N slot after LogDiffFilesView's +12 (logdifffiles.go).
func (v *HunkView) GetType() vtui.FrameType { return vtui.TypeUser + 13 }

// msg picks the staging or the unstaging variant of a text.
func (v *HunkView) msg(stage, unstage string) string {
	if v.patch.staged {
		return i18n.Msg(unstage)
	}
	return i18n.Msg(stage)
}

func (v *HunkView) GetTitle() string {
	return fmt.Sprintf(v.msg("GitHunks.PanelTitle", "GitHunks.UnstageTitle"), v.path, v.patch.selectedCount(), len(v.patch.hunks))
}

func (v *HunkView) ResizeConsole(w, h int) {
	top := vtui.FrameManager.WorkspaceTopInset()
	v.SetPosition(0, top, w-1, h-2)
}

// SetPosition keeps BaseFrame's own rectangle and the frame+table pair in
// step, the same split LogDiffFilesView.SetPosition keeps.
func (v *HunkView) SetPosition(x1, y1, x2, y2 int) {
	v.BaseFrame.SetPosition(x1, y1, x2, y2)
	v.frame.SetPosition(x1, y1, x2, y2)
	b := v.frame.GetBorderThickness()
	ix1, iy1, ix2, iy2 := x1+b, y1+b, x2-b, y2-b
	if ix2 < ix1 {
		ix2 = ix1
	}
	if iy2 < iy1 {
		iy2 = iy1
	}
	v.table.SetPosition(ix1, iy1, ix2, iy2)
}

func (v *HunkView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{"", v.msg("GitHunks.Stage", "GitHunks.Unstage"), "", "", "", "", "", "", "", i18n.Msg("GitLog.Close")},
	}
}

// ProcessKey: Esc/F10 close; Insert/Space toggle the line or the hunk
// under the cursor; Enter/F2 stage (or unstage) what is picked. Everything else is
// table navigation.
func (v *HunkView) ProcessKey(e *vtinput.InputEvent) bool {
	if e == nil || !e.KeyDown {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		v.SetExitCode(-1)
		return true
	}
	mods := e.ControlKeyState & (vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.LeftAltPressed | vtinput.RightAltPressed | vtinput.ShiftPressed)
	if mods == 0 {
		switch e.VirtualKeyCode {
		case vtinput.VK_INSERT, vtinput.VK_SPACE:
			v.toggle()
			return true
		case vtinput.VK_RETURN, vtinput.VK_F2:
			v.stageSelected()
			return true
		}
	}
	return v.table.ProcessKey(e)
}

func (v *HunkView) ProcessMouse(e *vtinput.InputEvent) bool { return v.table.ProcessMouse(e) }

func (v *HunkView) Show(scr *vtui.ScreenBuf) {
	v.frame.SetTitle(v.GetTitle())
	v.frame.Show(scr)
	v.table.Show(scr)
}

// cursorRow returns the row under the cursor.
func (v *HunkView) cursorRow() (hunkLineRow, bool) {
	idx := v.table.RowAt(v.table.SelectPos)
	if idx < 0 || idx >= len(v.table.Rows) {
		return hunkLineRow{}, false
	}
	row, ok := v.table.Rows[idx].(hunkLineRow)
	return row, ok
}

// toggle flips what is under the cursor. On a "+" or "-" line that is the
// line alone, and the cursor steps one row down, the way Insert marks a
// file and steps down in a file panel. On the "@@" line or a context line
// it is the whole hunk -- picked whole unless it already is, dropped whole
// if it is -- and the cursor moves to the next hunk's "@@" line, so Insert,
// Insert, ... walks through the file hunk by hunk. On the last row (or the
// last hunk) the cursor stays put.
func (v *HunkView) toggle() {
	row, ok := v.cursorRow()
	if !ok {
		return
	}
	if row.changeLine() {
		row.hunk.picked[row.line] = !row.hunk.picked[row.line]
		if v.table.SelectPos+1 < v.table.ItemCount {
			v.table.SelectPos++
			v.table.EnsureVisible()
		}
	} else {
		row.hunk.pickAll(!row.hunk.allPicked())
		for pos := v.table.SelectPos + 1; pos < v.table.ItemCount; pos++ {
			idx := v.table.RowAt(pos)
			if idx < 0 || idx >= len(v.table.Rows) {
				continue
			}
			next, ok := v.table.Rows[idx].(hunkLineRow)
			if ok && next.header() && next.hunk != row.hunk {
				v.table.SelectPos = pos
				v.table.EnsureVisible()
				break
			}
		}
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

// stageSelected applies the picked lines to the index -- or, for staged
// ones, takes them out of it -- and closes the view. With nothing picked
// it only says so; a pick buildPatch cannot turn into a patch, or a failed
// apply (the file changed on disk or in the index since the view was
// opened, say), keeps the view open with the reason.
func (v *HunkView) stageSelected() {
	n := v.patch.selectedCount()
	if n == 0 {
		toast.Show(i18n.Msg("GitHunks.NothingSelected"), 3e9)
		return
	}
	if err := applyFilePatch(context.Background(), v.dir, v.patch); err != nil {
		switch {
		case errors.Is(err, errWholeFileOnly):
			toast.Show(i18n.Msg("GitHunks.WholeFileOnly"), 3e9)
			return
		case errors.Is(err, errNoNewlineInside):
			toast.Show(i18n.Msg("GitHunks.NoNewlineInside"), 3e9)
			return
		}
		toast.Show(fmt.Sprintf(v.msg("GitHunks.ApplyFailed", "GitHunks.UnapplyFailed"), err), 3e9)
		return
	}
	toast.Show(fmt.Sprintf(v.msg("GitHunks.Staged", "GitHunks.Unstaged"), n, len(v.patch.hunks), v.path), 3e9)
	if v.onStaged != nil {
		v.onStaged()
	}
	v.SetExitCode(-1)
}

// showHunks is F4 on the status panel: load the unstaged diff of the file
// under the cursor and open a HunkView over it. `git diff` of one file is
// quick, so this runs synchronously, the same way the panel's own
// `git status` does.
func (p *statusPanel) showHunks() { p.showHunksOf(false) }

// showStagedHunks is Shift+F4: the same over the file's staged diff, to
// take hunks back out of the index.
func (p *statusPanel) showStagedHunks() { p.showHunksOf(true) }

func (p *statusPanel) showHunksOf(staged bool) {
	entry, ok := p.selectedEntry()
	if !ok {
		return
	}
	v, err := p.openHunkView(entry, staged)
	if err != nil {
		if errors.Is(err, errNoHunks) {
			msg := "GitHunks.NoHunks"
			if staged {
				msg = "GitHunks.NoStagedHunks"
			}
			toast.Show(fmt.Sprintf(i18n.Msg(msg), entry.Path), 3e9)
		} else {
			toast.Show(fmt.Sprintf(i18n.Msg("GitHunks.OpenFailed"), err), 3e9)
		}
		return
	}
	if vtui.FrameManager != nil {
		v.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
		vtui.FrameManager.AddScreen(v)
	}
}

// openHunkView builds the HunkView for entry -- over its staged changes
// when staged is set; after a successful apply it reloads the panel with
// the cursor kept on entry, as Insert does.
//
// A staged rename has no hunks to unstage in parts: `git diff --cached` of
// the new path alone shows it as an added file, and reversing that would
// drop the new path from the index while the old one stays deleted there.
// Insert unstages the rename whole.
func (p *statusPanel) openHunkView(entry statusEntry, staged bool) (*HunkView, error) {
	if staged && entry.OrigPath != "" {
		return nil, errNoHunks
	}
	patch, err := loadFilePatch(context.Background(), p.dir, entry.Path, staged)
	if err != nil {
		return nil, err
	}
	return newHunkView(p.dir, entry.Path, patch, func() {
		if err := p.reload(); err != nil {
			toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RefreshFailed"), err), 3e9)
			return
		}
		p.restoreSelectionByPath(entry.Path)
	}), nil
}
