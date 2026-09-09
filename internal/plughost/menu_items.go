package plughost

import (
	"sync"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

// PluginMenuItem is a row a plugin adds to the plugin menu through
// vfs.HostAPI.RegisterPluginMenuItem.
//
// The registry shares pluginRegistryMu with GlobalHotkeys: both are what
// plugins registered, and a plugin's Init can touch either while the menu is
// being built.
type PluginMenuItem struct {
	ActionName string
	Label      string
	Handler    func(app vfs.App)
}

var PluginMenuItems []PluginMenuItem

func RegisterPluginMenuItem(label string, handler func(app vfs.App)) {
	pluginRegistryMu.Lock()
	PluginMenuItems = append(PluginMenuItems, PluginMenuItem{
		ActionName: keymap.LegacyPluginActionName(len(PluginMenuItems)),
		Label:      label,
		Handler:    handler,
	})
	pluginRegistryMu.Unlock()
}

func PluginMenuItemsSnapshot() []PluginMenuItem {
	pluginRegistryMu.RLock()
	defer pluginRegistryMu.RUnlock()
	return append([]PluginMenuItem(nil), PluginMenuItems...)
}

var pluginRegistryMu sync.RWMutex

type HotkeyEntry struct {
	VK      uint16
	Mods    vtinput.ControlKeyState
	Handler func(app vfs.App)
}

var GlobalHotkeys []HotkeyEntry

func RegisterGlobalHotkey(vk uint16, mods vtinput.ControlKeyState, handler func(app vfs.App)) {
	pluginRegistryMu.Lock()
	GlobalHotkeys = append(GlobalHotkeys, HotkeyEntry{VK: vk, Mods: mods, Handler: handler})
	pluginRegistryMu.Unlock()
}

func GlobalHotkeysSnapshot() []HotkeyEntry {
	pluginRegistryMu.RLock()
	defer pluginRegistryMu.RUnlock()
	return append([]HotkeyEntry(nil), GlobalHotkeys...)
}

// SnapshotPluginRegistries copies both registries and returns the function that
// puts them back. A plugin registered by one test is visible to every later one
// until this is undone, and both registries share pluginRegistryMu because a
// plugin's Init can touch either while a menu is being built.
func SnapshotPluginRegistries() func() {
	pluginRegistryMu.Lock()
	hotkeys := append([]HotkeyEntry(nil), GlobalHotkeys...)
	items := append([]PluginMenuItem(nil), PluginMenuItems...)
	pluginRegistryMu.Unlock()
	return func() {
		pluginRegistryMu.Lock()
		GlobalHotkeys = hotkeys
		PluginMenuItems = items
		pluginRegistryMu.Unlock()
	}
}
