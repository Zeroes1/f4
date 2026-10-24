package settings

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

// hotkeyLinksFakeHost stands in for the application's Hotkey Configurator
// page: it keeps the links store the window hands it and counts how often the
// window asks the page to list its rows again.
type hotkeyLinksFakeHost struct {
	testHost
	links     *panel.DriveLinkStore
	refreshes *int
}

type hotkeyLinksFakePage struct {
	*vtui.Group
	refreshes *int
}

func (p hotkeyLinksFakePage) Refresh() { *p.refreshes++ }

func (h hotkeyLinksFakeHost) HotkeyPage(_ *vtui.Window, _ func(*keymap.HotkeyManager), links panel.DriveLinkStore) vtui.UIElement {
	*h.links = links
	return hotkeyLinksFakePage{Group: vtui.NewGroup(0, 0, 40, 10), refreshes: h.refreshes}
}

// f4#2171: the Hotkey Configurator page edits the links' global hot keys in
// the window's draft of its Drive links, so both pages show the same links
// and Apply writes them. The page's link dialog checked the key against the
// window's draft of the hotkey table and may have taken it from a command
// there, so Apply does not refuse it for the command the live table still
// has; the mark that says so never reaches the links file. The page lists its
// rows again when it comes back into view.
func TestSettingsHotkeyPageEditsTheDraftOfTheDriveLinks(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(160, 50)
	vtui.FrameManager.Init(scr)
	previousHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{},
		Bindings: map[string]map[string]string{"Shell": {"F5": "Test.Copy"}},
	}
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = previousHotkeys })
	path := panel.DriveBookmarksFilePath()
	t.Cleanup(func() { _ = os.Remove(path) })
	if err := panel.SaveDriveBookmarks(path, []panel.DriveBookmark{
		{Name: "Logs", Path: "/Logs"},
		{Name: "Docs", Path: "/Docs", GlobalHotkey: "CtrlAltD"},
	}); err != nil {
		t.Fatal(err)
	}

	var links panel.DriveLinkStore
	refreshes := 0
	previousHost := host
	Configure(hotkeyLinksFakeHost{links: &links, refreshes: &refreshes})
	t.Cleanup(func() { Configure(previousHost) })

	p := newCoreRecordSettingsProvider()
	d, err := p.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{provider: p, catalog: p.Catalog(), draft: d}})
	c.selectCategory("hotkeys")
	if links == nil {
		t.Fatal("the page got no links store")
	}
	if got := links.DriveLinks(); len(got) != 2 || got[0].Name != "Logs" || got[1].GlobalHotkey != "CtrlAltD" {
		t.Fatalf("the page sees links %+v", got)
	}

	logs := links.DriveLinks()[0]
	logs.GlobalHotkey = "F5"
	if err := links.SetDriveLink(0, logs); err != nil {
		t.Fatal(err)
	}
	rows := d.Records["drive-links"]
	if rows[0].Values["link.GlobalHotkey"] != "F5" {
		t.Fatalf("the draft holds %+v", rows[0].Values)
	}
	store := settingsRecordStoreByID(t, p, "drive-links")
	if err := store.validate(rows); err != nil {
		t.Fatalf("Apply refused the key the page's link dialog checked: %v", err)
	}
	unchecked := f4settings.Record{ID: "drive-link:x", Values: map[string]string{"link.Name": "Logs", "link.Path": "/Logs", "link.GlobalHotkey": "F5"}}
	if err := store.validate([]f4settings.Record{unchecked}); err == nil || !strings.Contains(err.Error(), "Test.Copy") {
		t.Fatalf("a key typed on the Drive links page was not refused: %v", err)
	}
	if err := store.save(rows); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "GlobalHotkey=F5") || strings.Contains(string(data), "Checked") {
		t.Fatalf("links file:\n%s", data)
	}

	c.selectCategory("drives")
	c.selectCategory("hotkeys")
	if refreshes != 1 {
		t.Fatalf("the page was asked to list its rows %d times on coming back, want 1", refreshes)
	}
}
