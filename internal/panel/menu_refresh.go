package panel

import (
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtui"
)

// menuRefreshAge is how long the checkmarks and the letters of a menu bar that
// nobody is using stay as they were set. GetMenuBar is called several times for
// every key event and again for every frame, and setting them each time took
// most of the time of a long paste into the command line (unxed/f4#2157). The
// bar is set again on every call while it is up, so what the user opens is
// current; a quarter of a second of delay in a bar nobody looks at is not seen.
const menuRefreshAge = 250 * time.Millisecond

// menuRefresh remembers when the bar was last set, and under which language,
// hotkeys and keyboard setting.
type menuRefresh struct {
	at      time.Time
	i18nGen uint64
	hotGen  uint64
	macKeyb string
	done    bool
	first   *vtui.MenuBarItem // the first item of the bar that was set
}

func (m *menuRefresh) due(active bool, now time.Time) bool {
	if active || !m.done || now.Sub(m.at) > menuRefreshAge {
		return true
	}
	return m.i18nGen != i18n.Generation() || m.hotGen != hotkeysGeneration() || m.macKeyb != config.App.MacKeyboard
}

func (m *menuRefresh) set(now time.Time) {
	m.at = now
	m.i18nGen = i18n.Generation()
	m.hotGen = hotkeysGeneration()
	m.macKeyb = config.App.MacKeyboard
	m.done = true
}

func hotkeysGeneration() uint64 {
	if keymap.GlobalHotkeysMgr == nil {
		return 0
	}
	return keymap.GlobalHotkeysMgr.Generation()
}
