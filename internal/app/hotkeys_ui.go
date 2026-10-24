package app

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/panel"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type hotkeyRow struct {
	Action    string
	Label     string
	Area      string
	Key       string
	RawKey    string
	Editable  bool
	Condition string
	Desc      string
	// IsLink: the row is the global hot key of the drive-menu link at
	// LinkIndex in the configurator's links (f4#2171).
	IsLink    bool
	LinkIndex int
}

// hotkeyBuiltinMark marks, in the configurator's first, one-cell column, a
// built-in key: one a frame handles itself (an action's NativeKeys or
// NativeKeyAliases). It is listed so the table holds every key f4 reacts to,
// and it cannot be reassigned.
const hotkeyBuiltinMark = "•"

// hotkeyShowBuiltin says whether the configurator lists the built-in keys.
// The checkbox under its table turns them off for the rest of the session.
var hotkeyShowBuiltin = true

// builtIn reports whether the row is a built-in key; every other row is
// editable, a binding or the empty row that offers one.
func (r hotkeyRow) builtIn() bool { return !r.Editable }

// The columns of the hotkey table, in order (hotkeyColumnTitles).
const (
	hotkeyColMark = iota
	hotkeyColCommand
	hotkeyColKey
	hotkeyColArea
	hotkeyColWhen
	hotkeyColDescription
)

func (r hotkeyRow) GetCellText(col int) string {
	switch col {
	case hotkeyColMark:
		if r.builtIn() {
			return hotkeyBuiltinMark
		}
		return ""
	case hotkeyColCommand:
		return r.Label
	case hotkeyColKey:
		return r.Key
	case hotkeyColArea:
		return r.Area
	case hotkeyColWhen:
		return r.Condition
	case hotkeyColDescription:
		return r.Desc
	}
	return ""
}

// visibleHotkeyRows drops the built-in keys while they are turned off.
func visibleHotkeyRows(rows []hotkeyRow) []hotkeyRow {
	if hotkeyShowBuiltin {
		return rows
	}
	kept := make([]hotkeyRow, 0, len(rows))
	for _, row := range rows {
		if !row.builtIn() {
			kept = append(kept, row)
		}
	}
	return kept
}

// newHotkeyBuiltinCheckbox is the switch for the built-in keys, labelled
// with their mark.
func newHotkeyBuiltinCheckbox() *vtui.Checkbox {
	cb := vtui.NewCheckbox(0, 0, fmt.Sprintf(i18n.Msg("Hotkeys.ShowBuiltin"), hotkeyBuiltinMark), false)
	if hotkeyShowBuiltin {
		cb.State = 1
	}
	return cb
}

func (r hotkeyRow) GetCellAttr(col int, def uint64) uint64 {
	if r.Key == "" || !r.Editable {
		return vtui.DimColor(def)
	}
	return def
}

func hotkeyDialogSizeForScreen(screenWidth, screenHeight int) (int, int) {
	if screenWidth <= 0 {
		screenWidth = 120
	}
	if screenHeight <= 0 {
		screenHeight = 48
	}
	width := screenWidth - 4
	if width < 40 {
		width = 40
	}
	height := screenHeight - 2
	if height < 10 {
		height = 10
	}
	return width, height
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func fitHotkeyColumnWidths(widths []int, budget int) []int {
	if budget < 0 {
		budget = 0
	}
	for sumInts(widths) > budget {
		changed := false
		for i := range widths {
			if widths[i] <= 1 {
				continue
			}
			widths[i]--
			changed = true
			if sumInts(widths) <= budget {
				break
			}
		}
		if !changed {
			break
		}
	}
	return widths
}

// hotkeyColumnTitles names the hotkey table's columns in the order
// hotkeyTableColumns lays them out: the built-in mark, one cell wide, first;
// the last one, Description, is the only elastic one.
var hotkeyColumnTitles = []string{hotkeyBuiltinMark, "Command", "Key", "Area", "When", "Description"}

// hotkeyColumnFloors are the readable minimum widths fitHotkeyColumns settles
// the four balanced columns at once there is not enough room for everything
// (#1239): Key and Area stay legible (no more "Ctrl+Alt+Sh" or "Com"), while
// Command and When give up the rest of the room first. HotkeyTableMinDialogWidth
// reuses these same numbers, plus Description's own header width, to size the
// settings dialog's auto-maximize threshold instead of guessing one (#1239
// follow-up).
const (
	hotkeyCommandFloor = 10
	hotkeyKeyFloor     = 12
	hotkeyAreaFloor    = 6
	hotkeyWhenFloor    = 4
	// hotkeyTablePadding is the table's own side padding, reserved before its
	// columns are fitted to the available width.
	hotkeyTablePadding = 4
	// hotkeyDialogWidthOffset is added to the table's own page width by its
	// callers before it reaches hotkeyTableColumns's dialogWidth parameter.
	hotkeyDialogWidthOffset = 4
)

// fitHotkeyColumns squeezes the Command, Key, Area and When columns into
// budget cells. The chord and the area are short and useless once cut ("Ctrl+
// Alt+Sh", "Com"), while a command name or a condition still reads when
// shortened, so those two give way first, in two rounds, each down to a floor;
// only if that is not enough does everything shrink evenly (#1239).
func fitHotkeyColumns(widths []int, budget int) []int {
	// The built-in mark is one cell and never gives way.
	const (
		command = hotkeyColCommand
		key     = hotkeyColKey
		area    = hotkeyColArea
		when    = hotkeyColWhen
	)
	for _, step := range []struct{ col, floor int }{{when, 8}, {command, 16}, {when, hotkeyWhenFloor}, {command, hotkeyCommandFloor}, {area, hotkeyAreaFloor}, {key, hotkeyKeyFloor}} {
		if step.col >= len(widths) {
			continue
		}
		over := sumInts(widths) - budget
		if over <= 0 {
			return widths
		}
		if give := min(over, widths[step.col]-step.floor); give > 0 {
			widths[step.col] -= give
		}
	}
	return fitHotkeyColumnWidths(widths, budget)
}

// hotkeyDescriptionMinWidth is the least room the elastic Description column
// ever gets: its own header, whatever the dialog width (#1239).
func hotkeyDescriptionMinWidth() int {
	return vtui.StringWidth(hotkeyColumnTitles[len(hotkeyColumnTitles)-1])
}

// hotkeyDescriptionWidth is how much of dialogWidth the elastic Description
// column gets: a quarter of the dialog, from its own header's width up to 30
// cells (#1239).
func hotkeyDescriptionWidth(dialogWidth int) int {
	return min(max(dialogWidth/4, hotkeyDescriptionMinWidth()), 30)
}

func hotkeyTableColumns(rows []hotkeyRow, dialogWidth int) []vtui.TableColumn {
	titles := hotkeyColumnTitles
	widths := make([]int, len(titles))
	for i, title := range titles {
		widths[i] = vtui.StringWidth(title)
	}
	for _, row := range rows {
		for col := range widths {
			if cellWidth := vtui.StringWidth(row.GetCellText(col)); cellWidth > widths[col] {
				widths[col] = cellWidth
			}
		}
	}

	// The description column remains elastic. Reserve room for it, the four
	// column separators, and the table's side padding before fitting the other
	// columns to the actual dialog width. Its header alone is not enough room
	// to read anything (#1239): it gets a quarter of the dialog, from the
	// header's width up to 30 cells.
	descRoom := hotkeyDescriptionWidth(dialogWidth)
	fixedBudget := dialogWidth - hotkeyTablePadding - (len(widths) - 1) - descRoom
	fixed := fitHotkeyColumns(append([]int(nil), widths[:len(widths)-1]...), fixedBudget)
	columns := make([]vtui.TableColumn, 0, len(widths))
	for i, width := range fixed {
		columns = append(columns, vtui.TableColumn{Title: titles[i], Width: width})
	}
	columns = append(columns, vtui.TableColumn{Title: titles[len(titles)-1], Width: 0})
	return columns
}

// HotkeyTableMinDialogWidth is the narrowest dialogWidth (hotkeyTableColumns's
// own units) that keeps every fixed column at its readable minimum -- the
// built-in mark whole, Command, Key, Area and When at their floors above,
// Description at its header -- rather than shrinking one of them further. It
// is derived from those same constants, not measured, so it tracks them if
// they ever change (#1239 follow-up).
func HotkeyTableMinDialogWidth() int {
	floorSum := vtui.StringWidth(hotkeyBuiltinMark) + hotkeyCommandFloor + hotkeyKeyFloor + hotkeyAreaFloor + hotkeyWhenFloor
	separators := len(hotkeyColumnTitles) - 1
	for dialogWidth := floorSum + separators + hotkeyTablePadding + hotkeyDescriptionMinWidth(); ; dialogWidth++ {
		fixedBudget := dialogWidth - hotkeyTablePadding - separators - hotkeyDescriptionWidth(dialogWidth)
		if fixedBudget >= floorSum {
			return dialogWidth
		}
	}
}

// HotkeyTableMinPageWidth is the narrowest the embedded hotkey table's own
// page area (as internal/settings lays it out: page width = dialogWidth -
// hotkeyDialogWidthOffset) can be while its columns stay at their readable
// minimum (#1239 follow-up).
func HotkeyTableMinPageWidth() int {
	return HotkeyTableMinDialogWidth() - hotkeyDialogWidthOffset
}

func selectedHotkeyRowAt(table *vtui.Table, rows []hotkeyRow, displayPos int) (hotkeyRow, bool) {
	if table == nil {
		return hotkeyRow{}, false
	}
	idx := table.RowAt(displayPos)
	if idx < 0 || idx >= len(rows) {
		return hotkeyRow{}, false
	}
	return rows[idx], true
}

func normalizeHotkeySearchQuery(query string) string {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '+' {
			return -1
		}
		return unicode.ToLower(r)
	}, strings.TrimSpace(query))
	if compact == "" {
		return query
	}

	var raw strings.Builder
	remaining := compact
	for {
		matched := false
		for _, modifier := range []struct {
			name  string
			canon string
		}{
			{name: "rctrl", canon: "RCtrl"},
			{name: "ctrl", canon: "Ctrl"},
			{name: "alt", canon: "Alt"},
			{name: "shift", canon: "Shift"},
		} {
			if strings.HasPrefix(remaining, modifier.name) && len(remaining) > len(modifier.name) {
				raw.WriteString(modifier.canon)
				remaining = remaining[len(modifier.name):]
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	if raw.Len() == 0 || remaining == "" {
		return query
	}

	key := map[string]string{
		"up": "Up", "down": "Down", "left": "Left", "right": "Right",
		"home": "Home", "end": "End", "pgup": "PgUp", "pgdn": "PgDn",
		"ins": "Ins", "del": "Del", "enter": "Enter", "esc": "Esc",
		"tab": "Tab", "back": "Back", "space": "Space",
	}[remaining]
	if key == "" {
		if len(remaining) == 1 || (remaining[0] == 'f' && len(remaining) <= 3) {
			key = strings.ToUpper(remaining)
		} else {
			return query
		}
	}
	raw.WriteString(key)
	if strings.HasPrefix(raw.String(), "RCtrl") {
		return raw.String()
	}
	return keymap.FormatKeyForUI(raw.String())
}

func configureHotkeyTableSearch(table *vtui.Table) {
	table.QuickSearch = true
	table.SearchExactOnHit = config.App.SearchExactOnHit
	normalizing := false
	table.OnSearchChange = func(text string) {
		if normalizing {
			return
		}
		normalized := normalizeHotkeySearchQuery(text)
		if normalized == text {
			return
		}
		normalizing = true
		table.SetSearchText(normalized)
		normalizing = false
	}
}

func selectedHotkeyRow(table *vtui.Table, rows []hotkeyRow) (hotkeyRow, bool) {
	if table == nil {
		return hotkeyRow{}, false
	}
	return selectedHotkeyRowAt(table, rows, table.SelectPos)
}

// buildHotkeyRows assembles the shortcut inventory the hotkey settings dialog
// shows: one editable row per configurable binding, one read-only row per
// framework-owned native chord, one editable row for every action that has
// no configurable binding yet, and one editable row for every drive-menu link
// in links, with its global hot key or without one.
func buildHotkeyRows(draft *keymap.HotkeyManager, links []panel.DriveBookmark) []hotkeyRow {
	if draft == nil {
		return nil
	}

	var hkRows []hotkeyRow
	activeBinds := draft.GetActiveBindings()
	actions := action.AllSorted()
	// Only configurable bindings count as assigned here. Native chords are
	// documented below but deliberately stay out of this set.
	assignedActions := make(map[string]bool)

	for area, binds := range activeBinds {
		for key, binding := range binds {
			parts := strings.SplitN(binding, ":", 2)
			actName := parts[0]
			cond := ""
			if len(parts) == 2 {
				cond = parts[1]
			}
			act, ok := GetAction(actName)
			if !ok {
				act = action.Action{Name: actName, Label: actName, Description: "Unknown action"}
			}
			hkRows = append(hkRows, hotkeyRow{
				Action:    act.Name,
				Label:     action.PlainLabel(act.DisplayLabel()),
				Area:      area,
				Key:       keymap.FormatKeyForUI(key),
				RawKey:    key,
				Editable:  true,
				Condition: cond,
				Desc:      act.DisplayDescription(),
			})
			assignedActions[strings.ToLower(act.Name)] = true
		}
	}

	// Native shortcuts are handled by the focused frame rather than the
	// configurable hotkey manager. They still belong in this inventory so
	// the dialog describes every shortcut the user can press. Keep these
	// rows read-only: assigning them would create a misleading binding that
	// cannot replace the frame-owned behavior.
	//
	// They must not mark the action as assigned, though. An action whose only
	// shortcut is native -- Next Workspace on Ctrl+Tab -- would otherwise get
	// no editable row at all from the loop below, and both Assign and Enter
	// gate on hotkeyRow.Editable, so the dialog offered no way to give it a
	// second key. That is exactly the case a user hits when the host swallows
	// the native chord: Ctrl+Tab switches browser tabs when f4 runs in a
	// browser, and iTerm2 over ssh does not deliver it either (issue #72).
	// Dispatch already honors such a binding -- macro.MacroManager.Filter resolves
	// it through keymap.ConfiguredHotkeyAction and RunAction, with Common as the
	// fallback area -- so only this inventory stood in the way. The read-only
	// native row stays alongside the editable one.
	for _, act := range actions {
		seenNative := make(map[string]bool)
		// The aliases are listed too: the configurator shows every key f4
		// reacts to, the ones menus leave out included.
		natives := append(append([]string(nil), act.NativeKeys...), act.NativeKeyAliases...)
		for _, spec := range natives {
			key, cond, _ := strings.Cut(spec, ":")
			key = strings.TrimSpace(key)
			displayKey := keymap.FormatKeyForUI(key)
			if key == "" || seenNative[displayKey] {
				continue
			}
			if !act.NativeKeysFirst && draft.GetAction(act.Area, key) != "" {
				continue
			}
			seenNative[displayKey] = true
			hkRows = append(hkRows, hotkeyRow{
				Action:    act.Name,
				Label:     action.PlainLabel(act.DisplayLabel()),
				Area:      act.Area,
				Key:       displayKey,
				RawKey:    key,
				Condition: cond,
				Desc:      act.DisplayDescription(),
			})
		}
	}

	for _, act := range actions {
		if !assignedActions[strings.ToLower(act.Name)] {
			hkRows = append(hkRows, hotkeyRow{
				Action:    act.Name,
				Label:     action.PlainLabel(act.DisplayLabel()),
				Area:      act.Area, // unassigned, shown under the action's native area
				Key:       "",
				Editable:  true,
				Condition: "",
				Desc:      act.DisplayDescription(),
			})
		}
	}

	// Plugin menu commands live outside the built-in action registry, but they
	// use the same persisted binding format. Show every loaded command so a
	// shortcut can be prepared even while its context-sensitive menu item is
	// currently hidden.
	for _, act := range panel.PluginHotkeyActionsSnapshot() {
		if assignedActions[strings.ToLower(act.Name)] {
			continue
		}
		key := panel.PluginActionShortcut(act.Name)
		if key == "" {
			key = panel.PluginActionDefaultShortcut(act.Name)
		}
		rawKey := ""
		if configured := panel.PluginActionConfiguredKey(act.Name); configured != "" {
			rawKey = configured
		}
		hkRows = append(hkRows, hotkeyRow{
			Action:   act.Name,
			Label:    action.PlainLabel(act.DisplayLabel()),
			Area:     act.Area,
			Key:      key,
			RawKey:   rawKey,
			Editable: true,
			Desc:     act.DisplayDescription(),
		})
	}

	// A link's global hot key opens it from the panels (f4#2171): a key f4
	// reacts to, so it is a row as well. The link keeps it in the links
	// file, not in the hotkey table, and the link dialog changes it.
	for i, link := range links {
		if strings.TrimSpace(link.Name) == "" || strings.TrimSpace(link.Path) == "" {
			continue
		}
		key := strings.TrimSpace(link.GlobalHotkey)
		hkRows = append(hkRows, hotkeyRow{
			Label:     fmt.Sprintf(i18n.Msg("Hotkeys.LinkRow"), strings.TrimSpace(link.Name)),
			Area:      "Shell",
			Key:       keymap.FormatKeyForUI(key),
			RawKey:    key,
			Editable:  true,
			Desc:      fmt.Sprintf(i18n.Msg("Hotkeys.LinkDesc"), strings.TrimSpace(link.Path)),
			IsLink:    true,
			LinkIndex: i,
		})
	}

	sort.Slice(hkRows, func(i, j int) bool {
		if hkRows[i].Area != hkRows[j].Area {
			// Rows without an area (shouldn't happen) go last
			if hkRows[i].Area == "" {
				return false
			}
			if hkRows[j].Area == "" {
				return true
			}
			return hkRows[i].Area < hkRows[j].Area
		}
		if hkRows[i].Label != hkRows[j].Label {
			return hkRows[i].Label < hkRows[j].Label
		}
		// One action can now contribute both a native row and the empty
		// row that offers an extra key. Order that pair deterministically,
		// chord first, so the empty row reads as "and you may add one here"
		// instead of appearing above the shortcut it complements.
		return hkRows[i].Key > hkRows[j].Key
	})

	return hkRows
}

func actionHotkeyConfig(pf *panel.PanelsFrame) {
	w, h := 120, 48
	if vtui.FrameManager != nil {
		w, h = hotkeyDialogSizeForScreen(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
	}

	btnAssign := vtui.NewButton(0, 0, i18n.Msg("Hotkeys.BtnAssign"))
	btnUnbind := vtui.NewButton(0, 0, i18n.Msg("Hotkeys.BtnUnbind"))
	btnSave := vtui.NewButton(0, 0, i18n.Msg("vtui.Save"))
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	// Enter in the table assigns the selected row (table.OnAction), so Assign
	// is the button the default-button highlight must point at (#320), as
	// Go to is in the find-file results.
	btnAssign.IsDefault = true

	if keymap.GlobalHotkeysMgr == nil {
		return
	}
	original := keymap.GlobalHotkeysMgr
	draft := original.CloneForEdit()
	// The links' global hot keys are edited in a draft of the links file,
	// which Save writes with the hotkey table. A file that cannot be read
	// shows no links and is not written.
	links, err := panel.LoadDriveLinksDraft()
	if err != nil {
		vtui.DebugLog("HOTKEYS: drive links: %v", err)
	}

	dlg, table := vtui.NewTableDialog(w, h, i18n.Msg("Hotkeys.Title"), []vtui.TableColumn{
		{Title: hotkeyBuiltinMark, Width: 1},
		{Title: i18n.Msg("Hotkeys.ColCommand"), Width: 23},
		{Title: i18n.Msg("Hotkeys.ColKey"), Width: 14},
		{Title: i18n.Msg("Hotkeys.ColArea"), Width: 10},
		{Title: i18n.Msg("Hotkeys.ColWhen"), Width: 17},
		{Title: i18n.Msg("Hotkeys.ColDescription"), Width: 0},
	}, btnAssign, btnUnbind, btnSave, btnCancel)
	// The switch for the built-in keys takes the blank row between the table
	// and the buttons, and follows the bottom edge on resize. AddItem would
	// pin the window's minimum size at its position (vtui.NewTableWithButtons),
	// so the minimum the table dialog chose is put back.
	builtin := newHotkeyBuiltinCheckbox()
	tx1, _, _, ty2 := table.GetPosition()
	cbx1, _, cbx2, _ := builtin.GetPosition()
	builtin.SetPosition(tx1, ty2+1, tx1+cbx2-cbx1, ty2+1)
	builtin.SetGrowMode(vtui.GrowLoY | vtui.GrowHiY)
	minW, minH := dlg.MinW, dlg.MinH
	dlg.AddItem(builtin)
	dlg.MinW, dlg.MinH = max(minW, cbx2-cbx1+5), minH
	refresh := configureHotkeyEditor(dlg, table, btnAssign, btnUnbind, builtin, draft, links, nil, w)

	btnSave.OnClick = func() {
		original.ReplaceBindingsFrom(draft)
		original.Save()
		dlg.Close()
		if err := links.Save(); err != nil {
			vtui.ShowMessage(i18n.Msg("DriveLink.ErrorTitle"), fmt.Sprintf(i18n.Msg("DriveLink.SaveError"), err), []string{i18n.Msg("vtui.Ok")})
		}
	}

	btnCancel.OnClick = func() { dlg.Close() }

	vtui.FrameManager.Push(dlg)
	refresh()
}

// showAreaSelectDialog asks for the area and the condition of a key for
// actionName, then for the key itself (pushHotkeyAssign). links are the
// drive-menu links whose global hot keys the key is checked against; nil
// checks none.
func showAreaSelectDialog(hm *keymap.HotkeyManager, links panel.DriveLinkStore, actionName, defaultArea, defaultCond string, onComplete func()) {
	dlg := vtui.NewCenteredDialog(40, 11, i18n.Msg("Hotkeys.SelectTitle"))
	dlg.ShowClose = true

	if defaultArea == "" {
		defaultArea = "Shell"
	}

	areas := []string{"Shell", "Terminal", "Editor", "Viewer", "Dialog", "Menu", "Disks", "Common"}

	combo := vtui.NewComboBox(0, 0, 20, areas)
	combo.DropdownOnly = true
	idx := 0
	for i, a := range areas {
		if a == defaultArea {
			idx = i
		}
	}
	combo.Menu.SetSelectPos(idx)
	combo.Edit.SetText(areas[idx])

	conds := keymap.GetConditions()
	comboCond := vtui.NewComboBox(0, 0, 20, conds)
	comboCond.DropdownOnly = true
	cIdx := 0
	for i, c := range conds {
		if strings.EqualFold(c, defaultCond) || (c == "None" && defaultCond == "") {
			cIdx = i
		}
	}
	comboCond.Menu.SetSelectPos(cIdx)
	comboCond.Edit.SetText(conds[cIdx])

	btnOk := vtui.NewButton(0, 0, i18n.Msg("Hotkeys.BtnNext"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))

	lbl := vtui.NewLabel(0, 0, i18n.Msg("Hotkeys.LabelArea"), combo)
	lblCond := vtui.NewLabel(0, 0, i18n.Msg("Hotkeys.LabelWhen"), comboCond)

	dlg.AddItem(lbl)
	dlg.AddItem(combo)
	dlg.AddItem(lblCond)
	dlg.AddItem(comboCond)
	dlg.AddItem(btnOk)
	dlg.AddItem(btnCancel)

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, 36, 7)
	row := vtui.NewHBoxLayout(0, 0, 36, 1)
	row.Add(lbl, vtui.Margins{Right: 1}, vtui.AlignLeft)
	row.Add(combo, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(row, vtui.Margins{}, vtui.AlignFill)

	rowCond := vtui.NewHBoxLayout(0, 0, 36, 1)
	rowCond.Add(lblCond, vtui.Margins{Right: 1}, vtui.AlignLeft)
	rowCond.Add(comboCond, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(rowCond, vtui.Margins{Top: 1}, vtui.AlignFill)

	btns := vtui.NewHBoxLayout(0, 0, 36, 1)
	btns.HorizontalAlign = vtui.AlignCenter
	btns.Spacing = 2
	btns.Add(btnOk, vtui.Margins{}, vtui.AlignTop)
	btns.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(btns, vtui.Margins{Top: 1}, vtui.AlignFill)

	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		area := combo.Menu.Items[combo.Menu.SelectPos].Text
		cond := comboCond.Menu.Items[comboCond.Menu.SelectPos].Text
		if cond == "None" {
			cond = ""
		}

		fullName := actionName
		if cond != "" {
			fullName = actionName + ":" + cond
		}

		dlg.Close()
		vtui.FrameManager.PostTask(func() {
			pushHotkeyAssign(hm, links, fullName, area, onComplete)
		})
	}
	vtui.FrameManager.Push(dlg)
}

// pushHotkeyAssign opens the window a key for actionName in area is pressed
// in. In the panels the key router asks the drive-menu links before the
// hotkey table (f4#2171), so a key one of links has as its global hot key is
// not bound in Shell or Common at once: the user is asked whether to take it
// from the link or to choose another.
func pushHotkeyAssign(hm *keymap.HotkeyManager, links panel.DriveLinkStore, actionName, area string, onComplete func()) {
	frame := dialog.NewHotkeyAssignFrame(hm, actionName, area, onComplete)
	if links != nil {
		frame.Confirm = func(key string, bind func()) {
			current := links.DriveLinks()
			index, ok := panel.DriveLinkBeforeCommand(current, area, key)
			if !ok {
				bind()
				return
			}
			link := current[index]
			question := fmt.Sprintf(i18n.Msg("Hotkeys.LinkKeyBusy"), keymap.FormatKeyForUI(strings.TrimSpace(link.GlobalHotkey)), strings.TrimSpace(link.Name))
			buttons := []string{i18n.Msg("DriveLink.Take"), i18n.Msg("DriveLink.ChooseAnother")}
			vtui.ShowMessage(i18n.Msg("Hotkeys.AssignTitle"), question, buttons).OnResult = func(choice int) {
				switch choice {
				case 0:
				case 1:
					vtui.FrameManager.PostTask(func() { pushHotkeyAssign(hm, links, actionName, area, onComplete) })
					return
				default:
					// Esc gives the key up, as Esc in the key window does.
					return
				}
				link.GlobalHotkey = ""
				if err := links.SetDriveLink(index, link); err != nil {
					vtui.ShowMessage(i18n.Msg("DriveLink.ErrorTitle"), fmt.Sprintf(i18n.Msg("DriveLink.SaveError"), err), []string{i18n.Msg("vtui.Ok")})
					return
				}
				bind()
			}
		}
	}
	vtui.FrameManager.Push(frame)
}

// configureHotkeyEditor is shared by the legacy dialog and its embedded Settings tab.
// links are the drive-menu links whose global hot keys the editor lists and
// changes (f4#2171); nil lists none.
func configureHotkeyEditor(dlg *vtui.Window, table *vtui.Table, btnAssign, btnUnbind *vtui.Button, builtin *vtui.Checkbox, draft *keymap.HotkeyManager, links panel.DriveLinkStore, onChange func(*keymap.HotkeyManager), w int) func() {
	theme.UseTableColors(table)
	table.ShowScrollBar = true
	table.Sortable = true // click a column header to sort, again to reverse
	//table.QuickSearch = true // type to fuzzy-filter (Myers bit-vector)
	configureHotkeyTableSearch(table)

	var rows []vtui.TableRow
	var hkRows []hotkeyRow

	refresh := func() {
		rows = nil
		// hkRows holds exactly the rows the table shows: selectedHotkeyRow
		// maps a table position back into it.
		var linkList []panel.DriveBookmark
		if links != nil {
			linkList = links.DriveLinks()
		}
		hkRows = visibleHotkeyRows(buildHotkeyRows(draft, linkList))

		for _, r := range hkRows {
			rows = append(rows, r)
		}
		width := w
		if table.X2 > table.X1 {
			width = table.X2 - table.X1 + 5
		}
		table.Columns = hotkeyTableColumns(hkRows, width)
		table.SetRows(rows)
		if vtui.FrameManager != nil {
			vtui.FrameManager.Redraw()
		}
	}

	changed := func() {
		if onChange != nil {
			onChange(draft)
		}
		refresh()
	}
	reportLinkError := func(err error) {
		if err != nil {
			vtui.ShowMessageOn(dlg, i18n.Msg("DriveLink.ErrorTitle"), fmt.Sprintf(i18n.Msg("DriveLink.SaveError"), err), []string{i18n.Msg("vtui.Ok")})
		}
	}
	// assign gives the row a key: a command through the area and the key
	// windows, a link through the link dialog, which checks the key against
	// the draft (f4#2171). A command the link takes the key from gives it up
	// in the draft, so the change is reported for both.
	assign := func(row hotkeyRow) {
		if !row.Editable {
			return
		}
		if row.IsLink {
			if links == nil {
				return
			}
			panel.EditDriveLinkIn(draft, links.DriveLinks(), row.LinkIndex, func(link panel.DriveBookmark) {
				reportLinkError(panel.PutDriveLink(links, row.LinkIndex, link))
				changed()
			})
			return
		}
		showAreaSelectDialog(draft, links, row.Action, row.Area, row.Condition, changed)
	}
	btnAssign.OnClick = func() {
		if row, ok := selectedHotkeyRow(table, hkRows); ok {
			assign(row)
		}
	}

	btnUnbind.OnClick = func() {
		if row, ok := selectedHotkeyRow(table, hkRows); ok && row.Editable {
			if row.RawKey != "" && row.Area != "" {
				question := fmt.Sprintf("%s %s?", action.PlainLabel(i18n.Msg("Hotkeys.BtnUnbind")), row.Key)
				vtui.ShowMessageOn(dlg, i18n.Msg("Hotkeys.Title"), question, []string{i18n.Msg("vtui.Ok"), i18n.Msg("vtui.Cancel")}).OnResult = func(choice int) {
					if choice != 0 {
						return
					}
					if row.IsLink {
						if links == nil {
							return
						}
						current := links.DriveLinks()
						if row.LinkIndex >= len(current) {
							return
						}
						link := current[row.LinkIndex]
						link.GlobalHotkey = ""
						reportLinkError(links.SetDriveLink(row.LinkIndex, link))
						changed()
						return
					}
					draft.Bind(row.Area, row.RawKey, "None")
					changed()
				}
			}
		}
	}

	table.OnAction = func(idx int) {
		// idx is the table's display position. Use it directly: the table may
		// be sorted or QuickSearch-filtered, and SelectPos can be observed
		// after the dispatcher's state has moved on.
		if row, ok := selectedHotkeyRowAt(table, hkRows, idx); ok {
			assign(row)
		}
	}

	if builtin != nil {
		builtin.OnChange = func(state int) {
			hotkeyShowBuiltin = state == 1
			refresh()
		}
	}

	return refresh
}

type hotkeyPage struct {
	*vtui.Group
	owner          *vtui.Window
	table          *vtui.Table
	assign, unbind *vtui.Button
	builtin        *vtui.Checkbox
	// builtinWidth is the checkbox's own width, kept apart from its position,
	// which a narrow page cuts.
	builtinWidth int
	refresh      func()
}

// Refresh lists the rows again. The Settings window calls it when the page
// comes back into view: the Drive links page of the same window may have
// changed a link's global hot key meanwhile.
func (p *hotkeyPage) Refresh() {
	if p.refresh != nil {
		p.refresh()
	}
}

// hotkeyDetailsText is one row of the list in full. The table cuts every cell
// to the width of its column, and a long description or condition is lost that
// way (#1239).
func hotkeyDetailsText(row hotkeyRow) string {
	var lines []string
	for _, field := range []struct{ title, value string }{
		{i18n.Msg("Hotkeys.ColCommand"), row.Label},
		{i18n.Msg("Hotkeys.ColKey"), row.Key},
		{i18n.Msg("Hotkeys.ColArea"), row.Area},
		{i18n.Msg("Hotkeys.ColWhen"), row.Condition},
	} {
		if field.value != "" {
			lines = append(lines, field.title+": "+field.value)
		}
	}
	if row.Desc != "" {
		lines = append(lines, "", row.Desc)
	}
	if row.builtIn() && row.Key != "" {
		lines = append(lines, "", hotkeyBuiltinMark+" "+i18n.Msg("Hotkeys.BuiltinNote"))
	}
	return strings.Join(lines, "\n")
}

// rows returns the list's rows in the order the table holds them.
func (p *hotkeyPage) rows() []hotkeyRow {
	rows := make([]hotkeyRow, 0, len(p.table.Rows))
	for _, row := range p.table.Rows {
		if r, ok := row.(hotkeyRow); ok {
			rows = append(rows, r)
		}
	}
	return rows
}

// Keep vertical navigation inside the configurator, while Tab can still leave it.
func (p *hotkeyPage) ProcessKey(e *vtinput.InputEvent) bool {
	const modifiers = vtinput.ShiftPressed | vtinput.LeftCtrlPressed | vtinput.RightCtrlPressed | vtinput.LeftAltPressed | vtinput.RightAltPressed
	if e.KeyDown && e.VirtualKeyCode == vtinput.VK_F3 && e.ControlKeyState&modifiers == 0 {
		if row, ok := selectedHotkeyRow(p.table, p.rows()); ok {
			title, text, buttons := i18n.Msg("Hotkeys.Title"), hotkeyDetailsText(row), []string{i18n.Msg("vtui.Ok")}
			if p.owner != nil {
				vtui.ShowMessageOnEx(p.owner, title, text, buttons, vtui.MessageInfo)
			} else {
				vtui.ShowMessageEx(title, text, buttons, vtui.MessageInfo)
			}
			return true
		}
	}
	// The keys of the other big lists (user menu, histories, folder shortcuts):
	// F4 changes the selected entry, Ins adds to it, Del removes. The buttons
	// stay, but reaching them takes the focus off the list, and it is then not
	// clear which row they act on (#1239). While a search text is being typed
	// Del belongs to the search.
	if e.KeyDown && e.ControlKeyState&modifiers == 0 {
		switch e.VirtualKeyCode {
		case vtinput.VK_F4, vtinput.VK_INSERT:
			if p.assign.OnClick != nil {
				p.assign.OnClick()
				return true
			}
		case vtinput.VK_DELETE:
			if p.table.SearchText() == "" && p.unbind.OnClick != nil {
				p.unbind.OnClick()
				return true
			}
		}
	}
	previous := p.WrapFocus
	p.WrapFocus = e.KeyDown && (e.VirtualKeyCode == vtinput.VK_UP || e.VirtualKeyCode == vtinput.VK_DOWN)
	defer func() { p.WrapFocus = previous }()
	return p.Group.ProcessKey(e)
}

func (p *hotkeyPage) SetPosition(x1, y1, x2, y2 int) {
	p.Group.SetPosition(x1, y1, x2, y2)
	p.table.SetPosition(x1, y1, x2, y2-2)
	p.table.Columns = hotkeyTableColumns(p.rows(), x2-x1+1+hotkeyDialogWidthOffset)
	// The switch for the built-in keys sits right under the table, as in the
	// Hotkey Configurator dialog.
	p.builtin.SetPosition(x1, y2-1, min(x2, x1+p.builtinWidth-1), y2-1)
	x := x1
	for _, button := range []*vtui.Button{p.assign, p.unbind} {
		width := vtui.StringWidth(button.GetCaption()) + 4
		button.SetPosition(x, y2, min(x2, x+width-1), y2)
		x += width + 1
	}
}

// HotkeyTableMinPageWidth implements settings.HotkeyTableSizeHost: it lets
// the Settings dialog size its auto-maximize threshold for the Hotkey
// Configurator from the same column-width logic the embedded table itself
// uses, rather than a guessed terminal-width constant (#1239 follow-up).
func (settingsHost) HotkeyTableMinPageWidth() int {
	return HotkeyTableMinPageWidth()
}

func (settingsHost) HotkeyPage(owner *vtui.Window, onChange func(*keymap.HotkeyManager), links panel.DriveLinkStore) vtui.UIElement {
	p := &hotkeyPage{Group: vtui.NewGroup(0, 0, 40, 10), owner: owner}
	p.SetId("hotkey-configurator")
	p.table = vtui.NewTable(0, 0, 40, 7, hotkeyTableColumns(nil, 44))
	p.table.SetId("hotkey-table")
	p.assign = vtui.NewButton(0, 0, i18n.Msg("Hotkeys.BtnAssign"))
	p.assign.SetId("hotkey-assign")
	p.unbind = vtui.NewButton(0, 0, i18n.Msg("Hotkeys.BtnUnbind"))
	p.unbind.SetId("hotkey-unbind")
	p.builtin = newHotkeyBuiltinCheckbox()
	p.builtin.SetId("hotkey-builtin")
	cbx1, _, cbx2, _ := p.builtin.GetPosition()
	p.builtinWidth = cbx2 - cbx1 + 1
	// Tab follows the page top to bottom: the table, the switch under it,
	// the buttons.
	p.AddItem(p.table)
	p.AddItem(p.builtin)
	p.AddItem(p.assign)
	p.AddItem(p.unbind)
	if keymap.GlobalHotkeysMgr == nil {
		p.SetDisabled(true)
		return p
	}
	p.refresh = configureHotkeyEditor(owner, p.table, p.assign, p.unbind, p.builtin, keymap.GlobalHotkeysMgr.CloneForEdit(), links, onChange, 44)
	p.refresh()
	p.SetFocusedItem(p.table)
	return p
}
