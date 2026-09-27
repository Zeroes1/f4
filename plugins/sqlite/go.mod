module github.com/unxed/f4/plugins/sqlite

go 1.26.6

// f4#1178 part 1 of 4 (same plan as plugins/cloudfox/android/ios; see
// plugins/cloudfox/go.mod's own comment for the pattern this mirrors).
//
// SQLite is its own module so its own code (the panel command, the
// mounted-database VFS, the interactive client) no longer links into the
// main github.com/unxed/f4 module, in either the full or the lite build --
// the same architectural "download on demand" treatment as cloudfox,
// android and iOS (see plugins/sqlite/rpc_plugin.go's package comment for
// the RPC adapter this module's own binary runs).
//
// Unlike cloudfox, this is only a partial dependency win: this module still
// requires github.com/ncruces/go-sqlite3 (a WASM build of SQLite), but so
// does the root module already, independently -- internal/sheet/store.go
// (the native ".f4s.sqlite" spreadsheet format, an unrelated feature) imports
// the very same driver package directly and unconditionally, so extracting
// plugins/sqlite does not remove go-sqlite3 (or whatever it in turn needs)
// from f4's own build graph. It still uses the main module's vfs,
// vfs/hostpath and sdk/f4plugin packages (a plain in-repo path, not a
// separate checkout), hence the replace below.
//
// This go.mod is intentionally not hand-tidied: per this repo's own policy
// (no local `go build`/`go mod tidy` -- see the build-sqlite-plugin CI job
// in .github/workflows/build.yml), the require block below was written by
// reading plugins/sqlite's imports rather than by running the Go toolchain.
// That CI job runs `go mod tidy` before building, which is the actual
// source of truth for the resulting go.mod/go.sum; committing its output
// back (or correcting anything it changes here) is expected follow-up, not
// a sign this file is wrong on arrival.
require (
	github.com/ncruces/go-sqlite3 v0.35.2
	github.com/unxed/f4 v0.0.0
	github.com/unxed/vtinput v0.1.8
	github.com/unxed/vtui v0.1.367
	github.com/vmihailenco/msgpack/v5 v5.4.1
)

replace github.com/unxed/f4 => ../..

// Same forks the root module uses (see ../../go.mod) -- vtui transitively
// needs ffi.Available, which only exists in unxed/pureffi, not upstream
// ebitengine/purego. Without these, `go mod tidy` here resolves the
// vanilla upstream modules instead and the build fails with
// "undefined: ffi.Available".
replace github.com/ebitengine/purego => github.com/unxed/pureffi v0.1.20

replace github.com/ebitengine/hideconsole => ../../internal/hideconsole

replace github.com/go-webgpu/goffi => github.com/unxed/goffi v0.1.11
