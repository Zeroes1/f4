package plughost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestNewPluginManagerInitializesOpenManager(t *testing.T) {
	api := newLuaTestHostAPI()
	manager := NewPluginManager(api)

	if manager.api == nil {
		t.Fatal("NewPluginManager did not retain the host API")
	}
	if manager.closed || len(manager.plugins) != 0 || manager.externalLoader != nil {
		t.Fatalf("new manager state = %#v", manager)
	}
}

func TestPluginManagerLoadExternalPluginDropsFailedPlugin(t *testing.T) {
	manager := NewPluginManager(newLuaTestHostAPI())
	manager.LoadExternalPlugin(" \t")

	if len(manager.plugins) != 0 {
		t.Fatalf("failed external plugin was retained: %v", manager.plugins)
	}
}

func TestPluginManagerLoadSinglePlugRingItemRejectsEmptyEntrypoint(t *testing.T) {
	manager := NewPluginManager(newLuaTestHostAPI())
	if err := manager.LoadSinglePlugRingItem(PlugRingItem{}); err == nil {
		t.Fatal("empty entrypoint was accepted")
	}

	if len(manager.plugins) != 0 {
		t.Fatalf("empty PlugRing item was retained: %v", manager.plugins)
	}
}

func TestPluginManagerLoadSinglePlugRingItemReturnsLoadError(t *testing.T) {
	manager := NewPluginManager(newLuaTestHostAPI())
	err := manager.LoadSinglePlugRingItem(PlugRingItem{ID: "broken", Entrypoint: "missing-plugin.lua"})
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Fatalf("load error = %v, want the failing plugin's ID", err)
	}
	if len(manager.plugins) != 0 {
		t.Fatal("failed plugin was retained")
	}
}

func TestPluginManagerKeepsBuiltinInsteadOfInstalledCopy(t *testing.T) {
	if !BuiltinPlugRingItem("android") {
		t.Skip("lite loads external plugins instead")
	}
	previous := config.UserConfigDir
	profile := t.TempDir()
	config.UserConfigDir = func() (string, error) { return profile, nil }
	config.ResetConfigDirForTest()
	t.Cleanup(func() {
		config.UserConfigDir = previous
		config.ResetConfigDirForTest()
	})
	dir := filepath.Join(config.GetF4ConfigDir(), "plugring", "android")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(`{"id":"android","entrypoint":"plugin.lua"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plugin.lua"), []byte(`local rpc = require('f4rpc'); rpc.register('Plugin.Init', function() return {'Android'} end)`), 0600); err != nil {
		t.Fatal(err)
	}
	manager := NewPluginManager(newLuaTestHostAPI())
	t.Cleanup(manager.CloseAll)
	manager.loadPlugRing()
	if len(manager.Names()) != 0 {
		t.Fatalf("full build loaded an external replacement: %v", manager.Names())
	}
	if err := manager.LoadSinglePlugRingItem(PlugRingItem{ID: "android", Entrypoint: "plugin.lua"}); err == nil {
		t.Fatal("hot-load accepted an external replacement of a built-in plugin")
	}
}
