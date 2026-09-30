package panel

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtui"
)

// applyPluginHotkeyChoice is what OK does in the plugin hotkey dialog: text is
// what the one-character field holds. Empty takes the hotkey back (an assigned
// one, or the plugin's own default), a letter or digit becomes the menu hotkey
// of the entry, anything else is refused with ok false so that the dialog stays
// open. changed reports whether the bindings moved.
func applyPluginHotkeyChoice(hm *keymap.HotkeyManager, actionName, declaredKey, text string) (changed, ok bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		if area, key := keymap.ConfiguredHotkeyBinding(hm, actionName); key != "" {
			return keymap.DeletePluginHotkey(hm, area, key), true
		}
		if declaredKey != "" && !PluginDefaultKeyOff(declaredKey) {
			SetPluginDefaultKeyOff(declaredKey, true)
			return true, true
		}
		return false, true
	}
	runes := []rune(text)
	if len(runes) != 1 || (!unicode.IsLetter(runes[0]) && !unicode.IsDigit(runes[0])) {
		return false, false
	}
	// Choosing the letter the entry already has is not a change.
	if _, key := keymap.ConfiguredHotkeyBinding(hm, actionName); strings.EqualFold(key, text) {
		return false, true
	}
	return bindPluginMenuHotkey(hm, actionName, runes[0]), true
}

// currentPluginHotkeyText is what the one-character field starts with: the
// assigned letter, else the plugin's own default when it is a single character,
// else nothing.
func currentPluginHotkeyText(hm *keymap.HotkeyManager, actionName, declaredKey string) string {
	if _, key := keymap.ConfiguredHotkeyBinding(hm, actionName); key != "" {
		if r := pluginMenuHotkeyRune(key); r != 0 {
			return string(r)
		}
		return ""
	}
	if declaredKey != "" && !PluginDefaultKeyOff(declaredKey) {
		if r := pluginMenuHotkeyRune(declaredKey); r != 0 {
			return string(r)
		}
	}
	return ""
}

// showPluginHotkeyDialog asks for the menu hotkey of a plugin entry in a
// one-character field with OK and Cancel: the letter is shown, edited and
// deleted (empty field) like any other field, as the ticket asked (#918).
func showPluginHotkeyDialog(hm *keymap.HotkeyManager, actionName, label string, onComplete func()) {
	if hm == nil || vtui.FrameManager == nil {
		return
	}
	cleanLabel, _, _ := vtui.ParseAmpersandString(label)
	declaredKey := declaredHotkeyString(PluginActionDefaultShortcut(actionName))

	const width, height = 56, 10
	dlg := vtui.NewCenteredDialog(width, height, i18n.Msg("Plugins.HotkeyTitle"))
	dlg.ShowClose = true

	edit := vtui.NewEdit(0, 0, 3, currentPluginHotkeyText(hm, actionName, declaredKey))
	title := vtui.NewText(0, 0, cleanLabel, vtui.Palette[vtui.ColDialogText])
	prompt := vtui.NewLabel(0, 0, "&"+i18n.Msg("Plugins.HotkeyFieldLabel")+":", edit)
	note := vtui.NewText(0, 0, i18n.Msg("Plugins.HotkeyFieldNote"), vtui.Palette[vtui.ColDialogText])
	btnOk := vtui.NewButton(0, 0, i18n.Msg("vtui.Ok"))
	btnOk.IsDefault = true
	btnCancel := vtui.NewButton(0, 0, i18n.Msg("vtui.Cancel"))
	for _, it := range []vtui.UIElement{title, prompt, edit, note, btnOk, btnCancel} {
		dlg.AddItem(it)
	}

	vbox := vtui.NewVBoxLayout(dlg.X1+2, dlg.Y1+2, width-4, height-4)
	vbox.Add(title, vtui.Margins{}, vtui.AlignCenter)
	row := vtui.NewHBoxLayout(0, 0, width-4, 1)
	row.Add(prompt, vtui.Margins{Right: 1}, vtui.AlignLeft)
	row.Add(edit, vtui.Margins{}, vtui.AlignLeft)
	vbox.Add(row, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Add(note, vtui.Margins{Top: 1}, vtui.AlignCenter)
	buttons := vtui.NewHBoxLayout(0, 0, width-4, 1)
	buttons.HorizontalAlign = vtui.AlignCenter
	buttons.Add(btnOk, vtui.Margins{Right: 2}, vtui.AlignTop)
	buttons.Add(btnCancel, vtui.Margins{}, vtui.AlignTop)
	vbox.Add(buttons, vtui.Margins{Top: 1}, vtui.AlignFill)
	vbox.Apply()

	btnCancel.OnClick = func() { dlg.Close() }
	btnOk.OnClick = func() {
		changed, ok := applyPluginHotkeyChoice(hm, actionName, declaredKey, edit.GetText())
		if !ok {
			vtui.ShowMessageOn(dlg, i18n.Msg("Plugins.HotkeyTitle"), fmt.Sprint(i18n.Msg("Plugins.HotkeyFieldInvalid")), []string{i18n.Msg("vtui.Ok")})
			return
		}
		dlg.Close()
		if changed && onComplete != nil {
			onComplete()
		}
	}
	dlg.SetFocusedItem(edit)
	vtui.FrameManager.Push(dlg)
}
