// Command android-plugin is the standalone subprocess entry point for the
// Android drive (f4#1178: the owner's decision at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851392645 to give
// Android the same "download this plugin on demand" treatment as cloud
// storage/iOS, part 1 of 4 of that same plan).
//
// It links the same ADB device-discovery and FISH+/ADB-Sync provider code
// that used to be registered in-process by internal/plughost's build
// (plugins/android), now built into its own binary via plugins/android's
// own go.mod. Unlike plugins/cloudfox's extraction, this one is not about
// shedding a large unique dependency -- plugins/android has none of its
// own, being a thin wrapper around the local `adb` server/executable, the
// same pattern as plugins/multiarc's CLI-archiver wrapper -- it exists so
// all three plugins the owner named (cloud storage, iOS, Android) install
// through one uniform mechanism. See plugins/android/rpc_plugin.go's
// package comment for the one dependency this extraction still had to
// account for (plugins/netfox's FISH+ reuse, and why this module builds
// with -tags lite). f4 launches this binary as a native OS subprocess and
// talks to it over the standard F4-RPC transport (docs/PLUGINS.md,
// sdk/f4plugin), exactly like any other subprocess plugin (see
// plugins/dummy_rpc/main.go for the minimal reference example this
// mirrors, and plugins/cloudfox/cmd/cloudfox-plugin/main.go for the sibling
// extraction this one repeats).
//
// See plugring-manifest.json in this directory for the declarative
// manifest a later part of the plan needs to offer this as an installable
// plugin. Publishing release binaries per platform, and wiring an actual
// install path through PlugRing or a lite-build menu entry, are parts 2-4
// of the plan and not done here.
package main

import (
	androidfs "github.com/unxed/f4/plugins/android"
	"github.com/unxed/f4/sdk/f4plugin"
)

func main() {
	f4plugin.Run(androidfs.NewRPCPlugin())
}
