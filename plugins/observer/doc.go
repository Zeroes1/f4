// Package observer is the host side of running Observer
// (github.com/lazyhamster/Observer) archive-format modules inside f4,
// compiled to WebAssembly and run in-process on top of wazero, the same way
// internal/plughost/transport_wazero.go runs a wasm plugin and colorer4go
// (github.com/unxed/colorer4go) runs a wasm C++ library.
//
// # Scope of this part (f4#1563, part 1 of N)
//
// This package is infrastructure only. It has no notion of any real archive
// format, is not registered as a vfs.VFSProvider, and is not reachable from
// Enter on a panel. What it does provide, and what its tests exercise
// end-to-end against a small test-only module (see testdata/stub), is:
//
//   - Go types and constants for the Observer module ABI (API v6, see
//     src/common/ModuleDef.h in lazyhamster/Observer), laid out the way a
//     wasm32 build of a module sees them: 4-byte pointers, 4-byte size_t,
//     and 4-byte wchar_t (wasi-sdk compiles wchar_t as a 32-bit int, unlike
//     the 16-bit wchar_t of the real Windows ABI the ModuleDef.h header
//     targets).
//   - A wazero host module ("observer") with the progress callback import.
//   - A loader that can instantiate an arbitrary WASI-reactor .wasm module
//     and drive LoadSubModule/OpenStorage/CloseStorage against it, proving
//     the struct marshaling and the host imports both work.
//   - Read access to the probed file for the guest, through a WASI
//     filesystem mount backed by an io.ReaderAt-like view of the parent
//     VFS, not a real path on the host disk. That is what will eventually
//     let a module opened on a nested archive member read straight through
//     to wherever the bytes actually live.
//
// # The module_cbs indirection
//
// Real Observer modules export only LoadSubModule and UnloadSubModule.
// LoadSubModule fills in a ModuleLoadParameters.ApiFuncs table of five
// function pointers (OpenStorage, CloseStorage, GetItem, ExtractItem,
// PrepareFiles) that the Windows host is meant to call directly. In wasm a C
// function pointer is an index into the module's own internal indirect-call
// table, which wazero has no public API to invoke from the host side without
// the module also exporting a callable trampoline for it.
//
// So a module targeting f4 additionally exports flat, pointer-taking
// trampolines under the fixed names in ExportOpenStorage, ExportCloseStorage
// and so on (see the Export* constants below), each simply forwarding to
// whatever module_cbs entry the module itself populated. ApiFuncs is still
// read back and kept on ModuleInfo after LoadSubModule returns, purely so a
// caller (or a test) can confirm the module actually populated its table --
// f4 never calls through those values itself.
//
// Only the trampolines this part actually drives -- ExportOpenStorage and
// ExportCloseStorage -- are required of testdata/stub's module; GetItem,
// ExtractItem and PrepareFiles are reserved names for a later part.
package observer
