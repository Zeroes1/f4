package git

import (
	"context"
	"errors"
	"fmt"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Column indices into branchRow.GetCellText, mirroring statusRow's
// colStatus/colPath (panel.go) and logRow's colHash/... (log.go).
const (
	colBranchCurrent = iota
	colBranchName
)

func branchColumns() []vtui.TableColumn {
	return []vtui.TableColumn{
		{Title: i18n.Msg("GitBranch.ColumnCurrent"), Width: 1},
		{Title: i18n.Msg("GitBranch.ColumnName"), MinWidth: 12},
	}
}

// branchRow adapts one branchEntry to vtui.Table's TableRow contract, the
// same role statusRow (panel.go) and logRow (log.go) play for their own
// entries.
type branchRow struct{ entry branchEntry }

func (r branchRow) GetCellText(col int) string {
	switch col {
	case colBranchCurrent:
		if r.entry.Current {
			return "*"
		}
		return ""
	case colBranchName:
		return r.entry.Name
	default:
		return ""
	}
}

// BranchView is Ctrl+S on the status panel (panel.go's ProcessKey): a
// read-only list of the repository's local branches, built from the same
// vtui.BorderedFrame+vtui.Table pair LogView (logview.go) composes for the
// commit log. Like LogView, and unlike statusPanel itself, it is not a
// vfs.PanelController occupying a panel slot -- it is its own full-screen
// vtui.Frame pushed with vtui.FrameManager.AddScreen, so it needs no "go
// back to what was open before" stack of its own: Escape simply pops this
// workspace and the status panel underneath is exactly as it was.
//
// Sorting is not offered, the same as LogView: a short branch list has no
// order worth re-sorting away from the one `git branch --list` already
// gives (current branch first is not even guaranteed, but alphabetical
// churn from a click would help nobody). QuickSearch stays on, the same
// type-to-filter gesture every other list in this plugin has.
type BranchView struct {
	vtui.BaseFrame

	frame  *vtui.BorderedFrame
	table  *vtui.Table
	dir    string       // repository-relative directory `git branch`/switch runs in.
	status *statusPanel // the panel Ctrl+S was pressed on; refreshed after a successful switch.
}

// newBranchView builds and immediately loads a BranchView for dir. Like
// newLogView (logview.go), loading runs synchronously on the caller's
// goroutine: a local `git branch --list` is the same "construct quickly"
// trade panel.go's own doc comment already accepts for `git status`.
func newBranchView(dir string, status *statusPanel) (*BranchView, error) {
	frame := vtui.NewBorderedFrame(0, 0, 1, 1, vtui.SingleBox, "")
	frame.ColorBoxIdx = theme.ColPanelBox
	frame.ColorTitleIdx = theme.ColPanelTitle
	frame.ColorBackgroundIdx = theme.ColPanelText

	table := vtui.NewTable(0, 0, 1, 1, branchColumns())
	table.QuickSearch = true
	table.ColorBoxIdx = theme.ColPanelBox
	table.ColorTitleIdx = theme.ColPanelColumnTitle
	table.ColorTextIdx = theme.ColPanelText
	table.ColorItemSelectTextIdx = theme.ColPanelSelectedText
	// This view is always the focused (indeed the only) widget on its own
	// workspace, the same reasoning newLogView's own comment gives for the
	// same fixed-to-focused cursor colors.
	table.ColorSelectedTextIdx = theme.ColPanelCursor
	table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
	table.SetFocus(true)

	bv := &BranchView{frame: frame, table: table, dir: dir, status: status}
	if err := bv.reload(); err != nil {
		return nil, err
	}
	return bv, nil
}

// reload runs `git branch --list --no-color` in bv.dir and replaces the
// table's rows. Called once from newBranchView, again on every F5, and
// again after a successful switch (switchBranch below) so the "*" marker
// moves to the branch just switched to. --no-color is defensive, the same
// as LogView's own reload: nothing here asks git for color, but it costs
// nothing to say so explicitly rather than rely on that staying true.
func (bv *BranchView) reload() error {
	output, err := runGitIn(context.Background(), bv.dir, "branch", "--list", "--no-color")
	if err != nil {
		return errors.New(firstLine(string(output), err))
	}
	entries := parseBranchList(output)
	rows := make([]vtui.TableRow, len(entries))
	for i, e := range entries {
		rows[i] = branchRow{entry: e}
	}
	bv.table.SetRows(rows)
	return nil
}

func (bv *BranchView) selectedEntry() (branchEntry, bool) {
	idx := bv.table.RowAt(bv.table.SelectPos)
	if idx < 0 || idx >= len(bv.table.Rows) {
		return branchEntry{}, false
	}
	row, ok := bv.table.Rows[idx].(branchRow)
	if !ok {
		return branchEntry{}, false
	}
	return row.entry, true
}

// GetType identifies BranchView on the frame stack, the next free
// vtui.TypeUser+N slot after LogView's own +10 (logview.go's doc comment
// traces the chain back to internal/diffview.DiffView's +9).
func (bv *BranchView) GetType() vtui.FrameType { return vtui.TypeUser + 11 }

func (bv *BranchView) GetTitle() string {
	return fmt.Sprintf(i18n.Msg("GitBranch.PanelTitle"), bv.table.ItemCount)
}

func (bv *BranchView) ResizeConsole(w, h int) {
	top := vtui.FrameManager.WorkspaceTopInset()
	bv.SetPosition(0, top, w-1, h-2)
}

// SetPosition keeps the embedded BaseFrame's own X1..X2 in sync (so
// GetPosition/HitTest, both inherited from it, stay correct) and repositions
// the frame+table pair inside it, the same split LogView.SetPosition
// (logview.go) keeps for the same two widgets.
func (bv *BranchView) SetPosition(x1, y1, x2, y2 int) {
	bv.BaseFrame.SetPosition(x1, y1, x2, y2)
	bv.frame.SetPosition(x1, y1, x2, y2)
	b := bv.frame.GetBorderThickness()
	ix1, iy1, ix2, iy2 := x1+b, y1+b, x2-b, y2-b
	if ix2 < ix1 {
		ix2 = ix1
	}
	if iy2 < iy1 {
		iy2 = iy1
	}
	bv.table.SetPosition(ix1, iy1, ix2, iy2)
}

func (bv *BranchView) GetKeyLabels() *vtui.KeySet {
	return &vtui.KeySet{
		Normal: vtui.KeyBarLabels{"", "", "", "", i18n.Msg("GitBranch.Refresh"), "", "", "", "", i18n.Msg("GitBranch.Close")},
	}
}

// ProcessKey handles Esc/F10 (close, unconditionally -- the same modifier-
// agnostic close LogView.ProcessKey gives its own Esc/F10), F5 (refresh) and
// Enter (switch, switchBranch below), then falls through to the table for
// navigation and QuickSearch, the same layering LogView.ProcessKey
// (logview.go) uses for its own extra keys.
func (bv *BranchView) ProcessKey(e *vtinput.InputEvent) bool {
	if e == nil || !e.KeyDown {
		return false
	}
	switch e.VirtualKeyCode {
	case vtinput.VK_ESCAPE, vtinput.VK_F10:
		bv.SetExitCode(-1)
		return true
	}
	ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
	alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
	shift := e.ControlKeyState&vtinput.ShiftPressed != 0
	if !ctrl && !alt && !shift {
		switch e.VirtualKeyCode {
		case vtinput.VK_F5:
			if err := bv.reload(); err != nil {
				toast.Show(fmt.Sprintf(i18n.Msg("GitBranch.RefreshFailed"), err), 3e9)
			}
			if vtui.FrameManager != nil {
				vtui.FrameManager.Redraw()
			}
			return true
		case vtinput.VK_RETURN:
			bv.switchBranch()
			return true
		}
	}
	return bv.table.ProcessKey(e)
}

func (bv *BranchView) ProcessMouse(e *vtinput.InputEvent) bool { return bv.table.ProcessMouse(e) }

func (bv *BranchView) Show(scr *vtui.ScreenBuf) {
	bv.frame.SetTitle(bv.GetTitle())
	bv.frame.Show(scr)
	bv.table.Show(scr)
}

// switchBranch is Enter on a listed branch: `git switch <branch>`, not
// `git checkout <branch>` -- git switch (added in git 2.23) exists
// specifically for this one gesture and gives a clearer error than
// checkout's own, more overloaded one when the working tree has local
// changes switching would overwrite. That error is exactly what this
// function surfaces as a toast on failure: it makes no attempt of its own
// to stash, merge or otherwise resolve the conflict git itself refuses --
// the ticket's own instruction for this case, and the same "run the
// command, toast git's own message on failure" shape toggleStage (stage.go)
// and runCommit (commit.go) already use for git add/restore/commit.
//
// Switching to the branch already checked out is not special-cased: git
// switch on the current branch is a harmless no-op ("Already on
// '<branch>'"), so there is nothing here worth guarding against that git
// does not already handle on its own.
//
// On success this reloads both the branch list (so the "*" marker moves to
// the new current branch) and the underlying status panel (so its title's
// branch name and its working-tree entries reflect the new HEAD) --
// BranchView stays open afterward, the same way LogView stays open after
// Enter shows a diff, rather than popping itself: nothing about switching
// branches implies "done looking at branches."
func (bv *BranchView) switchBranch() {
	entry, ok := bv.selectedEntry()
	if !ok {
		return
	}

	output, err := runGitIn(context.Background(), bv.dir, "switch", entry.Name)
	if err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitBranch.SwitchFailed"), firstLine(string(output), err)), 3e9)
		return
	}

	if err := bv.reload(); err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitBranch.RefreshFailed"), err), 3e9)
	}
	if bv.status != nil {
		if err := bv.status.reload(); err != nil {
			toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RefreshFailed"), err), 3e9)
		}
	}
	toast.Show(fmt.Sprintf(i18n.Msg("GitBranch.SwitchDone"), entry.Name), 3e9)
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

// showBranches is Ctrl+S on the status panel (panel.go's ProcessKey doc
// comment explains why Ctrl+S: it is free, and already precedent for a
// view-local "S" gesture -- internal/media/image_view.go's own Ctrl+S
// toggles its slide show the same way, scoped to that view alone, never a
// global action this one could shadow). Opens a BranchView of this
// repository's local branches on top of the status panel, the same "push a
// screen" gesture showLog (log.go) already uses for Ctrl+E.
func (p *statusPanel) showBranches() {
	bv, err := newBranchView(p.dir, p)
	if err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitBranch.OpenFailed"), err), 3e9)
		return
	}
	if vtui.FrameManager != nil {
		bv.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
		vtui.FrameManager.AddScreen(bv)
	}
}
