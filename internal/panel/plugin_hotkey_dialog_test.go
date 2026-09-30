package panel

import (
	"path/filepath"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/keymap"
)

func newDialogTestManager(t *testing.T) *keymap.HotkeyManager {
	t.Helper()
	return &keymap.HotkeyManager{
		Bindings: map[string]map[string]string{"Shell": {"Del": "DeleteFiles"}},
		Defaults: map[string]map[string]string{"Shell": {"Del": "DeleteFiles"}},
		IniPath:  filepath.Join(t.TempDir(), "hotkeys.ini"),
	}
}

func TestPluginHotkeyDialogChoice(t *testing.T) {
	old := config.App.PluginDefaultHotkeysOff
	t.Cleanup(func() { config.App.PluginDefaultHotkeysOff = old })
	config.App.PluginDefaultHotkeysOff = ""
	hm := newDialogTestManager(t)
	const action = "Plugin.Legacy.0"

	// A letter is assigned; the same letter again is no change.
	if changed, ok := applyPluginHotkeyChoice(hm, action, "", "q"); !ok || !changed || hm.Bindings["Shell"]["Q"] != action {
		t.Fatalf("assign q: changed=%v ok=%v bindings=%v", changed, ok, hm.Bindings["Shell"])
	}
	if changed, ok := applyPluginHotkeyChoice(hm, action, "", "Q"); !ok || changed {
		t.Fatalf("the same letter: changed=%v ok=%v, want no change", changed, ok)
	}
	if got := currentPluginHotkeyText(hm, action, ""); got != "Q" {
		t.Fatalf("field starts with %q, want Q", got)
	}

	// Anything that is not one letter or digit is refused and changes nothing.
	for _, bad := range []string{"F4", "ab", "-", "Del"} {
		if changed, ok := applyPluginHotkeyChoice(hm, action, "", bad); ok || changed {
			t.Errorf("%q: changed=%v ok=%v, want it refused", bad, changed, ok)
		}
	}
	if hm.Bindings["Shell"]["Q"] != action || hm.Bindings["Shell"]["Del"] != "DeleteFiles" {
		t.Fatalf("a refused choice moved the bindings: %v", hm.Bindings["Shell"])
	}

	// An empty field takes the assigned key back.
	if changed, ok := applyPluginHotkeyChoice(hm, action, "", ""); !ok || !changed {
		t.Fatalf("empty field: changed=%v ok=%v", changed, ok)
	}
	if _, still := hm.Bindings["Shell"]["Q"]; still {
		t.Fatal("the assigned hotkey survived an empty field")
	}
}

func TestPluginHotkeyDialogEmptyFieldSwitchesOffThePluginsDefault(t *testing.T) {
	old := config.App.PluginDefaultHotkeysOff
	t.Cleanup(func() { config.App.PluginDefaultHotkeysOff = old })
	config.App.PluginDefaultHotkeysOff = ""
	hm := newDialogTestManager(t)

	if got := currentPluginHotkeyText(hm, "Plugin.Legacy.0", "ShiftF1"); got != "" {
		t.Fatalf("a chord default does not fit a one-character field, field = %q", got)
	}
	if changed, ok := applyPluginHotkeyChoice(hm, "Plugin.Legacy.0", "ShiftF1", ""); !ok || !changed {
		t.Fatalf("empty field over a default: changed=%v ok=%v", changed, ok)
	}
	if !PluginDefaultKeyOff("ShiftF1") {
		t.Fatal("the default was not switched off")
	}
	if changed, _ := applyPluginHotkeyChoice(hm, "Plugin.Legacy.0", "ShiftF1", ""); changed {
		t.Fatal("removing an already removed default must not report a change")
	}
}
