//go:build !extralite

package plughost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/unxed/f4/internal/config"
)

const loadableLuaPlugin = `
	local f4rpc = require('f4rpc')

	f4rpc.register("Plugin.Init", function()
		return { Drives = { "Cov Loaded Drive" } }
	end)

	f4rpc.serve()
`

// brokenLuaPlugin does not even parse, so loading it fails in Init.
const brokenLuaPlugin = `this is not a Lua script`

// isolatePlugRingDir points the profile directory, and with it PlugRing's
// install directory and the permission store, at a fresh temporary directory
// for one test. It returns the profile directory.
func isolatePlugRingDir(t *testing.T) string {
	t.Helper()
	// Both are resolved lazily and then cached; resolve them before the
	// swap so that the temporary directory is not pinned for later tests.
	_ = config.GetF4ConfigDir()
	_ = PluginPermissions()
	oldConfigDir := config.CachedF4ConfigDir
	oldPermissionStore := pluginPermissionStore
	dir := t.TempDir()
	config.CachedF4ConfigDir = dir
	pluginPermissionStore = LoadPermissionStore(DefaultPermissionStorePath())
	t.Cleanup(func() {
		config.CachedF4ConfigDir = oldConfigDir
		pluginPermissionStore = oldPermissionStore
	})
	return dir
}

// installPlugRingPlugin lays a plugin out the way PlugRing installs one:
// <profile>/plugring/<id>/{manifest.json,main.lua}. A nil item writes no
// manifest.
func installPlugRingPlugin(t *testing.T, profile, id, source string, item *PlugRingItem) string {
	t.Helper()
	pluginDir := filepath.Join(profile, "plugring", id)
	// #nosec G703 -- pluginDir is inside the test's temporary profile directory.
	if err := os.MkdirAll(pluginDir, 0o750); err != nil {
		t.Fatal(err)
	}
	// #nosec G703 -- pluginDir is inside the test's temporary profile directory.
	if err := os.WriteFile(filepath.Join(pluginDir, "main.lua"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if item != nil {
		manifest, err := json.Marshal(item)
		if err != nil {
			t.Fatal(err)
		}
		// #nosec G703 -- pluginDir is inside the test's temporary profile directory.
		if err := os.WriteFile(filepath.Join(pluginDir, "manifest.json"), manifest, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return pluginDir
}

func sortedPluginNames(pm *PluginManager) []string {
	names := pm.Names()
	sort.Strings(names)
	return names
}

func TestPluginManagerLoadExternalPluginKeepsAWorkingLuaPlugin(t *testing.T) {
	api := newLuaTestHostAPI()
	pm := NewPluginManager(api)
	t.Cleanup(pm.CloseAll)

	path := writeLuaPlugin(t, loadableLuaPlugin)
	pm.LoadExternalPlugin(path)

	if names := pm.Names(); len(names) != 1 || names[0] != path+" (Lua)" {
		t.Fatalf("loaded plugins = %q, want the Lua plugin by its path", names)
	}
	if _, ok := api.drives["Cov Loaded Drive"]; !ok {
		t.Fatalf("the plugin's drive was not registered: %v", api.drives)
	}

	pm.LoadExternalPlugin(writeLuaPlugin(t, brokenLuaPlugin))
	if len(pm.Names()) != 1 {
		t.Fatalf("a plugin that failed to initialise was kept: %q", pm.Names())
	}
}

func TestPluginManagerLoadSinglePlugRingItemLoadsFromTheInstallDirectory(t *testing.T) {
	profile := isolatePlugRingDir(t)
	api := newLuaTestHostAPI()
	pm := NewPluginManager(api)
	t.Cleanup(pm.CloseAll)

	pluginDir := installPlugRingPlugin(t, profile, "cov-hot", loadableLuaPlugin, nil)
	pm.LoadSinglePlugRingItem(PlugRingItem{ID: "cov-hot", Entrypoint: "main.lua"})

	want := filepath.Join(pluginDir, "main.lua") + " (Lua)"
	if names := pm.Names(); len(names) != 1 || names[0] != want {
		t.Fatalf("hot-loaded plugins = %q, want [%s]", names, want)
	}
	if _, ok := api.drives["Cov Loaded Drive"]; !ok {
		t.Fatalf("the hot-loaded plugin's drive was not registered: %v", api.drives)
	}

	installPlugRingPlugin(t, profile, "cov-broken", brokenLuaPlugin, nil)
	pm.LoadSinglePlugRingItem(PlugRingItem{ID: "cov-broken", Entrypoint: "main.lua"})
	if len(pm.Names()) != 1 {
		t.Fatalf("a PlugRing plugin that failed to initialise was kept: %q", pm.Names())
	}

	// An item whose files were never installed fails the same way.
	pm.LoadSinglePlugRingItem(PlugRingItem{ID: "cov-absent", Entrypoint: "main.lua"})
	if len(pm.Names()) != 1 {
		t.Fatalf("a PlugRing item without files was kept: %q", pm.Names())
	}
}

func TestPluginManagerLoadExternalLoadsRegisteredAndInstalledPlugins(t *testing.T) {
	profile := isolatePlugRingDir(t)
	registered := writeLuaPlugin(t, loadableLuaPlugin)
	missing := filepath.Join(t.TempDir(), "missing.lua")

	oldRegistered := config.App.RegisteredPlugins
	config.App.RegisteredPlugins = []string{registered, missing}
	t.Cleanup(func() { config.App.RegisteredPlugins = oldRegistered })

	good := installPlugRingPlugin(t, profile, "cov-ring-good", loadableLuaPlugin,
		&PlugRingItem{ID: "cov-ring-good", Entrypoint: "main.lua"})
	// Installed, but with nothing to start, and installed but broken: neither
	// may take the others down.
	installPlugRingPlugin(t, profile, "cov-ring-noentry", loadableLuaPlugin,
		&PlugRingItem{ID: "cov-ring-noentry"})
	installPlugRingPlugin(t, profile, "cov-ring-broken", brokenLuaPlugin,
		&PlugRingItem{ID: "cov-ring-broken", Entrypoint: "main.lua"})
	// A directory without a manifest and a stray file are not plugins.
	installPlugRingPlugin(t, profile, "cov-ring-nomanifest", loadableLuaPlugin, nil)
	// #nosec G703 -- profile is the test's temporary directory.
	if err := os.WriteFile(filepath.Join(profile, "plugring", "stray.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	api := newLuaTestHostAPI()
	pm := NewPluginManager(api)
	t.Cleanup(pm.CloseAll)

	pm.LoadExternal()

	want := []string{registered + " (Lua)", filepath.Join(good, "main.lua") + " (Lua)"}
	sort.Strings(want)
	got := sortedPluginNames(pm)
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("loaded plugins = %q, want %q", got, want)
	}

	// The external phase runs once however many times startup asks for it.
	pm.LoadExternal()
	if again := sortedPluginNames(pm); len(again) != len(want) {
		t.Fatalf("a second LoadExternal loaded plugins again: %q", again)
	}
}
