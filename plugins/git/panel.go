package git

import (
	"context"
	"errors"
	"fmt"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Column indices into statusRow.GetCellText, mirroring plugins/proclist's
// colPID/colName/... constants.
const (
	colStatus = iota
	colPath
)

func statusColumns() []vtui.TableColumn {
	return []vtui.TableColumn{
		{Title: i18n.Msg("GitStatus.ColumnStatus"), Width: 4},
		{Title: i18n.Msg("GitStatus.ColumnPath"), MinWidth: 12},
	}
}

// statusRow adapts one statusEntry to vtui.Table's TableRow contract.
type statusRow struct {
	entry statusEntry
}

func (r statusRow) GetCellText(col int) string {
	switch col {
	case colStatus:
		return r.entry.XY
	case colPath:
		if r.entry.OrigPath != "" {
			return fmt.Sprintf("%s -> %s", r.entry.OrigPath, r.entry.Path)
		}
		return r.entry.Path
	default:
		return ""
	}
}

// statusPanel is the panel a PanelProvider.Open returns for f4.gitstatus: a
// theme-colored vtui.Table drawn inside a vtui.BorderedFrame, the same pair
// plugins/proclist/panel.go composes for its own live process list.
//
// Unlike ProcList, this panel does not refresh on a ticker: `git status` is
// a point-in-time snapshot the user asks for (opening the panel, or F5), not
// a continuously changing view like /proc, so there is no background
// goroutine or Close() cleanup to speak of here -- both the initial load and
// F5 run git synchronously, on the UI goroutine, the same "construct
// quickly" trade internal/app/compare_content_ui.go's first version makes
// for its own local, sub-second git/file work.
type statusPanel struct {
	frame *vtui.BorderedFrame
	table *vtui.Table
	dir   string // repository-relative directory git status ran in; also this panel's identity for GetSelectedName's callers.

	branch   string
	detached bool
}

// newStatusPanel is a vfs.PanelProvider.Open callback: dir comes from
// ctx.Current.Path, the active panel's current directory. If dir is not
// inside a git repository (or git is missing from PATH, or dir is not a
// local OS path git can chdir into at all), the underlying `git status`
// call fails and this returns that error; internal/panel/plugins.go's
// OpenRegisteredPanelProvider turns it into a toast for the user, so there
// is nothing extra to do here.
func newStatusPanel(ctx vfs.PanelContext) (vfs.PanelController, error) {
	dir := ctx.Current.Path
	if dir == "" {
		return nil, errors.New(i18n.Msg("GitStatus.NoPath"))
	}

	frame := vtui.NewBorderedFrame(0, 0, 1, 1, vtui.SingleBox, "")
	frame.ColorBoxIdx = theme.ColPanelBox
	frame.ColorTitleIdx = theme.ColPanelTitle
	frame.ColorBackgroundIdx = theme.ColPanelText

	table := vtui.NewTable(0, 0, 1, 1, statusColumns())
	table.Sortable = true
	table.QuickSearch = true
	table.ColorBoxIdx = theme.ColPanelBox
	table.ColorTitleIdx = theme.ColPanelColumnTitle
	table.ColorTextIdx = theme.ColPanelText
	table.ColorItemSelectTextIdx = theme.ColPanelSelectedText
	// No custom SortCompare: both columns are plain text (the status code
	// and the path), which is exactly what Table's default cell-text
	// comparator already sorts correctly -- unlike ProcList's numeric
	// PID/Mem/CPU% columns (plugins/proclist/panel.go), nothing here needs
	// its own comparator.
	table.SetSort(colPath, true)

	p := &statusPanel{frame: frame, table: table, dir: dir}
	p.SetFocus(false)
	p.SetPosition(ctx.Bounds[0], ctx.Bounds[1], ctx.Bounds[2], ctx.Bounds[3])

	if err := p.reload(); err != nil {
		return nil, err
	}
	return p, nil
}

// reload runs `git status --porcelain=v2 --branch` in p.dir and replaces the
// table's rows. Called once from newStatusPanel and again on every F5.
func (p *statusPanel) reload() error {
	output, err := runGitIn(context.Background(), p.dir, "status", "--porcelain=v2", "--branch")
	if err != nil {
		return errors.New(firstLine(string(output), err))
	}
	result := parseStatus(output)
	p.branch = result.Branch
	p.detached = result.Detached

	rows := make([]vtui.TableRow, len(result.Entries))
	for i, entry := range result.Entries {
		rows[i] = statusRow{entry: entry}
	}
	p.table.SetRows(rows)
	return nil
}

// firstLine turns a failed git invocation into a one-line message: git
// itself explains the failure on stderr (captured into output by runGitIn),
// and prepending err would only add exec's own uninformative "exit status
// 128" ahead of it.
func firstLine(output string, err error) string {
	for i := 0; i < len(output); i++ {
		if output[i] == '\n' {
			output = output[:i]
			break
		}
	}
	if output == "" {
		return err.Error()
	}
	return output
}

func (p *statusPanel) SetPosition(x1, y1, x2, y2 int) {
	p.frame.SetPosition(x1, y1, x2, y2)
	b := p.frame.GetBorderThickness()
	ix1, iy1, ix2, iy2 := x1+b, y1+b, x2-b, y2-b
	if ix2 < ix1 {
		ix2 = ix1
	}
	if iy2 < iy1 {
		iy2 = iy1
	}
	p.table.SetPosition(ix1, iy1, ix2, iy2)
}

func (p *statusPanel) GetPosition() (int, int, int, int) { return p.frame.GetPosition() }

func (p *statusPanel) SetFocus(focused bool) {
	if focused {
		p.table.ColorSelectedTextIdx = theme.ColPanelCursor
		p.table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
	} else {
		p.table.ColorSelectedTextIdx = theme.ColPanelInactiveCursor
		p.table.ColorItemSelectCursorIdx = theme.ColPanelInactiveSelectedCursor
	}
	p.table.SetFocus(focused)
}

func (p *statusPanel) IsFocused() bool { return p.table.IsFocused() }

// ProcessKey adds F5 (refresh), Enter (diff, diff.go) and Insert
// (stage/unstage, stage.go) on top of the table's own
// navigation/sort/quick-search handling. None of the three is a letter key:
// QuickSearch claims printable characters while the table is focused
// (plugins/proclist/panel.go avoids the same trap by keying its own actions
// off F-keys) -- and that includes plain Space, which is why staging is
// bound to Insert instead of the Space lazygit/tig use, following the
// existing "mark an item" key of Far/Norton-Commander-style file panels
// (internal/panel/menukeys.go's isAddItemKey) rather than a foreign tool's
// convention. F5/Enter are the refresh and open gestures a file panel
// already uses -- this panel has no file Copy or directory-enter of its own
// for either to collide with.
func (p *statusPanel) ProcessKey(e *vtinput.InputEvent) bool {
	if e != nil && e.Type == vtinput.KeyEventType && e.KeyDown {
		ctrl := e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed) != 0
		alt := e.ControlKeyState&(vtinput.LeftAltPressed|vtinput.RightAltPressed) != 0
		shift := e.ControlKeyState&vtinput.ShiftPressed != 0
		if !ctrl && !alt && !shift {
			switch e.VirtualKeyCode {
			case vtinput.VK_F5:
				if err := p.reload(); err != nil {
					toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RefreshFailed"), err), 3e9)
				}
				if vtui.FrameManager != nil {
					vtui.FrameManager.Redraw()
				}
				return true
			case vtinput.VK_RETURN:
				p.showDiff()
				return true
			case vtinput.VK_INSERT:
				p.toggleStage()
				return true
			}
		}
	}
	return p.table.ProcessKey(e)
}

func (p *statusPanel) ProcessMouse(e *vtinput.InputEvent) bool { return p.table.ProcessMouse(e) }

func (p *statusPanel) selectedEntry() (statusEntry, bool) {
	idx := p.table.RowAt(p.table.SelectPos)
	if idx < 0 || idx >= len(p.table.Rows) {
		return statusEntry{}, false
	}
	row, ok := p.table.Rows[idx].(statusRow)
	if !ok {
		return statusEntry{}, false
	}
	return row.entry, true
}

// GetSelectedName reports the path under the cursor, the closest thing this
// panel has to a file panel's selected file name.
func (p *statusPanel) GetSelectedName() string {
	entry, ok := p.selectedEntry()
	if !ok {
		return ""
	}
	return entry.Path
}

func (p *statusPanel) SetContext(vfs.PanelContext) {}

func (p *statusPanel) Show(scr *vtui.ScreenBuf) {
	p.frame.SetTitle(p.title())
	p.frame.Show(scr)
	p.table.Show(scr)
}

// title renders the frame's border title: the branch name (or a fixed
// "detached HEAD" caption) and the current change count, the same shape
// plugins/proclist/panel.go's own Show uses for "ProcList (%d)".
func (p *statusPanel) title() string {
	branch := p.branch
	if p.detached {
		branch = i18n.Msg("GitStatus.Detached")
	}
	return fmt.Sprintf(i18n.Msg("GitStatus.PanelTitle"), branch, p.table.ItemCount)
}

func (p *statusPanel) Close() error { return nil }
