package editor

import (
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtui"
)

// menuRefreshAge is how long the items of a menu bar that nobody is using
// stay as they were built. The items bake in the checkmarks and the disabled
// state, so they are rebuilt on every call while the bar is up; while it is
// not, the calls come from the key router and the frame manager (several per
// key event) and building the whole menu each time made a paste of 2000
// lines take fifty seconds (unxed/f4#2157).
const menuRefreshAge = 250 * time.Millisecond

// menuRefresh remembers when the items of a menu bar were built, and under
// which language, hotkeys and keyboard setting.
type menuRefresh struct {
	at       time.Time
	i18nGen  uint64
	hotGen   uint64
	macKeyb  string
	hasItems bool
}

// due reports whether the items have to be built again: the bar is up (its
// dropdowns are drawn from them), they were never built, they are old, or
// what they are made of has changed.
func (m *menuRefresh) due(active bool, now time.Time) bool {
	if active || !m.hasItems || now.Sub(m.at) > menuRefreshAge {
		return true
	}
	return m.i18nGen != i18n.Generation() || m.hotGen != hotkeysGeneration() || m.macKeyb != config.App.MacKeyboard
}

// built records that the items were just built.
func (m *menuRefresh) built(now time.Time) {
	m.at = now
	m.i18nGen = i18n.Generation()
	m.hotGen = hotkeysGeneration()
	m.macKeyb = config.App.MacKeyboard
	m.hasItems = true
}

func hotkeysGeneration() uint64 {
	if keymap.GlobalHotkeysMgr == nil {
		return 0
	}
	return keymap.GlobalHotkeysMgr.Generation()
}

// keyLabelsCache keeps the last key bar captions and what they were made of.
type keyLabelsCache struct {
	set      *vtui.KeySet
	codepage int
	state    menuRefresh
}

// fresh reports whether the kept captions can be used again.
func (c *keyLabelsCache) fresh(codepage int, now time.Time) bool {
	return c.set != nil && c.codepage == codepage && !c.state.due(false, now)
}

func (c *keyLabelsCache) store(set *vtui.KeySet, codepage int, now time.Time) {
	c.set, c.codepage = set, codepage
	c.state.built(now)
}
