//go:build lite

package plughost

import (
	"errors"

	"github.com/unxed/f4/vfs"
)

// This file is the lite side of f4#1178's wasm-plugin removal: it mirrors
// transport_wazero.go's exported shape (WasmPlugin, NewWasmPlugin,
// IsWasmEntrypoint, and the methods newPluginForEntrypoint/plugring_meta.go
// call on them) closely enough that neither of those callers, nor the
// shared identity_test.go, needs a build tag of its own -- but it never
// imports "github.com/tetratelabs/wazero", so a lite build links none of
// wazero's runtime, its wazevo JIT backends, or wasi_snapshot_preview1.
//
// A .wasm entrypoint is still recognised as such (IsWasmEntrypoint stays a
// plain extension check, same as the real file): PlugRingItemProblem still
// accepts a wasm entry into the catalog, since the catalog format itself is
// build-independent. What a lite build cannot do is actually run one, which
// errWasmLiteUnavailable below reports as soon as something tries to Init
// it, exactly the way colorer_lite.go reports Colorer's absence.
var errWasmLiteUnavailable = errors.New("wasm plugins are not available in a lite build; use a regular build to run one")

// WasmPlugin stands in for the real wazero-backed transport. It never runs a
// guest -- Init always fails -- so it only has to carry enough state
// (path, identity) for the permission-identity plumbing every transport
// shares.
type WasmPlugin struct {
	path     string
	identity PluginIdentity
}

// SetPermissionIdentity mirrors the real WasmPlugin's method so a lite build
// compiles newPluginForPlugRingItem unchanged.
func (p *WasmPlugin) SetPermissionIdentity(identity PluginIdentity) {
	p.identity = identity
}

// permissionIdentity mirrors the real WasmPlugin's fallback, unreachable in
// practice since Init never succeeds, but kept for identity_test.go and any
// caller that inspects a *WasmPlugin before starting it.
func (p *WasmPlugin) permissionIdentity() PluginIdentity {
	if p.identity.Key == "" {
		return PermissionIdentityForPath(p.path)
	}
	return p.identity
}

// NewWasmPlugin mirrors the real constructor: it still records the path, it
// just never gets to use it.
func NewWasmPlugin(path string) *WasmPlugin {
	return &WasmPlugin{path: path}
}

// IsWasmEntrypoint reports whether an entrypoint is a bare WebAssembly
// module. Detecting one costs nothing (it is a string check), so a lite
// build keeps recognising .wasm entrypoints -- newPluginForEntrypoint routes
// them to WasmPlugin.Init below rather than treating them as a native binary
// or leaving them for the RPC transport to mishandle.
func IsWasmEntrypoint(entrypoint string) bool {
	return isBareEntrypointWithExt(entrypoint, ".wasm")
}

func (p *WasmPlugin) GetName() string {
	return p.path + " (wasm)"
}

// Init always fails: a lite build carries no wasm runtime to load the module
// with.
func (p *WasmPlugin) Init(api vfs.HostAPI) error {
	return errWasmLiteUnavailable
}

// Close is a no-op: Init never got far enough to open anything.
func (p *WasmPlugin) Close() error {
	return nil
}
