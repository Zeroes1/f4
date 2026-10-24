package panel

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// DriveMenuFrame adds the Far-style operation hint to a drive menu while
// keeping the embedded VMenu available to the menu's existing callers.
type DriveMenuFrame struct {
	*vtui.VMenu
	bottomHint string
}

func (f *DriveMenuFrame) Show(scr *vtui.ScreenBuf) {
	f.VMenu.Show(scr)
	if f.bottomHint == "" {
		return
	}
	x1, _, x2, y2 := f.GetPosition()
	vtui.NewPainter(scr).DrawTitle(x1, y2, x2, f.bottomHint, vtui.Palette[vtui.ColMenuTitle])
}

func (f *DriveMenuFrame) ProcessKey(e *vtinput.InputEvent) bool {
	handled := f.VMenu.ProcessKey(e)
	// VMenu checks whether it is the exact top frame before reporting that
	// Esc/F10 was consumed. The wrapper is the top frame in this menu, so
	// finish that part of the contract here.
	if e != nil && e.KeyDown && f.IsDone() &&
		(e.VirtualKeyCode == vtinput.VK_ESCAPE || e.VirtualKeyCode == vtinput.VK_F10) {
		return true
	}
	return handled
}

// driveLinkHotkeyEdit keeps the caret on the captured character, rather than
// scrolling it out of a one-cell field to display the insertion point after it.
// Existing multi-character bindings remain intact until explicitly replaced.
type driveLinkHotkeyEdit struct{ *vtui.Edit }

func (e *driveLinkHotkeyEdit) Show(scr *vtui.ScreenBuf) {
	e.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_HOME})
	e.Edit.Show(scr)
}

type driveBookmarkEditDialog struct {
	*vtui.Window
	nameEdit   *vtui.Edit
	pathEdit   *vtui.Edit
	HotkeyEdit *driveLinkHotkeyEdit
	globalEdit *GlobalHotkeyEdit
	// others are the links besides the one edited: a global hot key is
	// checked against theirs.
	others []DriveBookmark
	// hotkeys is the Hotkey Configurator's draft of the hotkey table when
	// the configurator opened the dialog (EditDriveLinkIn): the key is
	// checked against its commands, and one the key is taken from gives it
	// up there. Nil is the live table, which is written down at once.
	hotkeys *keymap.HotkeyManager
	// takenKey is the global hot key the user agreed to take from what had
	// it, or that nothing had.
	takenKey string
	finished bool
	onFinish func(bool, DriveBookmark)
}

// driveLinkGlobalFieldWidth is the least width of the global hot key field:
// Ctrl+Alt+Shift+F11 fits.
const driveLinkGlobalFieldWidth = 18

// driveLinkColumnGap is the least room between the menu hot key, with its
// hint, and the global hot key, with its own.
const driveLinkColumnGap = 3

// elementWidth is how many columns el takes.
func elementWidth(el vtui.UIElement) int {
	x1, _, x2, _ := el.GetPosition()
	return x2 - x1 + 1
}

// driveBookmarkDialogMinWidth is the floor for the dialog's computed width:
// half the program window would otherwise squeeze the fields on a narrow
// terminal.
const driveBookmarkDialogMinWidth = 64

// driveBookmarkDialogWidth returns the add/edit link dialog's width for the
// given screen width: half the program window, the same proportion the file
// dialogs use (see FileDialogWidth, #891), clamped to a sane minimum and to
// the screen itself. The dialog's fields are laid out once at construction
// time and never reflow afterwards, so f4#1148 asked to size it from the
// screen instead of letting the user stretch it and leave the fields behind.
func driveBookmarkDialogWidth(screenWidth int) int {
	if screenWidth <= 0 {
		return driveBookmarkDialogMinWidth
	}
	w := screenWidth / 2
	if w < driveBookmarkDialogMinWidth {
		w = driveBookmarkDialogMinWidth
	}
	if w > screenWidth {
		w = screenWidth
	}
	return w
}

func NewDriveBookmarkEditDialog(initial DriveBookmark, defaultPath string, onFinish func(bool, DriveBookmark)) *driveBookmarkEditDialog {
	// Six rows of fields, the rule and the buttons; the only blank row is the
	// one between the path and the hot key, none under the top border or
	// above the bottom one (mitek1977-coder, f4#1148).
	const height = 11
	screenW := 0
	if vtui.FrameManager != nil {
		screenW = vtui.FrameManager.GetScreenSize()
	}
	// The hot key row holds two columns (f4#2171): the menu letter with its
	// hint on the left, the global hot key with its own on the right. The
	// dialog grows to fit both.
	// The hint of the menu hot key is shorter than the plugin menu's, which
	// shares DriveLink.HotkeyHint: it sits beside the field, next to the hint
	// of the global hot key.
	hotkeyText, hintText := i18n.Msg("DriveLink.Hotkey"), i18n.Msg("DriveLink.HotkeyHintShort")
	globalText, globalHintText := i18n.Msg("DriveLink.GlobalHotkey"), i18n.Msg("DriveLink.GlobalHotkeyHint")
	leftWidth := max(vtui.StringWidth(hotkeyText)+2, vtui.StringWidth(hintText))
	rightWidth := max(vtui.StringWidth(globalText)+1+driveLinkGlobalFieldWidth, vtui.StringWidth(globalHintText))
	width := max(driveBookmarkDialogWidth(screenW), leftWidth+driveLinkColumnGap+rightWidth+4)
	if screenW > 0 && width > screenW {
		width = screenW
	}
	inner := width - 4
	// The right column starts half way, or further left when it would not
	// fit there, and never over the left one.
	rightX := max(leftWidth+driveLinkColumnGap, min(inner/2, inner-rightWidth))
	globalWidth := max(1, inner-rightX-vtui.StringWidth(globalText)-1)

	titleKey := "DriveLink.CreateTitle"
	if initial.Name != "" || initial.Path != "" {
		titleKey = "DriveLink.EditTitle"
	}
	dialog := &driveBookmarkEditDialog{
		Window:     vtui.NewCenteredDialog(width, height, i18n.Msg(titleKey)),
		nameEdit:   vtui.NewEdit(0, 0, width-6, initial.Name),
		pathEdit:   vtui.NewEdit(0, 0, width-6, initial.Path),
		HotkeyEdit: &driveLinkHotkeyEdit{vtui.NewEdit(0, 0, max(1, vtui.StringWidth(initial.Hotkey)), initial.Hotkey)},
		globalEdit: NewGlobalHotkeyEdit(initial.GlobalHotkey, globalWidth),
		onFinish:   onFinish,
	}
	dialog.globalEdit.OnKey = dialog.offerGlobalKey
	if dialog.pathEdit.GetText() == "" {
		dialog.pathEdit.SetText(defaultPath)
	}

	nameLabel := vtui.NewLabel(0, 0, i18n.Msg("DriveLink.Name"), dialog.nameEdit)
	pathLabel := vtui.NewLabel(0, 0, i18n.Msg("DriveLink.Path"), dialog.pathEdit)
	hotkeyLabel := vtui.NewLabel(0, 0, hotkeyText, dialog.HotkeyEdit)
	hotkeyHint := vtui.NewText(0, 0, hintText, vtui.Palette[vtui.ColDialogText])
	globalLabel := vtui.NewLabel(0, 0, globalText, dialog.globalEdit)
	globalHint := vtui.NewText(0, 0, globalHintText, vtui.Palette[vtui.ColDialogText])
	createText := i18n.Msg("DriveLink.Create")
	if initial.Name != "" || initial.Path != "" {
		createText = i18n.Msg("DriveLink.Save")
	}
	createButton := vtui.NewButton(0, 0, createText)
	createButton.IsDefault = true
	cancelButton := vtui.NewButton(0, 0, i18n.Msg("DriveLink.Cancel"))
	createButton.OnClick = func() { dialog.submit() }
	cancelButton.OnClick = func() { dialog.finish(false, DriveBookmark{}) }
	separator := vtui.NewSeparator(0, 0, width, true, true)

	dialog.AddItem(nameLabel)
	dialog.AddItem(dialog.nameEdit)
	dialog.AddItem(pathLabel)
	dialog.AddItem(dialog.pathEdit)
	dialog.AddItem(hotkeyLabel)
	dialog.AddItem(dialog.HotkeyEdit)
	dialog.AddItem(globalLabel)
	dialog.AddItem(dialog.globalEdit)
	dialog.AddItem(hotkeyHint)
	dialog.AddItem(globalHint)
	dialog.AddItem(separator)
	dialog.AddItem(createButton)
	dialog.AddItem(cancelButton)

	vbox := vtui.NewVBoxLayout(dialog.X1+2, dialog.Y1+1, width-4, height-2)
	vbox.Add(nameLabel, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(dialog.nameEdit, vtui.Margins{}, vtui.AlignFill)
	vbox.Add(pathLabel, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(dialog.pathEdit, vtui.Margins{}, vtui.AlignFill)
	hotkeyRow := vtui.NewHBoxLayout(0, 0, inner, 1)
	hotkeyRow.Spacing = 0
	hotkeyRow.Add(hotkeyLabel, vtui.Margins{}, vtui.AlignLeft)
	hotkeyRow.Add(dialog.HotkeyEdit, vtui.Margins{Left: 1}, vtui.AlignLeft)
	hotkeyRow.Add(globalLabel, vtui.Margins{Left: max(1, rightX-elementWidth(hotkeyLabel)-1-elementWidth(dialog.HotkeyEdit))}, vtui.AlignLeft)
	hotkeyRow.Add(dialog.globalEdit, vtui.Margins{Left: 1}, vtui.AlignLeft)
	vbox.Add(hotkeyRow, vtui.Margins{Top: 1}, vtui.AlignFill)
	// The hint of the global hot key ends under the end of its field.
	hintRow := vtui.NewHBoxLayout(0, 0, inner, 1)
	hintRow.Spacing = 0
	hintRow.Add(hotkeyHint, vtui.Margins{}, vtui.AlignLeft)
	hintRow.Add(globalHint, vtui.Margins{Left: max(1, inner-elementWidth(hotkeyHint)-elementWidth(globalHint))}, vtui.AlignLeft)
	vbox.Add(hintRow, vtui.Margins{}, vtui.AlignFill)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Spacing = 2
	buttons.Add(createButton, vtui.Margins{}, vtui.AlignTop)
	buttons.Add(cancelButton, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(separator, vtui.Margins{Left: -2, Right: -2}, vtui.AlignFill)
	vbox.Add(buttons, vtui.Margins{}, vtui.AlignFill)
	vbox.Apply()

	dialog.SetFocusedItem(dialog.nameEdit)
	dialog.OnResult = func(code int) {
		if !dialog.finished {
			dialog.finished = true
			if dialog.onFinish != nil {
				dialog.onFinish(false, DriveBookmark{})
			}
		}
	}
	return dialog
}

func (d *driveBookmarkEditDialog) finish(saved bool, bookmark DriveBookmark) {
	if d.finished {
		return
	}
	d.finished = true
	d.SetExitCode(1)
	if d.onFinish != nil {
		d.onFinish(saved, bookmark)
	}
}

func (d *driveBookmarkEditDialog) submit() {
	bookmark := DriveBookmark{
		Name:         strings.TrimSpace(d.nameEdit.GetText()),
		Path:         strings.TrimSpace(d.pathEdit.GetText()),
		Hotkey:       strings.TrimSpace(d.HotkeyEdit.GetText()),
		GlobalHotkey: d.globalEdit.Key(),
	}
	if bookmark.Name == "" {
		vtui.ShowMessageOn(d, i18n.Msg("DriveLink.ErrorTitle"), i18n.Msg("DriveLink.NameRequired"), []string{"&Ok"})
		d.SetFocusedItem(d.nameEdit)
		return
	}
	if bookmark.Path == "" {
		vtui.ShowMessageOn(d, i18n.Msg("DriveLink.ErrorTitle"), i18n.Msg("DriveLink.PathRequired"), []string{"&Ok"})
		d.SetFocusedItem(d.pathEdit)
		return
	}
	if key := bookmark.GlobalHotkey; key != "" {
		if !DriveLinkGlobalKeyAllowed(key) {
			d.refuseGlobalKey(fmt.Sprintf(i18n.Msg("DriveLink.GlobalHotkeyNeedsModifier"), keymap.FormatKeyForUI(key)))
			return
		}
		// A key the link already had, or one read from the file, was never
		// checked in this dialog: something may have been bound to it since.
		if !strings.EqualFold(key, d.takenKey) {
			d.checkGlobalKey(key, d.submit)
			return
		}
		for _, owner := range d.keyOwners(key) {
			if owner.Release == nil {
				continue
			}
			if err := owner.Release(); err != nil {
				vtui.ShowMessageOn(d, i18n.Msg("DriveLink.ErrorTitle"), fmt.Sprintf(i18n.Msg("DriveLink.SaveError"), err), []string{"&Ok"})
				return
			}
		}
	}
	d.finish(true, bookmark)
}

// offerGlobalKey is what a key pressed in the global hot key field does
// (f4#2171): Delete clears the field; a key that may be a global hot key and
// that nothing else has, or that the user agrees to take, goes into it.
func (d *driveBookmarkEditDialog) offerGlobalKey(key string) {
	if key == "" {
		d.globalEdit.SetKey("")
		return
	}
	if !DriveLinkGlobalKeyAllowed(key) {
		d.refuseGlobalKey(fmt.Sprintf(i18n.Msg("DriveLink.GlobalHotkeyNeedsModifier"), keymap.FormatKeyForUI(key)))
		return
	}
	d.checkGlobalKey(key, func() { d.globalEdit.SetKey(key) })
}

// checkGlobalKey lists what key would be taken from and runs take once the
// user agrees, at once when nothing has the key. A key the link could never
// take is refused; "Choose another" leaves the field as it was.
func (d *driveBookmarkEditDialog) checkGlobalKey(key string, take func()) {
	owners := d.keyOwners(key)
	var fixed, all []string
	for _, owner := range owners {
		if owner.Fixed {
			fixed = append(fixed, "  "+owner.Name)
		}
		all = append(all, "  "+owner.Name)
	}
	shown := keymap.FormatKeyForUI(key)
	if len(fixed) > 0 {
		d.refuseGlobalKey(fmt.Sprintf(i18n.Msg("DriveLink.GlobalHotkeyFixed"), shown, strings.Join(fixed, "\n")))
		return
	}
	if len(owners) == 0 {
		d.takenKey = key
		take()
		return
	}
	question := fmt.Sprintf(i18n.Msg("DriveLink.GlobalHotkeyBusy"), shown, strings.Join(all, "\n"))
	buttons := []string{i18n.Msg("DriveLink.Take"), i18n.Msg("DriveLink.ChooseAnother")}
	vtui.ShowMessageOn(d, i18n.Msg("DriveLink.GlobalHotkeyTitle"), question, buttons).OnResult = func(choice int) {
		if choice != 0 {
			d.SetFocusedItem(d.globalEdit)
			return
		}
		d.takenKey = key
		take()
	}
}

// keyOwners lists what key would be taken from, in the hotkey table the
// dialog edits (hotkeys).
func (d *driveBookmarkEditDialog) keyOwners(key string) []DriveLinkKeyOwner {
	if d.hotkeys != nil {
		return driveLinkKeyOwnersIn(d.hotkeys, key, d.others)
	}
	return driveLinkKeyOwners(key, d.others)
}

func (d *driveBookmarkEditDialog) refuseGlobalKey(message string) {
	vtui.ShowMessageOn(d, i18n.Msg("DriveLink.GlobalHotkeyTitle"), message, []string{"&Ok"})
	d.SetFocusedItem(d.globalEdit)
}

// ProcessMouse blocks the corner-drag resize gesture vtui.Window otherwise
// offers on every modal dialog. The dialog's width already tracks half the
// program window (f4#1148); the fields and buttons are laid out once at
// construction and never reflow on resize, so letting the user stretch the
// dialog further just left them behind, off to one side.
func (d *driveBookmarkEditDialog) ProcessMouse(e *vtinput.InputEvent) bool {
	if e != nil && e.ButtonState == vtinput.FromLeft1stButtonPressed && e.KeyDown &&
		int(e.MouseX) == d.X2 && int(e.MouseY) == d.Y2 {
		return true
	}
	return d.Window.ProcessMouse(e)
}

func (d *driveBookmarkEditDialog) ProcessKey(e *vtinput.InputEvent) bool {
	// The global hot key field sees its keys before the dialog: Alt+N there
	// is a hot key, not the jump to Name. What it leaves (Tab, Enter, Esc)
	// the dialog does as usual.
	if e != nil && (e.Type == vtinput.KeyEventType || e.Type == vtinput.PasteEventType) &&
		d.GetFocusedItem() == d.globalEdit && d.globalEdit.ProcessKey(e) {
		return true
	}
	if e != nil && e.KeyDown && d.GetFocusedItem() == d.HotkeyEdit {
		if e.VirtualKeyCode == vtinput.VK_TAB || e.VirtualKeyCode == vtinput.VK_ESCAPE || e.VirtualKeyCode == vtinput.VK_RETURN {
			return d.Window.ProcessKey(e)
		}
		if e.VirtualKeyCode == vtinput.VK_DELETE {
			d.HotkeyEdit.SetText("")
			return true
		}
		key := keymap.EventToHotkeyString(e)
		if len([]rune(key)) == 1 {
			d.HotkeyEdit.SetText(key)
		}
		return true
	}
	return d.Window.ProcessKey(e)
}

// driveBookmarkDefaultPath is the path a new link offers: the address of the
// panel's location, as a folder bookmark would store it (bookmarkPathOf).
func (pf *PanelsFrame) driveBookmarkDefaultPath(panelIdx int) string {
	if panelIdx < 0 || panelIdx >= len(pf.Panels) {
		return ""
	}
	fsp, ok := pf.Panels[panelIdx].(*FileSystemPanel)
	if !ok || fsp.Vfs == nil {
		return ""
	}
	return bookmarkPathOf(fsp)
}

func (pf *PanelsFrame) openDriveBookmarkEditor(panelIdx int, menu *vtui.VMenu, bookmarks []DriveBookmark, index int, reopen func()) {
	var initial DriveBookmark
	if index >= 0 && index < len(bookmarks) {
		initial = bookmarks[index]
	}
	if menu != nil {
		menu.Close()
	}
	var others []DriveBookmark
	for i, bookmark := range bookmarks {
		if i != index {
			others = append(others, bookmark)
		}
	}
	vtui.FrameManager.PostTask(func() {
		dialog := NewDriveBookmarkEditDialog(initial, pf.driveBookmarkDefaultPath(panelIdx), func(saved bool, bookmark DriveBookmark) {
			if !saved {
				vtui.FrameManager.PostTask(reopen)
				return
			}
			current, err := LoadDriveBookmarks(DriveBookmarksFilePath())
			if err == nil {
				at := index
				if at >= 0 && at < len(current) {
					current[at] = bookmark
				} else {
					at = len(current)
					current = append(current, bookmark)
				}
				current = takeDriveLinkGlobalKey(current, at)
				err = SaveDriveBookmarks(DriveBookmarksFilePath(), current)
			}
			if err != nil {
				message := vtui.ShowMessage(i18n.Msg("DriveLink.ErrorTitle"), fmt.Sprintf(i18n.Msg("DriveLink.SaveError"), err), []string{"&Ok"})
				message.OnResult = func(int) { vtui.FrameManager.PostTask(reopen) }
				return
			}
			vtui.FrameManager.PostTask(reopen)
		})
		dialog.others = others
		vtui.FrameManager.Push(dialog)
	})
}

// takeDriveLinkGlobalKey clears the global hot key of every link that shares
// it with the link at index, which the user has just agreed to give it to
// (f4#2171). The list is changed in place and returned.
func takeDriveLinkGlobalKey(links []DriveBookmark, index int) []DriveBookmark {
	if index < 0 || index >= len(links) {
		return links
	}
	key := strings.TrimSpace(links[index].GlobalHotkey)
	if key == "" {
		return links
	}
	for i := range links {
		if i != index && strings.EqualFold(strings.TrimSpace(links[i].GlobalHotkey), key) {
			links[i].GlobalHotkey = ""
		}
	}
	return links
}

func (pf *PanelsFrame) deleteDriveBookmark(menu *vtui.VMenu, bookmarks []DriveBookmark, index int, reopen func()) {
	if index < 0 || index >= len(bookmarks) {
		return
	}
	question := fmt.Sprintf(i18n.Msg("DriveLink.DeleteQuestion"), bookmarks[index].Name)
	vtui.ShowMessageOn(menu, i18n.Msg("DriveLink.DeleteTitle"), question, []string{"&Delete", i18n.Msg("DriveLink.Cancel")}).OnResult = func(choice int) {
		if choice != 0 {
			return
		}
		current, err := LoadDriveBookmarks(DriveBookmarksFilePath())
		if err == nil && index >= 0 && index < len(current) {
			current = append(current[:index], current[index+1:]...)
			err = SaveDriveBookmarks(DriveBookmarksFilePath(), current)
		}
		if err != nil {
			message := vtui.ShowMessage(i18n.Msg("DriveLink.ErrorTitle"), fmt.Sprintf(i18n.Msg("DriveLink.SaveError"), err), []string{"&Ok"})
			message.OnResult = func(int) { menu.Close(); vtui.FrameManager.PostTask(reopen) }
			return
		}
		menu.Close()
		vtui.FrameManager.PostTask(reopen)
	}
}

// moveDriveBookmark returns the list with the link at index moved by delta
// places (-1 up, 1 down), and whether anything moved: the first link cannot go
// up nor the last one down. The list itself is left as it was.
func moveDriveBookmark(bookmarks []DriveBookmark, index, delta int) ([]DriveBookmark, bool) {
	target := index + delta
	if index < 0 || index >= len(bookmarks) || target < 0 || target >= len(bookmarks) || delta == 0 {
		return bookmarks, false
	}
	moved := append([]DriveBookmark(nil), bookmarks...)
	moved[index], moved[target] = moved[target], moved[index]
	return moved, true
}

// moveBookmarkSlotInFile swaps a slot of the folder bookmarks with the nearest
// assigned slot in the direction of delta (the drive menu lists assigned slots
// only, so that is the row the user sees it pass) and saves the table. It
// reports false when there is no such slot.
func moveBookmarkSlotInFile(slot, delta int) (bool, error) {
	set, err := LoadBookmarks(BookmarksFilePath())
	if err != nil {
		return false, err
	}
	if slot < 0 || slot >= len(set) {
		return false, nil
	}
	step := 1
	if delta < 0 {
		step = -1
	}
	for next := slot + step; next >= 0 && next < len(set); next += step {
		if set[next].IsEmpty() {
			continue
		}
		set.SwapSlots(slot, next)
		return true, SaveBookmarks(BookmarksFilePath(), set)
	}
	return false, nil
}

// moveDriveBookmarkInFile moves one link of the saved list by delta places
// and saves the new order, so that it is remembered.
func moveDriveBookmarkInFile(index, delta int) (bool, error) {
	current, err := LoadDriveBookmarks(DriveBookmarksFilePath())
	if err != nil {
		return false, err
	}
	moved, ok := moveDriveBookmark(current, index, delta)
	if !ok {
		return false, nil
	}
	return true, SaveDriveBookmarks(DriveBookmarksFilePath(), moved)
}
