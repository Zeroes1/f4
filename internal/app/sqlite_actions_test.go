package app

import (
	"context"
	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// sqliteMenuItem finds the SQLite client entry in a built menu bar.
func sqliteMenuItem(items []vtui.MenuBarItem) (vtui.MenuBarItem, vtui.MenuItem, bool) {
	label := action.PlainLabel(i18n.Msg("Action.App.SQLite"))
	for _, bar := range items {
		for _, item := range bar.SubItems {
			if action.PlainLabel(item.Text) == label {
				return bar, item, true
			}
		}
	}
	return vtui.MenuBarItem{}, vtui.MenuItem{}, false
}

// TestSQLiteClientIsInTheCommandsMenuWithItsKey covers what the user actually
// looks for: a row that names the command and the key that runs it.
//
// The second half is the spreadsheet's lesson. Menu items are rebuilt on every
// GetMenuBar call and the open dropdown is the top frame while they are, so a
// menu-visible entry must not depend on what sits on top.
func TestSQLiteClientIsInTheCommandsMenuWithItsKey(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	oldHotkeys := keymap.GlobalHotkeysMgr
	keymap.GlobalHotkeysMgr = keymap.NewHotkeyManager("")
	t.Cleanup(func() { keymap.GlobalHotkeysMgr = oldHotkeys })

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	bar, item, ok := sqliteMenuItem(pf.GetMenuBar().Items)
	if !ok {
		t.Fatal("the SQLite client is missing from the panels menu")
	}
	if commands := action.PlainLabel(i18n.Msg("Menu.Shell.Commands")); !strings.Contains(bar.Label, commands) {
		t.Errorf("the SQLite client sits in %q, expected %q", bar.Label, commands)
	}
	if want := keymap.FormatKeyForUI("CtrlAltD"); item.Shortcut != want {
		t.Errorf("the SQLite client shows %q, expected %q", item.Shortcut, want)
	}

	popup := vtui.NewVMenu("dropdown")
	popup.SetPosition(1, 1, 20, 10)
	vtui.FrameManager.Push(popup)
	if _, _, ok := sqliteMenuItem(BuildMenuBarItems("Shell")); !ok {
		t.Error("the SQLite client disappeared from the menu while a popup was on top")
	}
}

// TestSQLiteActionReachesThePluginCommand checks both ends of the bridge: no
// plugin means no success (f4#1178 part 4 sends the user to PlugRing
// instead, which TestSQLiteActionOpensPlugRingFocusedOnSQLiteWhenNotLoaded
// pins down on its own; this test just drains and closes that dialog so it
// does not linger), and a registered plugin is handed the panels frame even
// when a popup is what the command was chosen from.
func TestSQLiteActionReachesThePluginCommand(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	previousCatalog := plugRingCatalog
	plugRingCatalog = func(context.Context) ([]plughost.PlugRingItem, error) { return nil, nil }
	t.Cleanup(func() { plugRingCatalog = previousCatalog })

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	if actionSQLiteClient() {
		t.Error("the action reported success with no SQLite plugin loaded")
	}

	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("no SQLite plugin loaded did not open the PlugRing dialog, top frame = %T", vtui.FrameManager.GetTopFrame())
	}
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the PlugRing catalog fetch")
	}
	dlg.Close()

	var ran vfs.App
	registration, err := (&coreAPI{}).RegisterPluginCommand(vfs.PluginCommand{
		ID:       sqlitePluginCommandID,
		Location: vfs.PluginCommandPanel,
		Label:    "SQLite client",
		Run:      func(app vfs.App) { ran = app },
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(registration.Unregister)

	popup := vtui.NewVMenu("dropdown")
	popup.SetPosition(1, 1, 20, 10)
	vtui.FrameManager.Push(popup)

	if !actionSQLiteClient() {
		t.Fatal("the action did not reach the registered plugin command")
	}
	if ran != vfs.App(pf) {
		t.Errorf("the command ran against %v, expected the panels frame", ran)
	}
}

// TestSQLiteActionOpensPlugRingFocusedOnSQLiteWhenNotLoaded is f4#1178, part
// 4 of 4: it pins down what Ctrl+Alt+D/the SQLite client menu row does now
// that no build ever loads plugins/sqlite in-process any more (see
// sqlitePluginCommandID's own comment for the history), the same way
// TestAndroidDriveOpensPlugRingFocusedOnAndroid
// (android_plugin_menu_test.go) and
// TestCloudStorageLiteDriveOpensPlugRingFocusedOnCloudfox
// (cloud_storage_lite_test.go) do for their own now-external plugin.
func TestSQLiteActionOpensPlugRingFocusedOnSQLiteWhenNotLoaded(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	previousCatalog := plugRingCatalog
	plugRingCatalog = func(context.Context) ([]plughost.PlugRingItem, error) {
		return []plughost.PlugRingItem{
			{ID: "aaa-before", Name: "Alphabetically first", Entrypoint: "aaa.lua", Category: "tools"},
			{ID: "sqlite", Name: "SQLite database browser", Entrypoint: "sqlite-plugin", Category: "filesystem", FirstParty: true},
			{ID: "zzz-after", Name: "Alphabetically last", Entrypoint: "zzz.lua", Category: "tools"},
		}, nil
	}
	t.Cleanup(func() { plugRingCatalog = previousCatalog })

	pf := panel.NewPanelsFrame()
	defer pf.Close()
	vtui.FrameManager.Push(pf)

	if actionSQLiteClient() {
		t.Error("the action reported success with no SQLite plugin loaded")
	}

	dlg, ok := vtui.FrameManager.GetTopFrame().(*vtui.Window)
	if !ok {
		t.Fatalf("selecting the SQLite client with no plugin loaded did not open a dialog, top frame = %T", vtui.FrameManager.GetTopFrame())
	}
	if got, want := dlg.GetTitle(), i18n.Msg("PlugRing.Title"); got != want {
		t.Fatalf("dialog title = %q, want the PlugRing dialog %q", got, want)
	}

	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the PlugRing catalog fetch")
	}

	var table *vtui.Table
	for _, child := range dlg.GetChildren() {
		if candidate, ok := child.(*vtui.Table); ok {
			table = candidate
			break
		}
	}
	if table == nil {
		t.Fatal("PlugRing dialog has no table")
	}
	if table.SelectPos < 0 || table.SelectPos >= len(table.Rows) {
		t.Fatalf("SelectPos = %d out of range for %d rows", table.SelectPos, len(table.Rows))
	}
	row, ok := table.Rows[table.SelectPos].(plugRingRow)
	if !ok || row.item.ID != "sqlite" {
		t.Fatalf("opening the SQLite client with no plugin loaded did not land on the sqlite row, selected row = %#v", table.Rows[table.SelectPos])
	}
}
