package observer

// This file is f4#1563's first vfs.VFSProvider slice: a provider that opens
// exactly one already end-to-end-proven Observer module (isoimg, see
// isoimg_e2e_test.go) against exactly one family of container (ISO9660
// images) so that Enter on a .iso in a panel has something behind it at
// all. The ticket's own design (see the comment thread on f4#1563 and
// status/1563.md in the accounting repository) sketches a much larger
// provider -- observer.ini-driven module selection by mask/signature,
// cancellation beyond ctx, PlugRing distribution -- all of that is
// deliberately left for later, atomic parts. Password retry for
// SOR_PASSWORD_REQUIRED landed in part 7 (see password.go); CanOpen below
// treats it the same as SOR_SUCCESS -- recognized, just locked -- so Enter
// still reaches Open, which is where the user is actually asked, not this
// cheap probe. Part 8 closed the ArchiveEnterExcludeMask-shaped gap the
// design also named: PanelEnterAllowed below lets ObserverEnterExcludeMask
// hold ordinary Enter back from a recognized container while Ctrl+PgDn keeps
// opening it, the way plugins/archive already does. What is here is real, not a
// stub: a genuine unmodified isoimg.wasm (built the way
// scripts/build_isoimg_test_wasm.sh already does for the existing
// plugins/observer tests) opens a real ISO image and the resulting tree is
// browsable, which is the whole point of this part.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/filemask"
	"github.com/unxed/f4/vfs"
)

// Provider also implements vfs.PanelEnterPolicyProvider (see
// PanelEnterAllowed below), the same optional capability
// plugins/multiarc.Provider and plugins/archive.ArchiveProvider assert for
// themselves.
var _ vfs.PanelEnterPolicyProvider = (*Provider)(nil)

// isoimgModuleFileName is the file Provider looks for in its modules
// directory. Observer modules are never embedded in the f4 binary (see
// doc.go and the licensing discussion in f4#1563): a user, or PlugRing once
// it grows a "modules Observer" category, drops a compiled .wasm there
// themselves. A missing file simply means the format is not available yet,
// exactly the way plugins/archive silently ignores a format none of its
// linked libraries understand.
const isoimgModuleFileName = "isoimg.wasm"

// isoExtensions gates the (expensive: a wasm instantiation plus a real
// OpenStorage round trip, see runtime.go) module probe behind a cheap name
// check, the same role plugins/multiarc's detectFormat and
// plugins/archive's extension detector play before either pays for a real
// read. It is not a claim about what isoimg.cpp itself parses -- OpenStorage
// returning SORInvalidFile is still the real, final answer either way --
// only a narrow first cut matching what this part has an actual test
// fixture for. Widening it (NRG, MDF, BIN/CUE) is later work once each has
// one too.
var isoExtensions = []string{".iso"}

// Provider is the vfs.VFSProvider this package registers (see Plugin.Init
// in plugin.go). See the package-level comment above for how narrow it is
// on purpose.
type Provider struct {
	modulesDir string

	mu      sync.Mutex
	isoimg  []byte
	scanned bool
}

// NewProvider builds a Provider that looks for isoimg.wasm under
// modulesDir.
func NewProvider(modulesDir string) *Provider {
	return &Provider{modulesDir: modulesDir}
}

func (p *Provider) Name() string { return "observer" }

// Priority mirrors plugins/archive.ArchiveProvider's own comment (higher
// polled sooner, archives usually low): actual polling order still follows
// registration order (vfs.FindProvider walks providerRegistry.items in the
// order RegisterProvider saw them, see vfs/vfs.go), and
// internal/plughost/manager.go registers this package after
// plugins/archive/plugins/multiarc for exactly that reason, matching the
// ticket's own "register after archive" requirement. Priority is set lower
// than ArchiveProvider's 10 only to document that intent for whenever
// vfs.RegisterProvider starts actually sorting by it.
func (p *Provider) Priority() int { return 5 }

// isoimgBytes lazily reads and caches modulesDir/isoimg.wasm. A read failure
// (most commonly: not installed) is cached as "no module" rather than
// retried on every CanOpen -- the same one-shot cost LoadModule's own
// sharedCompilationCache accepts for a compiled module, here for the raw
// file bytes instead.
func (p *Provider) isoimgBytes() []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.scanned {
		return p.isoimg
	}
	p.scanned = true
	b, err := os.ReadFile(filepath.Join(p.modulesDir, isoimgModuleFileName))
	if err != nil {
		return nil
	}
	p.isoimg = b
	return p.isoimg
}

func hasIsoExtension(name string) bool {
	lower := strings.ToLower(name)
	for _, ext := range isoExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// baseName returns parent.Base(path), falling back to path itself for a nil
// parent -- defensive the way plugins/archive.ArchiveProvider.CanOpen is,
// since a real caller always supplies one.
func baseName(parent vfs.VFS, path string) string {
	if parent == nil {
		return path
	}
	if base := parent.Base(path); base != "" {
		return base
	}
	return path
}

// PanelEnterAllowed is ArchiveProvider.PanelEnterAllowed's counterpart for
// Observer containers (f4#1563): ObserverEnterExcludeMask names files Enter
// must leave to their extension association even though a Provider module
// recognizes them, and Ctrl+PgDn keeps opening them regardless, the same
// deliberate escape hatch plugins/archive gives self-extracting archives and
// masked documents. Unlike ArchiveProvider there is no self-extracting-exe
// case to also guard here: isoimg, the only module Provider drives so far
// (see the package comment), never is itself a program Enter would
// otherwise run, so the mask is the whole policy.
func (p *Provider) PanelEnterAllowed(ctx context.Context, parent vfs.VFS, path string) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if _, isLocal := parent.(*vfs.OSVFS); !isLocal {
		// Off the local file system there is no association and no system
		// opener for Enter to be held back in favour of, exactly the
		// reasoning ArchiveProvider.PanelEnterAllowed uses for the same
		// case.
		return true
	}
	return !observerEnterBarredByMask(baseName(parent, path))
}

// observerEnterBarredByMask reports whether the configured mask claims this
// name for its association, mirroring plugins/archive.enterBarredByMask.
// An empty mask (the default -- see config.ObserverEnterExcludeMask) bars
// nothing, which is the setting a user writes when they want Enter to follow
// the content and only the content.
func observerEnterBarredByMask(name string) bool {
	mask := strings.TrimSpace(config.App.ObserverEnterExcludeMask)
	if mask == "" {
		return false
	}
	return filemask.Match(name, mask, true)
}

func (p *Provider) CanOpen(ctx context.Context, parent vfs.VFS, path string) bool {
	if ctx != nil && ctx.Err() != nil {
		return false
	}
	if parent == nil {
		return false
	}
	if !hasIsoExtension(baseName(parent, path)) {
		return false
	}
	wasmBytes := p.isoimgBytes()
	if wasmBytes == nil {
		return false
	}
	ok, err := probeIsoimg(ctx, parent, path, wasmBytes)
	return err == nil && ok
}

// probeIsoimg drives just enough of the ABI (LoadModule, LoadSubModule,
// OpenStorage) to answer "does isoimg recognize this file", then tears
// everything down -- CanOpen has no tree to keep and no ExtractItem to make,
// so it does not pay for WithExtractDir the way newObserverVFS's real Open
// does. It never passes a password: SOR_PASSWORD_REQUIRED counts as
// recognized here too (see below), so CanOpen stays a cheap, non-interactive
// probe and never itself pops the password dialog -- newObserverVFS's
// openStorageWithPasswordPrompt (password.go) is what actually asks, once
// Open is called.
func probeIsoimg(ctx context.Context, parent vfs.VFS, path string, wasmBytes []byte) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ra, err := parent.Open(ctx, path)
	if err != nil {
		return false, err
	}
	defer func() { _ = ra.Close() }()

	guestName := baseName(parent, path)
	if guestName == "" {
		guestName = "target"
	}

	modCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	mount := NewSingleFileFS(modCtx, guestName, ra)
	mod, err := LoadModule(modCtx, wasmBytes, mount, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = mod.Close() }()

	if _, err := mod.LoadSubModule(""); err != nil {
		return false, err
	}
	res, err := mod.OpenStorage(StorageOpenParams{FilePath: "/" + guestName})
	if err != nil {
		return false, err
	}
	switch res.Code {
	case SORSuccess:
		_ = mod.CloseStorage(res.Storage)
		return true, nil
	case SORPasswordRequired:
		// Recognized, just locked -- there is no open storage handle to
		// close here (OpenStorage never got past the password check), and
		// nothing to ask the user: that is Open's job, not this probe's.
		return true, nil
	default:
		return false, nil
	}
}

// Open builds an ObserverVFS rooted on path, driving the full sequence
// (LoadModule with a real extract directory this time, LoadSubModule,
// OpenStorage, then a complete GetItem walk to build the browsable tree --
// see vfs.go).
func (p *Provider) Open(ctx context.Context, parent vfs.VFS, path string) (vfs.VFS, error) {
	wasmBytes := p.isoimgBytes()
	if wasmBytes == nil {
		return nil, fmt.Errorf("observer: %s is not installed in %s", isoimgModuleFileName, p.modulesDir)
	}
	v, err := newObserverVFS(ctx, parent, path, wasmBytes, "")
	if err != nil {
		return nil, err
	}
	return v, nil
}
