package panel

import (
	"sync"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// A lone Alt press -- Alt goes down and comes back up with nothing else in
// between -- opens and closes the panel autofilter (f4 #1131, the key Far's
// autofilter macro uses). It has to be told apart from every chord that
// starts with Alt: Alt+letter (the quick search), Alt+F-keys, Alt+arrows,
// Alt+click, Alt+Shift (a keyboard layout switch on many desktops). So the
// press only counts when no other key, mouse button or focus change arrived
// while Alt was down, and only when it was short: holding Alt to read the
// Alt row of the key bar and letting go must not open anything.
//
// The observer has to see every event, including the ones the hotkey
// dispatcher consumes before any frame does (Alt+F7 never reaches the
// panels), so the application calls LoneAltTap from its EventFilter rather
// than from PanelsFrame.ProcessKey.
//
// A lone Alt is visible only where the input source reports modifier keys on
// their own: the GUI backends, the Windows console, and terminals speaking
// win32-input-mode or the kitty keyboard protocol with all keys reported. A
// classic terminal sends nothing at all for it -- Alt+letter arrives there as
// ESC+letter -- which is what the Panel.AutoFilter action (Ctrl+Alt+F by
// default) is for.

// loneAltMaxHold is how long Alt may stay down and still count as a tap.
const loneAltMaxHold = 700 * time.Millisecond

type loneAltTracker struct {
	mu    sync.Mutex
	armed bool
	since time.Time
}

var loneAlt loneAltTracker

func isAltKey(vk uint16) bool {
	return vk == vtinput.VK_MENU || vk == vtinput.VK_LMENU || vk == vtinput.VK_RMENU
}

// observe feeds one input event to the tracker and reports whether it is the
// release that completes a lone Alt tap.
func (t *loneAltTracker) observe(e *vtinput.InputEvent, now time.Time) bool {
	if e == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	switch e.Type {
	case vtinput.KeyEventType:
		if !isAltKey(e.VirtualKeyCode) {
			t.armed = false
			return false
		}
		if e.KeyDown {
			// Ctrl+Alt is AltGr on Windows, Shift+Alt a layout switch:
			// neither is a lone Alt.
			if e.ControlKeyState&(vtinput.LeftCtrlPressed|vtinput.RightCtrlPressed|vtinput.ShiftPressed) != 0 {
				t.armed = false
				return false
			}
			// Autorepeat sends more presses while Alt is held; the hold
			// is timed from the first one.
			if !t.armed {
				t.armed = true
				t.since = now
			}
			return false
		}
		tapped := t.armed && now.Sub(t.since) <= loneAltMaxHold
		t.armed = false
		return tapped
	case vtinput.MouseEventType:
		// Pointer motion alone does not spoil the tap; a click or a wheel
		// turn with Alt down is a chord.
		if e.ButtonState != 0 || e.WheelDirection != 0 {
			t.armed = false
		}
		return false
	default:
		t.armed = false
		return false
	}
}

// LoneAltTap feeds an input event to the lone-Alt detector and reports
// whether it completes a lone Alt tap. The caller must pass every event, in
// order, before anything else may consume it.
func LoneAltTap(e *vtinput.InputEvent) bool {
	return loneAlt.observe(e, time.Now())
}

// HandleLoneAlt reacts to a lone Alt tap: with the autofilter enabled it
// opens the filter on the active panel, or closes it when it is open.
func (pf *PanelsFrame) HandleLoneAlt() bool {
	if !config.App.PanelAutoFilter {
		return false
	}
	// A raised menu bar owns the keyboard.
	if vtui.FrameManager != nil {
		if menu := vtui.FrameManager.GetActiveMenuBar(); menu != nil && menu.Active {
			return false
		}
	}
	return pf.ToggleAutoFilter()
}

// ToggleAutoFilter opens the autofilter on the active file panel, or closes
// it when it is open. It reports whether there was a panel to do it on.
func (pf *PanelsFrame) ToggleAutoFilter() bool {
	if pf == nil || !pf.ShowPanels {
		return false
	}
	// An info, quick-view, tree or player panel in the active slot has no
	// rows to filter.
	if pf.ActiveIdx >= 0 && pf.ActiveIdx < len(pf.AltPanels) && pf.AltPanels[pf.ActiveIdx] != nil {
		return false
	}
	fsp := pf.GetActivePanel()
	if fsp == nil {
		return false
	}
	// In search-first mode the typing has to go to the panel, not to the
	// command input, or the filter would open without receiving a key.
	if pf.SearchFirstMode() && pf.CommandLineFocused && !fsp.AutoFilterActive() {
		pf.SetCommandLineFocus(false)
	}
	fsp.ToggleAutoFilter()
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
	return true
}
