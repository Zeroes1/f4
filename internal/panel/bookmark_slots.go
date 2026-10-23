package panel

import (
	"github.com/unxed/f4/vfs/hostmode"
	"github.com/unxed/vtui"
)

// The numbered folder bookmarks answer far2l's keys: [RightCtrl | Ctrl+Alt]
// and a digit jumps to the slot, the same with Shift stores the active panel's
// location there, and [RightCtrl | Ctrl+Alt] with the grave key goes home.
// PanelsFrame.ProcessKey reads those keys; the methods below are what they do,
// and what the Panel.Bookmark.* actions run from the command palette or a
// macro.

// GoToBookmarkSlot moves the active panel to the folder bookmark in slot
// (0-9). An empty slot does nothing, as in far2l. The table is read afresh:
// another f4 or far2l instance may have rewritten the file.
func (pf *PanelsFrame) GoToBookmarkSlot(slot int) bool {
	if slot < 0 || slot > 9 {
		return false
	}
	file := BookmarksFilePath()
	set, err := LoadBookmarks(file)
	if err != nil {
		vtui.DebugLog("BOOKMARKS: load %q failed: %v", file, err)
		return false
	}
	if set[slot].IsEmpty() {
		return true
	}
	if fsp := pf.GetActivePanel(); fsp != nil {
		pf.NavigateToBookmark(fsp, set[slot])
	}
	return true
}

// SaveBookmarkSlot stores the active panel's location in slot (0-9).
func (pf *PanelsFrame) SaveBookmarkSlot(slot int) bool {
	if slot < 0 || slot > 9 {
		return false
	}
	file := BookmarksFilePath()
	set, err := LoadBookmarks(file)
	if err != nil {
		vtui.DebugLog("BOOKMARKS: load %q failed: %v", file, err)
		return false
	}
	fsp := pf.GetActivePanel()
	if fsp == nil {
		return true
	}
	set[slot] = pf.BookmarkForPanel(fsp)
	if err := SaveBookmarks(file, set); err != nil {
		vtui.DebugLog("BOOKMARKS: save %q failed: %v", file, err)
	}
	return true
}

// GoToHomeFolder moves the active panel to the user's home folder.
func (pf *PanelsFrame) GoToHomeFolder() bool {
	home, _ := hostmode.UserHomeDir()
	if home == "" {
		return false
	}
	if fsp := pf.GetActivePanel(); fsp != nil {
		pf.NavigateToPath(fsp, home)
	}
	return true
}
