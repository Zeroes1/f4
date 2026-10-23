package panel

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// A link of the drive menu can have a global hot key (f4#2171): a key of the
// user's choosing that opens the link from the panels without the menu. It
// lives in the link itself, in drive-bookmarks.ini, next to the letter that
// picks the link in the open menu; there is no separate table of link keys.
// The link dialog says what else answers to the key and lets the user take it
// from them or choose another.

// driveLinkHotkeyParts is a key in the spelling of keymap.EventToHotkeyString
// split into its modifiers and the key itself.
type driveLinkHotkeyParts struct {
	rctrl, ctrl, alt, shift bool
	base                    string
}

func splitDriveLinkHotkey(key string) driveLinkHotkeyParts {
	var p driveLinkHotkeyParts
	rest := strings.TrimSpace(key)
	take := func(prefix string) bool {
		if len(rest) > len(prefix) && strings.EqualFold(rest[:len(prefix)], prefix) {
			rest = rest[len(prefix):]
			return true
		}
		return false
	}
	if take("RCtrl") {
		p.rctrl = true
	} else if take("Ctrl") {
		p.ctrl = true
	}
	p.alt = take("Alt")
	p.shift = take("Shift")
	p.base = rest
	return p
}

func isDriveLinkFunctionKey(base string) bool {
	if len(base) < 2 || (base[0] != 'F' && base[0] != 'f') {
		return false
	}
	n := 0
	for _, c := range base[1:] {
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
		if n > 24 {
			return false
		}
	}
	return n >= 1
}

// DriveLinkGlobalKeyAllowed reports whether key can be a link's global hot
// key: it holds Ctrl or Alt, or it is a function key. A key without either
// types text or moves the cursor, so the panels and the command line could no
// longer be used with it.
func DriveLinkGlobalKeyAllowed(key string) bool {
	p := splitDriveLinkHotkey(key)
	return p.base != "" && (p.ctrl || p.rctrl || p.alt || isDriveLinkFunctionKey(p.base))
}

// driveLinkForKey finds the link whose global hot key key is. A link written
// with Ctrl answers either Ctrl, as a hotkey binding does; one written with
// RCtrl answers the right one only, and it wins over a Ctrl link on the right
// Ctrl.
func driveLinkForKey(links []DriveBookmark, key string) (DriveBookmark, bool) {
	find := func(k string) (DriveBookmark, bool) {
		for _, link := range links {
			if driveBookmarkIsValid(link) && strings.EqualFold(strings.TrimSpace(link.GlobalHotkey), k) {
				return link, true
			}
		}
		return DriveBookmark{}, false
	}
	if key == "" {
		return DriveBookmark{}, false
	}
	if link, ok := find(key); ok {
		return link, true
	}
	if strings.HasPrefix(key, "RCtrl") {
		return find("Ctrl" + key[len("RCtrl"):])
	}
	return DriveBookmark{}, false
}

// driveLinksCache keeps the links file between keys: the key router asks for
// the links on every chord pressed in the panels. A change of the file's time
// or size reads it again, and SaveDriveBookmarks drops it, so a link saved in
// the same second as the previous one is not missed.
var driveLinksCache struct {
	sync.Mutex
	valid   bool
	path    string
	modTime time.Time
	size    int64
	links   []DriveBookmark
}

func cachedDriveLinks() []DriveBookmark {
	path := DriveBookmarksFilePath()
	info, err := os.Stat(path)
	c := &driveLinksCache
	c.Lock()
	defer c.Unlock()
	if err != nil {
		c.valid = false
		return nil
	}
	if c.valid && c.path == path && c.modTime.Equal(info.ModTime()) && c.size == info.Size() {
		return c.links
	}
	links, err := LoadDriveBookmarks(path)
	if err != nil {
		vtui.DebugLog("DRIVE BOOKMARKS: load %q failed: %v", path, err)
		c.valid = false
		return nil
	}
	c.valid, c.path, c.modTime, c.size, c.links = true, path, info.ModTime(), info.Size(), links
	return links
}

func forgetDriveLinksCache() {
	driveLinksCache.Lock()
	driveLinksCache.valid = false
	driveLinksCache.links = nil
	driveLinksCache.Unlock()
}

// OpenDriveLinkForKey opens, in the active panel, the link whose global hot
// key e is (f4#2171), and reports whether there was one. The application's key
// router asks it while the panels are on screen, before macros, plugin keys
// and the hotkey table: the link dialog asked whether to take the key from
// them when the key was given to the link.
func (pf *PanelsFrame) OpenDriveLinkForKey(e *vtinput.InputEvent) bool {
	if pf == nil || e == nil || e.Type != vtinput.KeyEventType || !e.KeyDown || !pf.ShowPanels {
		return false
	}
	key := keymap.EventToHotkeyString(e)
	if !DriveLinkGlobalKeyAllowed(key) {
		return false
	}
	link, ok := driveLinkForKey(cachedDriveLinks(), key)
	if !ok {
		return false
	}
	if fsp := pf.GetActivePanel(); fsp != nil {
		pf.NavigateToBookmark(fsp, Bookmark{Path: link.Path})
	}
	return true
}

// DriveLinkKeyOwner is something a link's global hot key would take its key
// from (f4#2171): a command bound to it, another link, a macro, or something
// the panels do with the key themselves.
type DriveLinkKeyOwner struct {
	// Name says what it is, as the link dialog lists it.
	Name string
	// Fixed: the key cannot be taken. What it does is done before the links
	// are asked, so the link would never see the key.
	Fixed bool
	// Release takes the key from it when the link takes the key. Nil when
	// there is nothing to undo: in the panels the link simply comes first.
	Release func() error
}

// DriveLinkExtraKeyOwners adds what only the application knows: its macros,
// and the keys its key router handles before it asks the links. The
// application fills it in; nil adds nothing.
var DriveLinkExtraKeyOwners func(key string) []DriveLinkKeyOwner

// driveLinkKeyOwners lists what the key would be taken from if a link got it
// as its global hot key: the reserved keys first, then the other links, the
// commands of the panels, the panels' own keys and the application's part.
// others are the links besides the one being edited.
func driveLinkKeyOwners(key string, others []DriveBookmark) []DriveLinkKeyOwner {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	var owners []DriveLinkKeyOwner
	hm := keymap.GlobalHotkeysMgr

	// The numbered folder bookmarks answer before the links do, unless the
	// user bound the key to a command (ConfigurableHotkeyOwnsPanelBookmark).
	if e := keymap.ParseFarKey(key); e != nil && keymap.IsPanelBookmarkHotkey(e) &&
		!keymap.ConfigurableHotkeyOwnsPanelBookmark(hm, "Shell", e) {
		owners = append(owners, DriveLinkKeyOwner{Name: driveLinkBookmarkKeyName(e), Fixed: true})
	}

	for _, link := range others {
		if driveBookmarkIsValid(link) && strings.EqualFold(strings.TrimSpace(link.GlobalHotkey), key) {
			// The links file is written once, with the new link in it; the
			// editor clears the key of the others there.
			owners = append(owners, DriveLinkKeyOwner{Name: fmt.Sprintf(i18n.Msg("DriveLink.OwnerLink"), strings.TrimSpace(link.Name))})
		}
	}

	// A Ctrl link catches the right Ctrl as well, so a command bound to the
	// RCtrl spelling loses it too. An RCtrl link leaves the left Ctrl alone.
	spellings := []string{key}
	if p := splitDriveLinkHotkey(key); p.ctrl {
		spellings = append(spellings, "R"+key)
	}
	for _, spelling := range spellings {
		if name := driveLinkCommandOn(hm, spelling); name != "" {
			spelling := spelling
			owners = append(owners, DriveLinkKeyOwner{
				Name: fmt.Sprintf(i18n.Msg("DriveLink.OwnerCommand"), name),
				Release: func() error {
					// "None" in Shell: the command keeps the key where the
					// links are never asked, as a Common binding does in
					// the editor.
					hm.Bind("Shell", spelling, "None")
					return hm.SaveError()
				},
			})
		}
	}

	if p := splitDriveLinkHotkey(key); p.alt && !p.ctrl && !p.rctrl {
		if !p.shift && len(p.base) == 1 && p.base[0] >= '1' && p.base[0] <= '9' &&
			vtui.FrameManager != nil && vtui.FrameManager.WorkspaceAltNumberSwitch {
			owners = append(owners, DriveLinkKeyOwner{Name: fmt.Sprintf(i18n.Msg("DriveLink.OwnerWorkspace"), int(p.base[0]-'0'))})
		}
		if driveLinkTypesCharacter(p.base) {
			owners = append(owners, DriveLinkKeyOwner{Name: i18n.Msg("DriveLink.OwnerFastFind")})
		}
	}

	if DriveLinkExtraKeyOwners != nil {
		owners = append(owners, DriveLinkExtraKeyOwners(key)...)
	}
	return owners
}

// DriveLinkGlobalKeyOwners lists what key would be taken from besides the
// other links: the reserved keys, the commands, the panels' own keys and the
// application's part (f4#2171). The Settings window, which cannot ask whether
// to take a key, refuses a new one that has any.
func DriveLinkGlobalKeyOwners(key string) []DriveLinkKeyOwner {
	return driveLinkKeyOwners(key, nil)
}

// driveLinkTypesCharacter reports whether base, the key without its
// modifiers, is one that types a character: Alt with it starts the quick
// search of the panel, as in Far.
func driveLinkTypesCharacter(base string) bool {
	if utf8.RuneCountInString(base) == 1 {
		return true
	}
	return strings.EqualFold(base, "Space") || strings.HasPrefix(strings.ToUpper(base), "VK_")
}

// driveLinkCommandOn names the command spelling runs in the panels: the Shell
// binding, else the Common one, as the hotkey manager resolves it. A key
// unbound with "None" runs nothing.
func driveLinkCommandOn(hm *keymap.HotkeyManager, spelling string) string {
	if hm == nil {
		return ""
	}
	for _, area := range []string{"Shell", "Common"} {
		binding, ok := hm.Bindings[area][spelling]
		if !ok {
			continue
		}
		name, _, _ := strings.Cut(binding, ":")
		name = strings.TrimSpace(name)
		if name == "" || strings.EqualFold(name, "none") {
			return ""
		}
		if a, ok := keymap.LookupAction(name); ok {
			if label := action.PlainLabel(a.DisplayLabel()); label != "" {
				return label
			}
		}
		return name
	}
	return ""
}

// driveLinkBookmarkKeyName names what a folder bookmark key does.
func driveLinkBookmarkKeyName(e *vtinput.InputEvent) string {
	if e.VirtualKeyCode == vtinput.VK_OEM_3 {
		return i18n.Msg("DriveLink.OwnerHomeFolder")
	}
	slot := int(e.VirtualKeyCode - vtinput.VK_0)
	if e.ControlKeyState&vtinput.ShiftPressed != 0 {
		return fmt.Sprintf(i18n.Msg("DriveLink.OwnerBookmarkSave"), slot)
	}
	return fmt.Sprintf(i18n.Msg("DriveLink.OwnerBookmark"), slot)
}
