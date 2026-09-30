package app

import (
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/macro"
)

// The editor reports what happens to it through editor.OnEvent; the Lua macros
// hear it as their EditorEvent (f4#1686, Step 7).
func init() {
	editor.OnEvent = func(ev *editor.EditorView, event int) {
		macro.MacroMgr.RaiseEditorEvent(ev.MacroID, event)
	}
}
