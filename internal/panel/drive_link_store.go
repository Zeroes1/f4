package panel

import (
	"strings"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtui"
)

// The Hotkey Configurator lists the global hot keys of the drive menu's links
// next to the keys of the commands (f4#2171): every key f4 reacts to is a row
// there. It changes them in a store of its own, as it changes the hotkey table
// in a draft: the links file, read when the configurator opens and written
// when the user saves, or the Settings window's draft of its Drive links.

// DriveLinkStore is where the Hotkey Configurator finds the drive menu's links
// and puts a link it changed. index is the link's place in DriveLinks.
type DriveLinkStore interface {
	DriveLinks() []DriveBookmark
	SetDriveLink(index int, link DriveBookmark) error
}

// DriveLinksDraft is the links file as an editor read it. The changes stay in
// the draft until Save.
type DriveLinksDraft struct {
	path    string
	links   []DriveBookmark
	changed bool
}

// LoadDriveLinksDraft reads the links file into a draft.
func LoadDriveLinksDraft() (*DriveLinksDraft, error) {
	path := DriveBookmarksFilePath()
	links, err := LoadDriveBookmarks(path)
	return &DriveLinksDraft{path: path, links: links}, err
}

func (d *DriveLinksDraft) DriveLinks() []DriveBookmark {
	return append([]DriveBookmark(nil), d.links...)
}

func (d *DriveLinksDraft) SetDriveLink(index int, link DriveBookmark) error {
	if index < 0 || index >= len(d.links) {
		return nil
	}
	if d.links[index] != link {
		d.links[index] = link
		d.changed = true
	}
	return nil
}

// Save writes the links file when the draft has changed.
func (d *DriveLinksDraft) Save() error {
	if !d.changed {
		return nil
	}
	if err := SaveDriveBookmarks(d.path, d.links); err != nil {
		return err
	}
	d.changed = false
	return nil
}

// PutDriveLink puts link at index in store and takes its global hot key from
// the other links that had it: the user agreed to that in the link dialog,
// which lists them (f4#2171).
func PutDriveLink(store DriveLinkStore, index int, link DriveBookmark) error {
	if err := store.SetDriveLink(index, link); err != nil {
		return err
	}
	key := strings.TrimSpace(link.GlobalHotkey)
	if key == "" {
		return nil
	}
	for i, other := range store.DriveLinks() {
		if i != index && strings.EqualFold(strings.TrimSpace(other.GlobalHotkey), key) {
			other.GlobalHotkey = ""
			if err := store.SetDriveLink(i, other); err != nil {
				return err
			}
		}
	}
	return nil
}

// EditDriveLinkIn opens the link dialog on links[index] for the Hotkey
// Configurator, with the global hot key field focused. The key is checked
// against hm, the configurator's draft of the hotkey table, and a command the
// user takes the key from gives it up there. done gets the link once the user
// saves it; the caller puts it into its store (PutDriveLink).
func EditDriveLinkIn(hm *keymap.HotkeyManager, links []DriveBookmark, index int, done func(DriveBookmark)) {
	if index < 0 || index >= len(links) || vtui.FrameManager == nil {
		return
	}
	var others []DriveBookmark
	for i, link := range links {
		if i != index {
			others = append(others, link)
		}
	}
	dialog := NewDriveBookmarkEditDialog(links[index], "", func(saved bool, link DriveBookmark) {
		if saved && done != nil {
			done(link)
		}
	})
	dialog.others = others
	dialog.hotkeys = hm
	dialog.SetFocusedItem(dialog.globalEdit)
	vtui.FrameManager.Push(dialog)
}

// DriveLinkBeforeCommand finds the link that gets key in the panels before a
// command bound to it in area would (f4#2171): the key router asks the links
// before the hotkey table there. Only the bindings of Shell and Common work in
// the panels. A Ctrl link answers the right Ctrl as well, and an RCtrl link
// takes the right Ctrl from a command on plain Ctrl, which answers either
// (driveLinkKeySpellings). It reports the link's index in links.
func DriveLinkBeforeCommand(links []DriveBookmark, area, key string) (int, bool) {
	if strings.TrimSpace(key) == "" || (!strings.EqualFold(area, "Shell") && !strings.EqualFold(area, "Common")) {
		return 0, false
	}
	spellings := driveLinkKeySpellings(key)
	for i, link := range links {
		global := strings.TrimSpace(link.GlobalHotkey)
		if driveBookmarkIsValid(link) && global != "" && spellings[driveLinkCanonicalKey(global)] {
			return i, true
		}
	}
	return 0, false
}
