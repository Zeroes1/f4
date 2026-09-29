package panel

import (
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/toast"
)

// A bookmark of a panel-plugin location (M-Commander parity, unxed/f4#1669).
//
// far2l keeps the identity of a plugin panel in the Plugin, PluginData and
// PluginFile keys of bookmarks.ini. f4 writes the ID of its own
// vfs.PanelProvider into Plugin behind a fixed prefix, so a plugin name that
// far2l wrote (for example "NetRocks") can never be mistaken for one of
// ours, and the file stays loadable by far2l. Path keeps the directory the
// plugin panel was opened over, which is what the panel shows underneath and
// what an older f4, or a host without the plugin, still navigates to.
const bookmarkPanelPluginPrefix = "f4-panel:"

// bookmarkPanelProviderID returns the panel provider a bookmark asks to open
// after navigating to its Path.
func bookmarkPanelProviderID(b Bookmark) (string, bool) {
	if !strings.HasPrefix(b.Plugin, bookmarkPanelPluginPrefix) {
		return "", false
	}
	id := strings.TrimSpace(strings.TrimPrefix(b.Plugin, bookmarkPanelPluginPrefix))
	return id, id != ""
}

// BookmarkForPanel describes the location fsp shows right now: its VFS path
// and, when a panel plugin covers it (Git status, process list, ...), that
// plugin's provider identity.
func (pf *PanelsFrame) BookmarkForPanel(fsp *FileSystemPanel) Bookmark {
	if fsp == nil || fsp.Vfs == nil {
		return Bookmark{}
	}
	b := Bookmark{Path: fsp.Vfs.GetPath()}
	if pf == nil {
		return b
	}
	for slot, p := range pf.Panels {
		if p != Panel(fsp) {
			continue
		}
		if inst, ok := pf.AltPanels[slot].(*PluginPanelInstance); ok && inst != nil && inst.providerID != "" {
			b.Plugin = bookmarkPanelPluginPrefix + inst.providerID
		}
		break
	}
	return b
}

// openBookmarkPanelProvider opens the provider a bookmark names over the
// panel it has just navigated. A provider that is not registered (the plugin
// is disabled or not installed here) leaves the directory in place and says
// so instead of failing silently.
func (pf *PanelsFrame) openBookmarkPanelProvider(providerID string) {
	if _, ok := plughost.LookupPanelProvider(providerID); !ok {
		toast.Show(fmt.Sprintf(i18n.Msg("Bookmarks.PanelPluginUnavailable"), providerID), 3e9)
		return
	}
	OpenRegisteredPanelProvider(pf, providerID)
}
