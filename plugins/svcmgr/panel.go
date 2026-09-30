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
	ctl   controller
	det   detailer
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

	p := &servicesPanel{frame: frame, table: table, list: list, ctl: serviceController, det: serviceDetailer}
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

// PanelKeys declares F5 (reload), Enter (the service's details) and, on the Shift row, Start (Shift+F1), Stop
// (Shift+F2, after a confirmation) and Pause/Resume (Shift+F3, whichever the
// service's state calls for). They are F-keys rather than letters because
// QuickSearch claims printable characters while the table has the focus, and
// they act on the service under the cursor, so they stand down on an empty list.
func (p *servicesPanel) PanelKeys() []vfs.PanelKey {
	return []vfs.PanelKey{
		{VK: vtinput.VK_F5, Label: i18n.Msg("SvcMgr.KeyBar.Refresh"), Run: p.refresh},
		{VK: vtinput.VK_RETURN, Run: p.showDetails, Enabled: p.hasSelected},
		{VK: vtinput.VK_F1, Mods: vtinput.ShiftPressed, Label: i18n.Msg("SvcMgr.KeyBar.Start"), Run: p.start, Enabled: p.hasSelected},
		{VK: vtinput.VK_F2, Mods: vtinput.ShiftPressed, Label: i18n.Msg("SvcMgr.KeyBar.Stop"), Run: p.confirmStop, Enabled: p.hasSelected},
		{VK: vtinput.VK_F3, Mods: vtinput.ShiftPressed, Label: i18n.Msg("SvcMgr.KeyBar.Pause"), Run: p.pauseOrResume, Enabled: p.hasSelected},
		{VK: vtinput.VK_F4, Mods: vtinput.ShiftPressed, Label: i18n.Msg("SvcMgr.KeyBar.StartType"), Run: p.chooseStartType, Enabled: p.hasSelected},
	}
}

func (p *servicesPanel) hasSelected() bool {
	_, ok := p.selectedService()
	return ok
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

// detailsText renders a service and its configuration as the lines the
// details window shows.
func detailsText(svc service, d serviceDetails) string {
	lines := []string{
		fmt.Sprintf(i18n.Msg("SvcMgr.DetailName"), svc.Name),
		fmt.Sprintf(i18n.Msg("SvcMgr.DetailDisplay"), svc.Display),
		fmt.Sprintf(i18n.Msg("SvcMgr.DetailState"), stateName(svc.State)),
		fmt.Sprintf(i18n.Msg("SvcMgr.DetailStart"), startTypeName(d.StartType, d.Delayed)),
		fmt.Sprintf(i18n.Msg("SvcMgr.DetailAccount"), d.Account),
		fmt.Sprintf(i18n.Msg("SvcMgr.DetailPath"), d.BinaryPath),
	}
	if len(d.DependsOn) > 0 {
		lines = append(lines, fmt.Sprintf(i18n.Msg("SvcMgr.DetailDepends"), strings.Join(d.DependsOn, ", ")))
	}
	if d.Description != "" {
		lines = append(lines, "", d.Description)
	}
	return strings.Join(lines, "\n")
}

// showDetails is Enter: how the service starts, what it runs and as whom,
// and its description, read on demand for the service under the cursor.
func (p *servicesPanel) showDetails() {
	svc, ok := p.selectedService()
	if !ok {
		return
	}
	d, err := p.det.Details(svc.Name)
	if err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("SvcMgr.ActionFailed"), svc.Name, err), 3e9)
		return
	}
	if vtui.FrameManager == nil {
		return
	}
	vtui.ShowMessageEx(i18n.Msg("SvcMgr.DetailsTitle"), detailsText(svc, d), []string{i18n.Msg("vtui.Ok")}, vtui.MessageInfo)
}

// startTypeChoices are the start types the panel offers, in the order of the
// dialog's buttons. Boot and System are for drivers and are not offered.
var startTypeChoices = []uint32{startAuto, startManual, startDisabled}

// chooseStartType is Shift+F4: a dialog with one button per start type.
func (p *servicesPanel) chooseStartType() {
	svc, ok := p.selectedService()
	if !ok || vtui.FrameManager == nil {
		return
	}
	buttons := make([]string, 0, len(startTypeChoices)+1)
	for _, t := range startTypeChoices {
		buttons = append(buttons, startTypeName(t, false))
	}
	buttons = append(buttons, i18n.Msg("vtui.Cancel"))
	dlg := vtui.ShowMessageEx(i18n.Msg("SvcMgr.StartTypeTitle"),
		fmt.Sprintf(i18n.Msg("SvcMgr.StartTypePrompt"), svc.Name), buttons, vtui.MessageInfo)
	if dlg == nil {
		return
	}
	dlg.OnResult = func(code int) {
		if code >= 0 && code < len(startTypeChoices) {
			p.setStartType(startTypeChoices[code])
		}
	}
}

// setStartType applies a start type to the service under the cursor.
func (p *servicesPanel) setStartType(startType uint32) {
	p.run(func(name string) error { return p.ctl.SetStartType(name, startType) })
}

// run applies one action to the service under the cursor, reports a failure
// as a toast, and reloads the list either way so the new (often still
// pending) state shows.
func (p *servicesPanel) run(action func(name string) error) {
	svc, ok := p.selectedService()
	if !ok {
		return
	}
	if err := action(svc.Name); err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("SvcMgr.ActionFailed"), svc.Name, err), 3e9)
	}
	p.refresh()
}

func (p *servicesPanel) start() { p.run(p.ctl.Start) }

// confirmStop asks before stopping: a stopped service takes whatever depends
// on it down with it.
func (p *servicesPanel) confirmStop() {
	svc, ok := p.selectedService()
	if !ok || vtui.FrameManager == nil {
		return
	}
	confirm := vtui.ShowMessageEx(i18n.Msg("SvcMgr.StopTitle"),
		fmt.Sprintf(i18n.Msg("SvcMgr.StopConfirm"), svc.Name),
		[]string{i18n.Msg("SvcMgr.StopButton"), i18n.Msg("vtui.Cancel")}, vtui.MessageWarn)
	if confirm == nil {
		return
	}
	confirm.OnResult = func(code int) {
		if code == 0 {
			p.run(p.ctl.Stop)
		}
	}
}

// pauseOrResume pauses a running service and resumes a paused one; in any
// other state it says so instead of sending a control the service would
// refuse.
func (p *servicesPanel) pauseOrResume() {
	svc, ok := p.selectedService()
	if !ok {
		return
	}
	switch svc.State {
	case stateRunning:
		p.run(p.ctl.Pause)
	case statePaused:
		p.run(p.ctl.Resume)
	default:
		toast.Show(fmt.Sprintf(i18n.Msg("SvcMgr.CannotPause"), svc.Name, stateName(svc.State)), 3e9)
	}
}

func (p *servicesPanel) SetContext(vfs.PanelContext) {}

func (p *servicesPanel) Show(scr *vtui.ScreenBuf) {
	p.frame.SetTitle(fmt.Sprintf(i18n.Msg("SvcMgr.PanelTitle"), p.table.ItemCount))
	p.frame.Show(scr)
	p.table.Show(scr)
}

func (p *servicesPanel) Close() error { return nil }
