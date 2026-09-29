package svcmgr

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// Column indices into serviceRow.GetCellText.
const (
	colName = iota
	colDisplay
	colState
	colPID
)

func serviceColumns() []vtui.TableColumn {
	return []vtui.TableColumn{
		{Title: i18n.Msg("SvcMgr.ColumnName"), Width: 24},
		{Title: i18n.Msg("SvcMgr.ColumnDisplay"), MinWidth: 16},
		{Title: i18n.Msg("SvcMgr.ColumnState"), Width: 10},
		{Title: i18n.Msg("SvcMgr.ColumnPID"), Width: 7},
	}
}

// serviceRow adapts one service to vtui.Table's TableRow contract.
type serviceRow struct {
	svc service
}

func (r serviceRow) GetCellText(col int) string {
	switch col {
	case colName:
		return r.svc.Name
	case colDisplay:
		return r.svc.Display
	case colState:
		return stateName(r.svc.State)
	case colPID:
		if r.svc.PID == 0 {
			return ""
		}
		return strconv.FormatUint(uint64(r.svc.PID), 10)
	}
	return ""
}

// servicesPanel is the panel a PanelProvider.Open returns: a theme-colored
// vtui.Table inside a vtui.BorderedFrame, built the way plugins/git's status
// panel is. The list is a snapshot taken when the panel opens and again on
// F5; list is where it comes from (the Service Control Manager in
// production, a fake in tests).
type servicesPanel struct {
	frame *vtui.BorderedFrame
	table *vtui.Table
	list  func() ([]service, error)
}

func newServicesPanel(ctx vfs.PanelContext, list func() ([]service, error)) (vfs.PanelController, error) {
	frame := vtui.NewBorderedFrame(0, 0, 1, 1, vtui.SingleBox, "")
	frame.ColorBoxIdx = theme.ColPanelBox
	frame.ColorTitleIdx = theme.ColPanelTitle
	frame.ColorBackgroundIdx = theme.ColPanelText

	table := vtui.NewTable(0, 0, 1, 1, serviceColumns())
	table.Sortable = true
	table.QuickSearch = true
	table.ColorBoxIdx = theme.ColPanelBox
	table.ColorTitleIdx = theme.ColPanelColumnTitle
	table.ColorTextIdx = theme.ColPanelText
	table.ColorItemSelectTextIdx = theme.ColPanelSelectedText
	table.SetSort(colDisplay, true)

	p := &servicesPanel{frame: frame, table: table, list: list}
	p.SetFocus(false)
	p.SetPosition(ctx.Bounds[0], ctx.Bounds[1], ctx.Bounds[2], ctx.Bounds[3])
	if err := p.reload(); err != nil {
		return nil, err
	}
	return p, nil
}

// reload replaces the rows with a fresh list, keeping the cursor on the same
// service when it is still there.
func (p *servicesPanel) reload() error {
	services, err := p.list()
	if err != nil {
		return err
	}
	keep, _ := p.selectedService()
	rows := make([]vtui.TableRow, len(services))
	for i, s := range services {
		rows[i] = serviceRow{svc: s}
	}
	p.table.SetRows(rows)
	if keep.Name != "" {
		p.selectByName(keep.Name)
	}
	return nil
}

func (p *servicesPanel) selectByName(name string) {
	for pos := 0; pos < p.table.ItemCount; pos++ {
		idx := p.table.RowAt(pos)
		if idx < 0 || idx >= len(p.table.Rows) {
			continue
		}
		if row, ok := p.table.Rows[idx].(serviceRow); ok && strings.EqualFold(row.svc.Name, name) {
			p.table.SelectPos = pos
			p.table.EnsureVisible()
			return
		}
	}
}

func (p *servicesPanel) selectedService() (service, bool) {
	idx := p.table.RowAt(p.table.SelectPos)
	if idx < 0 || idx >= len(p.table.Rows) {
		return service{}, false
	}
	row, ok := p.table.Rows[idx].(serviceRow)
	if !ok {
		return service{}, false
	}
	return row.svc, true
}

// GetSelectedName reports the service name under the cursor.
func (p *servicesPanel) GetSelectedName() string {
	s, _ := p.selectedService()
	return s.Name
}

func (p *servicesPanel) SetPosition(x1, y1, x2, y2 int) {
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

func (p *servicesPanel) GetPosition() (int, int, int, int) { return p.frame.GetPosition() }

func (p *servicesPanel) SetFocus(focused bool) {
	if focused {
		p.table.ColorSelectedTextIdx = theme.ColPanelCursor
		p.table.ColorItemSelectCursorIdx = theme.ColPanelSelectedCursor
	} else {
		p.table.ColorSelectedTextIdx = theme.ColPanelInactiveCursor
		p.table.ColorItemSelectCursorIdx = theme.ColPanelInactiveSelectedCursor
	}
	p.table.SetFocus(focused)
}

func (p *servicesPanel) IsFocused() bool { return p.table.IsFocused() }

var _ vfs.PanelKeyProvider = (*servicesPanel)(nil)

// PanelKeys declares F5 (reload). It is an F-key rather than a letter because
// QuickSearch claims printable characters while the table has the focus.
func (p *servicesPanel) PanelKeys() []vfs.PanelKey {
	return []vfs.PanelKey{
		{VK: vtinput.VK_F5, Label: i18n.Msg("SvcMgr.KeyBar.Refresh"), Run: p.refresh},
	}
}

func (p *servicesPanel) ProcessKey(e *vtinput.InputEvent) bool {
	if vfs.DispatchPanelKey(p.PanelKeys(), e) {
		return true
	}
	return p.table.ProcessKey(e)
}

func (p *servicesPanel) ProcessMouse(e *vtinput.InputEvent) bool { return p.table.ProcessMouse(e) }

// refresh is F5: reload the list, reporting a failure as a toast and keeping
// the old rows.
func (p *servicesPanel) refresh() {
	if err := p.reload(); err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("SvcMgr.RefreshFailed"), err), 3e9)
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

func (p *servicesPanel) SetContext(vfs.PanelContext) {}

func (p *servicesPanel) Show(scr *vtui.ScreenBuf) {
	p.frame.SetTitle(fmt.Sprintf(i18n.Msg("SvcMgr.PanelTitle"), p.table.ItemCount))
	p.frame.Show(scr)
	p.table.Show(scr)
}

func (p *servicesPanel) Close() error { return nil }
