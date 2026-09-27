// Command sqlite-plugin is the standalone subprocess entry point for the
// SQLite VFS mount (f4#1178, part 1 of 4 of the same plan as
// plugins/cloudfox/cmd/cloudfox-plugin, plugins/android/cmd/android-plugin
// and plugins/ios/cmd/ios-plugin).
//
// It links the same databaseProvider/databaseVFS/databaseSession code that
// used to be registered in-process by internal/plughost's loadInternal
// (plugins/sqlite), now built into its own binary via plugins/sqlite's own
// go.mod so that code -- the panel command, the mounted-database VFS, the
// interactive client -- no longer links into f4 itself, in either the full
// or the lite build. This is only a partial dependency win, unlike
// cloudfox/android/ios: internal/sheet/store.go (the native ".f4s.sqlite"
// spreadsheet format, an unrelated feature) imports the same
// github.com/ncruces/go-sqlite3 driver directly and unconditionally, so
// that dependency stays in f4's own build graph regardless; see
// plugins/sqlite/rpc_plugin.go's package comment. f4 launches this binary
// as a native OS subprocess and talks to it over the standard F4-RPC
// transport (docs/PLUGINS.md, sdk/f4plugin), exactly like any other
// subprocess plugin (see plugins/dummy_rpc/main.go for the minimal
// reference example this mirrors).
//
// See plugring-manifest.json in this directory for the declarative
// manifest PlugRing needs to offer this as an installable plugin.
// Building and publishing release binaries per platform is part 2 of the
// plan; internal/plughost/plugring_firstparty.go's FirstPartyPlugRingItems
// (part 3) mirrors this same manifest by hand into a first-party PlugRing
// entry. Keep the two in sync when either changes; they cannot share code
// across the module boundary plugins/sqlite's own go.mod draws (part 1).
package main

import (
	"github.com/unxed/f4/plugins/sqlite"
	"github.com/unxed/f4/sdk/f4plugin"
)

func main() {
	f4plugin.Run(sqlite.NewRPCPlugin())
}
