package editor

import (
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// PluginsMenuFrame wraps the editor's F11 menu so the DialogOuterBorder
// setting can recognize it (app.RenderDialogOuterBorder). The menu is a
// vtui.VMenu like any other menu, whose GetType reports TypeMenu, so the
// wrapper f4 pushes it as is what tells it apart from the dropdown menus that
// never get the extra border. All Frame interface methods are promoted from
// the embedded VMenu; ProcessKey is overridden for the same reason the drive
// and the user menu wrappers override it: VMenu reports Esc and F10 as
// consumed only while it is the exact top frame, and here the wrapper is.
type PluginsMenuFrame struct {
	*vtui.VMenu
}

func (f *PluginsMenuFrame) ProcessKey(e *vtinput.InputEvent) bool {
	handled := f.VMenu.ProcessKey(e)
	if e != nil && e.KeyDown && f.IsDone() &&
		(e.VirtualKeyCode == vtinput.VK_ESCAPE || e.VirtualKeyCode == vtinput.VK_F10) {
		return true
	}
	return handled
}
