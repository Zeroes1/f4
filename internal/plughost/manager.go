package plughost

import (
	"path/filepath"
	"sync"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/plugins/chroma"
	dotnetplugin "github.com/unxed/f4/plugins/dotnet"
	"github.com/unxed/f4/plugins/dummy_internal"
	"github.com/unxed/f4/plugins/envman"
	gitplugin "github.com/unxed/f4/plugins/git"
	"github.com/unxed/f4/plugins/id3editor"
	"github.com/unxed/f4/plugins/ide"
	"github.com/unxed/f4/plugins/intchecker"
	"github.com/unxed/f4/plugins/mediainfo"
	observerplugin "github.com/unxed/f4/plugins/observer"
	pdfviewplugin "github.com/unxed/f4/plugins/pdfview"
	"github.com/unxed/f4/plugins/proclist"
	sqliteplugin "github.com/unxed/f4/plugins/sqlite"
	"github.com/unxed/f4/plugins/svcmgr"
	"github.com/unxed/f4/plugins/visren"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// Plugin represents a loaded module.
type Plugin interface {
	Init(api vfs.HostAPI) error
	Close() error
	GetName() string
}
type PluginManager struct {
	mu           sync.Mutex
	api          vfs.HostAPI
	plugins      []Plugin
	closed       bool
	internalOnce sync.Once
	externalOnce sync.Once
	// externalLoader is a test seam for the startup phase. Production managers
	// leave it nil and use the configured/PlugRing discovery below.
	externalLoader func()
}

var GlobalPluginManager *PluginManager

func NewPluginManager(api vfs.HostAPI) *PluginManager {
	return &PluginManager{api: api}
}

func (pm *PluginManager) LoadAll() {
	vtui.DebugLog("--- Loading Plugins ---")
	pm.LoadInternal()
	pm.LoadExternal()
}

// LoadInternal initializes built-in plugins synchronously. Their Init methods
// only register local capabilities, so panels can restore URI-backed sessions
// without waiting for external plugin discovery or subprocess startup.
func (pm *PluginManager) LoadInternal() {
	pm.internalOnce.Do(pm.loadInternal)
}

// LoadExternal loads configured and PlugRing plugins. Startup calls this only
// after the initial desktop and panels have been constructed: external plugin
// initialization may ask for permissions or synchronously call back into the
// UI, neither of which can complete before FrameManager.Run starts consuming
// posted tasks.
func (pm *PluginManager) LoadExternal() {
	pm.externalOnce.Do(func() {
		if pm.externalLoader != nil {
			pm.externalLoader()
			return
		}
		for _, path := range config.App.RegisteredPlugins {
			pm.LoadExternalPlugin(path)
		}
		pm.loadPlugRing()
	})
}

// StartExternal schedules the potentially interactive external phase without
// holding up startup. Built-in plugins are loaded separately and synchronously
// so their URI providers are already present when the saved panel paths are
// restored.
func (pm *PluginManager) StartExternal() {
	if pm == nil {
		return
	}
	pm.mu.Lock()
	closed := pm.closed
	pm.mu.Unlock()
	if closed {
		return
	}
	go pm.LoadExternal()
}

func (pm *PluginManager) keepPlugin(p Plugin) bool {
	pm.mu.Lock()
	if pm.closed {
		pm.mu.Unlock()
		_ = p.Close()
		return false
	}
	pm.plugins = append(pm.plugins, p)
	pm.mu.Unlock()
	return true
}

func (pm *PluginManager) LoadExternalPlugin(path string) {
	p := newPluginForEntrypoint("", path)
	if err := p.Init(pm.api); err == nil {
		if pm.keepPlugin(p) {
			vtui.DebugLog("Loaded plugin: %s", p.GetName())
		}
	} else {
		vtui.DebugLog("Failed plugin %s: %v", path, err)
	}
}
func (pm *PluginManager) loadPlugRing() {
	installed := GetInstalledPlugRingItems()
	plugringDir := filepath.Join(config.GetF4ConfigDir(), "plugring")
	for id, item := range installed {
		if item.Entrypoint != "" {
			// We build a pseudo-path that NewRPCPlugin will handle specifically later if needed,
			// but since NewRPCPlugin uses exec.Command directly, we pass the command.
			// The RPCPlugin execution logic will need to handle splitting by spaces if it's a shell command.
			p := newPluginForPlugRingItem(filepath.Join(plugringDir, id), item)
			if err := p.Init(pm.api); err == nil {
				if pm.keepPlugin(p) {
					vtui.DebugLog("Loaded PlugRing RPC plugin: %s", p.GetName())
				}
			} else {
				vtui.DebugLog("Failed PlugRing RPC plugin %s: %v", id, err)
			}
		}
	}
}
func (pm *PluginManager) LoadSinglePlugRingItem(item PlugRingItem) {
	if item.Entrypoint == "" {
		return
	}
	plugringDir := filepath.Join(config.GetF4ConfigDir(), "plugring")
	pluginDir := filepath.Join(plugringDir, item.ID)

	p := newPluginForPlugRingItem(pluginDir, item)
	if err := p.Init(pm.api); err == nil {
		if pm.keepPlugin(p) {
			vtui.DebugLog("Hot-loaded PlugRing RPC plugin: %s", p.GetName())
		}
	} else {
		vtui.DebugLog("Failed to hot-load PlugRing RPC plugin %s: %v", item.ID, err)
	}
}

func (pm *PluginManager) loadInternal() {
	plugins := []Plugin{
		&chroma.Plugin{},
		&dummy_internal.InternalDummyPlugin{},
		&visren.Plugin{},
		&id3editor.ID3EditorPlugin{},
		// Integrity checker (f4#1623 step 1): checksum file generation.
		intchecker.NewPlugin(config.GetF4ConfigDir()),
		envman.NewPlugin(config.GetF4ConfigDir()),
		mediainfo.NewPlugin(config.GetF4ConfigDir()),
		// Both builds: the lite build's SQLite client runs the host's
		// sqlite3 tool instead of linking the engine (plugins/sqlite's
		// backend_default_lite.go).
		sqliteplugin.NewPlugin(),
		proclist.NewPlugin(config.GetF4ConfigDir()),
		// Windows service list (f4#311 part 1): registers nothing off Windows.
		svcmgr.NewPlugin(),
		// Git status view (f4#659 part 1 of N): wraps the host's own `git`
		// binary, no platform gate at this layer -- gitplugin.Available()
		// (internal/app/git_actions.go's Visible check) covers "git is
		// missing from PATH" instead, the same plugin/action split
		// plugins/sqlite's CLI backend uses.
		gitplugin.NewPlugin(),
		// .NET assembly browser (f4#1666): Ctrl+PgDn on a .dll/.exe with .NET
		// metadata mounts its references, types and resources read-only.
		dotnetplugin.NewPlugin(),
		// PDF browser (f4#1665): Ctrl+PgDn on a .pdf mounts its text and pictures
		// read-only; F3 on a picture opens f4's image viewer.
		pdfviewplugin.NewPlugin(),
		// IDE mode (f4#382): scaffold only for now -- registration and the
		// three IDE.Build/Run/Test commands, no toolchain integration yet.
		// See plugins/ide's package doc for the full plan.
		ide.NewPlugin(),
	}
	// cloudfox (cloud services), android (ADB device browsing) and iOS
	// (Apple mobile devices over usbmuxd) stay excluded from both builds
	// entirely, in either build. All three moved out to their own module
	// and their own subprocess RPC plugin binary
	// (plugins/cloudfox/cmd/cloudfox-plugin,
	// plugins/android/cmd/android-plugin, plugins/ios/cmd/ios-plugin), per
	// the owner's decision to give every plugin that is not part of f4's
	// baseline feature set the same "download on demand" treatment
	// (f4#1178, https://github.com/unxed/f4/issues/1178#issuecomment-5851392645)
	// -- for android that is an architectural unification, not a binary-size
	// win the way cloudfox's ~30 MB of cloud SDKs or iOS's go-ios/gvisor/
	// quic-go userspace networking stack was; see
	// plugins/android/rpc_plugin.go's package comment. Archive support comes
	// back as plugins/multiarc, a CLI-archiver wrapper, in place of the
	// native-library one (f4#1178, part 2), and netfox comes back cut down
	// to FISH+ over a subprocess ssh dialer, in place of the full build's
	// FTP/SFTP/FISH+ trio (part 3) -- no ftp/sftp/scp VFS provider. See
	// plugins_lite.go/plugins_full.go, the single point of truth for which
	// build tag gets which set, for the full accounting.
	plugins = append(plugins, optionalVFSPlugins()...)

	// observerplugin registers after archive/multiarc (f4#1563's own design
	// requires this: "Регистрировать провайдер нужно после archive, чтобы
	// zip, 7z, rar и SFX оставались за ним"), in both build tags -- unlike
	// everything optionalVFSPlugins covers, an Observer module is a wazero
	// wasm reactor, the same dependency transport_wazero.go already keeps in
	// both builds for the generic wasm plugin transport, so there is no
	// lite/full split to make here at all.
	plugins = append(plugins, observerplugin.NewPlugin(config.GetF4ConfigDir()))

	for _, p := range plugins {
		if err := p.Init(pm.api); err == nil {
			if pm.keepPlugin(p) {
				vtui.DebugLog("Loaded internal plugin: %s", p.GetName())
			}
		} else {
			vtui.DebugLog("Failed to init internal plugin %T: %v", p, err)
		}
	}
}

func (pm *PluginManager) CloseAll() {
	pm.mu.Lock()
	pm.closed = true
	plugins := append([]Plugin(nil), pm.plugins...)
	pm.plugins = nil
	pm.mu.Unlock()
	// Close outside the manager lock so a plugin can finish callbacks without
	// deadlocking on registry or manager work. Reverse order mirrors startup.
	for i := len(plugins) - 1; i >= 0; i-- {
		if err := plugins[i].Close(); err != nil {
			vtui.DebugLog("Failed to close plugin %s: %v", plugins[i].GetName(), err)
		}
	}
}

// Names lists the loaded plugins by GetName, in load order; it is what
// f4:about reports. The lock is held only to copy the list, so a GetName
// that takes its time cannot hold up a plugin being loaded meanwhile.
func (pm *PluginManager) Names() []string {
	if pm == nil {
		return nil
	}
	pm.mu.Lock()
	plugins := append([]Plugin(nil), pm.plugins...)
	pm.mu.Unlock()
	names := make([]string, 0, len(plugins))
	for _, p := range plugins {
		names = append(names, p.GetName())
	}
	return names
}
