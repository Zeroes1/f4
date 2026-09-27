package app

import (
	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/vfs"
)

// sqlitePluginCommandID mirrors the ID plugins/sqlite registers with.
//
// The plugin owns the command; this file only gives it a key and a menu row.
// Bindings resolve through the action registry (hotkeys.ini -> action name ->
// RunAction), so a plugin command cannot claim one on its own: its Shortcut
// field is display-only metadata, as vfs.PluginCommand says.
//
// f4#1178 part 1 of 4 moved plugins/sqlite out of every build (see
// internal/plughost/manager.go's own comment on loadInternal), so no
// in-process plugin ever registers this ID any more. It stays here as a
// dead-but-documented constant for exactly the same reason
// androidPlugRingID/cloudStoragePlugRingID do in android_plugin_menu.go/
// cloud_storage_lite.go: plugins/sqlite's own RPC plugin
// (plugins/sqlite/rpc_plugin.go) does not implement f4plugin.CommandProvider
// either -- see that file's package comment for why -- so this ID has no
// path back into ExecutePluginCommand succeeding again until a later part
// ports the interactive client itself.
const sqlitePluginCommandID = "f4.sqlite.open"

// sqlitePlugRingID is plugins/sqlite/cmd/sqlite-plugin's PlugRing catalog
// id, duplicated from internal/plughost/plugring_firstparty.go's
// FirstPartyPlugRingItems by hand for the same reason
// androidPlugRingID/cloudStoragePlugRingID are: there is no shared constant
// for it, only the literal in that one first-party list entry.
const sqlitePlugRingID = "sqlite"

// actionSQLiteClient opens the SQLite client on the database under the panel
// cursor.
//
// The panels frame is found by walking the frame stack rather than by asking
// for the top frame: when this runs from the menu or from the command palette,
// that popup is what sits on top.
//
// False means the SQLite plugin is not loaded at all -- true today in every
// build, per this file's own comment on sqlitePluginCommandID -- and, per
// f4#1178 part 4 of 4, the honest, working thing to do instead of leading
// nowhere is send the user straight to the entry that installs it
// (androidDriveFactory/cloudStorageLiteDriveFactory do the same for their own
// now-external plugin, in android_plugin_menu.go/cloud_storage_lite.go). When
// a plugin command IS registered under this ID -- were a later part to add
// PanelCommand support to plugins/sqlite/rpc_plugin.go, or a build that still
// loads a plugin under this ID in-process -- ExecutePluginCommand finds and
// runs it before this fallback is ever reached, exactly as it always did.
func actionSQLiteClient() bool {
	pf := panel.FindPanelsFrameAnyScreen()
	if pf == nil {
		return false
	}
	if plughost.ExecutePluginCommand(vfs.PluginCommandPanel, sqlitePluginCommandID, pf) {
		return true
	}
	actionPlugRingFocused(pf, sqlitePlugRingID)
	return false
}

func init() {
	registerAction(action.Action{
		Name:        "App.SQLite",
		Area:        "Shell",
		Label:       "SQLite &client",
		LabelKey:    "Action.App.SQLite",
		Description: "Open the SQLite client on the database under the cursor",
		DescKey:     "Action.App.SQLite.Desc",
		// D for database, next to the spreadsheet's Ctrl+Alt+S. Nothing in
		// the registry claims it: Ctrl+Alt is otherwise taken by A, Ins, L,
		// M, P, S and the digits that switch workspaces and jump to
		// bookmarks. What the registry cannot know is that Ctrl+Alt is AltGr
		// on many keyboard layouts, where the combination composes a
		// character and never reaches us; rebinding in hotkeys.ini remains
		// the answer there.
		DefaultKeys: []string{"CtrlAltD"},
		MenuPath:    "Commands",
		// Deliberately no Visible predicate. It is asked every time the menu
		// is built, and a row that comes and goes with the panel cursor is
		// what made this command impossible to find.
		Handler: actionSQLiteClient,
	})
}
