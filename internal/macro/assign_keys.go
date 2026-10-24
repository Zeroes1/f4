package macro

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/keymap"
)

// macroKeyCandidates are the key combinations the assign dialog offers, in the
// order it lists them (Far's spelling, the one the key field shows): the
// function keys bare and with Shift, Ctrl and Alt, then Ctrl and Alt with the
// letters and the digits, and last the mouse wheel, which cannot be pressed in
// the key field. They are what a user reaches for first; any other key is
// still taken by pressing it in the key field (f4#1870).
func macroKeyCandidates() []string {
	var keys []string
	for _, mod := range []string{"", "Shift", "Ctrl", "Alt"} {
		for n := 1; n <= 12; n++ {
			keys = append(keys, fmt.Sprintf("%sF%d", mod, n))
		}
	}
	for _, mod := range []string{"Ctrl", "Alt"} {
		for c := 'A'; c <= 'Z'; c++ {
			keys = append(keys, mod+string(c))
		}
		for c := '0'; c <= '9'; c++ {
			keys = append(keys, mod+string(c))
		}
	}
	for _, mod := range []string{"", "Ctrl", "Alt", "Shift"} {
		keys = append(keys, mod+"MsWheelUp", mod+"MsWheelDown")
	}
	return keys
}

// FreeKeys lists the common key combinations (macroKeyCandidates) that
// neither a macro nor a command uses in area, so that a macro assigned to one
// of them takes nothing away. In use are the keys of the macros recorded in
// area and in Common (a Common macro works everywhere), those of the Lua
// macros, and the keys bound to commands in area and in Common by hm. A key
// the user unbound ("None") is free. hm may be nil.
func (m *MacroManager) FreeKeys(area string, hm *keymap.HotkeyManager) []string {
	areas := []string{area}
	if !strings.EqualFold(area, "Common") {
		areas = append(areas, "Common")
	}
	used := make(map[string]bool)
	var bindings map[string]map[string]string
	if hm != nil {
		bindings = hm.GetActiveBindings()
	}
	for _, a := range areas {
		for key := range m.Macros[a] {
			used[strings.ToLower(key)] = true
		}
		for key := range bindings[a] {
			used[strings.ToLower(key)] = true
		}
	}

	var free []string
	for _, key := range macroKeyCandidates() {
		// Alt+1..9 switch workspaces in the panels, independently of the
		// configurable hotkey table, so offering them as free macro keys is
		// misleading even when no command binding is present in hm.
		if isWorkspaceShortcut(key) || used[strings.ToLower(key)] || (m.Lua != nil && m.Lua.Find(area, key) != nil) {
			continue
		}
		free = append(free, key)
	}
	return free
}

// PlaysOn reports whether a macro plays on key in area: one recorded there or
// in Common, or a Lua macro. key is in the spelling of
// keymap.EventToHotkeyString. A recorded macro's key is compared in that
// spelling too, so Far's "Ctrl;" and the hotkey table's "CtrlVK_BA" are one
// key. The drive-menu link dialog asks it before a link takes a key
// (f4#2171).
func (m *MacroManager) PlaysOn(area, key string) bool {
	key = strings.TrimSpace(key)
	if m == nil || key == "" {
		return false
	}
	want := hotkeySpelling(key)
	areas := []string{area}
	if !strings.EqualFold(area, "Common") {
		areas = append(areas, "Common")
	}
	for _, a := range areas {
		for recorded := range m.Macros[a] {
			if strings.EqualFold(hotkeySpelling(recorded), want) {
				return true
			}
		}
	}
	if m.Lua != nil {
		// A Lua macro names its key as Far does: "CtrlAltT", "Ctrl.".
		for _, far := range []string{keymap.EventToFarString(keymap.ParseFarKey(key)), strings.ReplaceAll(keymap.FormatKeyForUI(key), "+", "")} {
			if m.Lua.Find(area, far) != nil {
				return true
			}
		}
	}
	return false
}

func hotkeySpelling(key string) string {
	return keymap.EventToHotkeyString(keymap.ParseFarKey(key))
}

func isWorkspaceShortcut(key string) bool {
	return len(key) == len("Alt0") && strings.HasPrefix(key, "Alt") && key[len("Alt")] >= '1' && key[len("Alt")] <= '9'
}
