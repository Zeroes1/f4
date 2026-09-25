package panel

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtui"
)

// panelViewModeName is the menu name of the mode Ctrl+key selects.
func panelViewModeName(key int) string {
	if key < 0 || key >= PanelViewModeCount {
		return ""
	}
	return i18n.Msg(panelViewModeNameKeys[key])
}

var panelViewModeNameKeys = [PanelViewModeCount]string{
	"Panel.Modes.Mode0", "Panel.Modes.Mode1", "Panel.Modes.Mode2", "Panel.Modes.Mode3", "Panel.Modes.Mode4",
	"Panel.Modes.Mode5", "Panel.Modes.Mode6", "Panel.Modes.Mode7", "Panel.Modes.Mode8", "Panel.Modes.Mode9",
}

// panelModesMenuPos is the list row of a mode: far2l lists Ctrl+1 .. Ctrl+9
// first and Ctrl+0 last (flmodes.cpp, IndexToMenuPos).
func panelModesMenuPos(mode ViewMode) int {
	return (mode.Key() + PanelViewModeCount - 1) % PanelViewModeCount
}

func panelModesMenuKey(pos int) int {
	return (pos + 1) % PanelViewModeCount
}

// ShowPanelModesMenu is far2l's Options -> File panel modes
// (FileList::SetFilePanelModes): the ten modes, Enter edits the one under
// the cursor and returns to the list.
func ShowPanelModesMenu(pf *PanelsFrame) {
	pos := 0
	if pf != nil {
		if fsp := pf.GetActivePanel(); fsp != nil {
			pos = panelModesMenuPos(fsp.EffectiveViewMode())
		}
	}
	openPanelModesMenu(pf, pos)
}

func openPanelModesMenu(pf *PanelsFrame, pos int) {
	if vtui.FrameManager == nil {
		return
	}
	menu := vtui.NewVMenu(i18n.Msg("Panel.Modes.Title"))
	for p := 0; p < PanelViewModeCount; p++ {
		key := panelModesMenuKey(p)
		menu.AddItem(vtui.MenuItem{Text: panelViewModeName(key), Shortcut: "Ctrl+" + strconv.Itoa(key)})
	}
	menu.SetSelectPos(pos)
	menu.OnAction = func(idx int) {
		if idx < 0 || idx >= PanelViewModeCount {
			return
		}
		vtui.FrameManager.PostTask(func() { editPanelViewMode(pf, idx) })
	}
	screenW, screenH := vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight()
	w := min(46, screenW)
	h := min(PanelViewModeCount+2, screenH)
	x := max(0, (screenW-w)/2)
	y := max(0, (screenH-h)/2)
	menu.SetPosition(x, y, x+w-1, y+h-1)
	vtui.FrameManager.Push(menu)
}

// panelModeErrorText turns a TextToViewSettings error into the message the
// dialog shows.
func panelModeErrorText(err error) string {
	var typeErr *PanelColumnTypeError
	var widthErr *PanelColumnWidthError
	switch {
	case errors.As(err, &typeErr):
		return fmt.Sprintf(i18n.Msg("Panel.Modes.BadColumnType"), typeErr.Token)
	case errors.As(err, &widthErr):
		return fmt.Sprintf(i18n.Msg("Panel.Modes.BadColumnWidth"), widthErr.Token)
	case errors.Is(err, ErrNoPanelColumns):
		return i18n.Msg("Panel.Modes.NoColumns")
	}
	return err.Error()
}

// editPanelViewMode is far2l's mode dialog: column types and widths, the
// full screen switch, and Reset back to the built-in definition. Closing it
// returns to the list, as in far2l.
func editPanelViewMode(pf *PanelsFrame, pos int) {
	key := panelModesMenuKey(pos)
	mode, ok := ViewModeForKey(key)
	if !ok || vtui.FrameManager == nil {
		return
	}
	settings := PanelViewModeSettings(mode)
	types, widths := ViewSettingsToText(settings.Columns)

	const width = 64
	const height = 13
	dlg := vtui.NewCenteredDialog(width, height, " "+panelViewModeName(key)+" ")
	dlg.ShowClose = true

	editTypes := vtui.NewEdit(0, 0, width-6, types)
	editWidths := vtui.NewEdit(0, 0, width-6, widths)
	chkFullScreen := vtui.NewCheckbox(0, 0, i18n.Msg("Panel.Modes.FullScreen"), false)
	if settings.FullScreen {
		chkFullScreen.State = 1
	}
	btnOk := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnReset := vtui.NewButton(0, 0, i18n.Msg("Panel.Modes.Reset"))
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	lblTypes := vtui.NewLabel(0, 0, i18n.Msg("Panel.Modes.ColumnTypes"), editTypes)
	lblWidths := vtui.NewLabel(0, 0, i18n.Msg("Panel.Modes.ColumnWidths"), editWidths)

	dlg.AddItem(lblTypes)
	dlg.AddItem(editTypes)
	dlg.AddItem(lblWidths)
	dlg.AddItem(editWidths)
	dlg.AddItem(chkFullScreen)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnReset)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+3, dlg.Y1+2, width-6, height-4)
	vbox.Add(lblTypes, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(editTypes, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(lblWidths, vtui.Margins{Top: 1}, vtui.AlignLeft)
	vbox.Add(editWidths, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(chkFullScreen, vtui.Margins{Top: 1}, vtui.AlignLeft)
	btnRow := vtui.NewHBoxLayout(0, 0, width-6, 1)
	btnRow.HorizontalAlign = vtui.AlignCenter
	btnRow.Spacing = 2
	btnRow.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	btnRow.Add(btnReset, vtui.Margins{}, vtui.AlignTop)
	btnRow.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(btnRow, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	finish := func(changed *PanelViewSettings, reset bool) {
		if changed != nil || reset {
			if err := SetPanelViewModeSettings(mode, changed); err != nil {
				vtui.ShowMessageOn(dlg, " "+i18n.Msg("Panel.Modes.Title")+" ", err.Error(), []string{"&Ok"})
				return
			}
			applyPanelViewModes(pf)
		}
		dlg.Close()
		vtui.FrameManager.PostTask(func() { openPanelModesMenu(pf, pos) })
	}
	btnCancel.OnClick = func() { finish(nil, false) }
	btnReset.OnClick = func() { finish(nil, true) }
	btnOk.OnClick = func() {
		columns, err := TextToViewSettings(editTypes.GetText(), editWidths.GetText())
		if err != nil {
			vtui.ShowMessageOn(dlg, " "+i18n.Msg("Panel.Modes.Title")+" ", panelModeErrorText(err), []string{"&Ok"})
			return
		}
		changed := &PanelViewSettings{Columns: columns, FullScreen: chkFullScreen.State != 0}
		finish(changed, false)
	}

	dlg.SetFocusedItem(editTypes)
	vtui.FrameManager.Push(dlg)
}

// applyPanelViewModes lays the panels out again after a mode changed.
func applyPanelViewModes(pf *PanelsFrame) {
	if pf == nil {
		return
	}
	if pf.LastW > 0 && pf.LastH > 0 {
		pf.ResizeConsole(pf.LastW, pf.LastH)
	}
	pf.UpdateMenuCheckmarks()
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}
