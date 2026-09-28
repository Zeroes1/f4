package app

import (
	"testing"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/f4/internal/macro"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// panelKeysTestController is a panel plugin that declares F8 through the
// shared vfs.PanelKeyProvider primitive and counts what reaches ProcessKey.
type panelKeysTestController struct {
	panelPluginTestController
	ran int
}

func (p *panelKeysTestController) PanelKeys() []vfs.PanelKey {
	return []vfs.PanelKey{{VK: vtinput.VK_F8, Label: "Kill", Run: func() { p.ran++ }}}
}

func (p *panelKeysTestController) ProcessKey(e *vtinput.InputEvent) bool {
	p.keys++
	return e != nil && e.VirtualKeyCode == vtinput.VK_F4
}

// TestPanelPluginOwnsItsKeysAndKeyBar covers the f4#312 primitive end to
// end through the real key router: a declared key runs ahead of the file
// panel's own binding for it, a File.* binding stands down and hands its key
// to the plugin, a window-level binding keeps its caption, and the keybar
// shows the declared caption instead of the file panel's.
func TestPanelPluginOwnsItsKeysAndKeyBar(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	screen := vtui.NewSilentScreenBuf()
	screen.AllocBuf(80, 25)
	vtui.FrameManager.Init(screen)
	pf := paneltest.SetupMockPanelsFrame(t)
	pf.ResizeConsole(80, 25)
	defer pf.Close()
	vtui.FrameManager.Push(pf)

	shell := map[string]string{"F1": "App.Help", "F4": "File.Edit", "F8": "File.Delete"}
	previousHotkeys, previousMacro := keymap.GlobalHotkeysMgr, macro.MacroMgr
	keymap.GlobalHotkeysMgr = &keymap.HotkeyManager{
		Defaults: map[string]map[string]string{"Shell": shell},
		Bindings: map[string]map[string]string{"Shell": shell},
	}
	manager := &macro.MacroManager{}
	macro.MacroMgr = manager
	t.Cleanup(func() {
		keymap.GlobalHotkeysMgr = previousHotkeys
		macro.MacroMgr = previousMacro
	})

	controller := &panelKeysTestController{}
	registration, err := (&coreAPI{}).RegisterPanelProvider(vfs.PanelProvider{
		ID:    "test.panel.keys",
		Title: "Keys panel",
		Open:  func(vfs.PanelContext) (vfs.PanelController, error) { return controller, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer registration.Unregister()

	panel.OpenRegisteredPanelProvider(pf, "test.panel.keys")
	instance, ok := pf.AltPanels[pf.ActiveIdx].(*panel.PluginPanelInstance)
	if !ok {
		t.Fatalf("active slot contains %T, want *panel.PluginPanelInstance", pf.AltPanels[pf.ActiveIdx])
	}
	instance.SetFocus(true)
	if !pf.PluginPanelFocused() {
		t.Fatal("PluginPanelFocused is false with a focused plugin panel in the active slot")
	}

	f8 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F8}
	if !macroFilter(manager, f8) || controller.ran != 1 {
		t.Fatalf("declared F8: ran=%d, want the plugin's key to run once ahead of File.Delete", controller.ran)
	}

	f4 := &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F4}
	if macroFilter(manager, f4) {
		t.Fatal("File.Edit on F4 was dispatched although a panel plugin owns the keyboard")
	}
	if !pf.ProcessKey(f4) || controller.keys == 0 {
		t.Fatal("F4 did not reach the plugin's ProcessKey after File.Edit stood down")
	}

	if !pf.PluginPanelStandsDown("File.Delete") || pf.PluginPanelStandsDown("App.Help") {
		t.Fatal("stand-down must cover File.* and nothing window-level")
	}

	labels := pf.GetKeyLabels()
	if labels.Normal[7] != "Kill" {
		t.Fatalf("F8 caption = %q, want the plugin's declared \"Kill\"", labels.Normal[7])
	}
	if labels.Normal[3] != "" {
		t.Fatalf("F4 caption = %q, want blank while File.Edit stands down", labels.Normal[3])
	}
	if labels.Normal[0] == "" {
		t.Fatal("F1 (App.Help) lost its caption under a panel plugin")
	}

	instance.Close()
	if pf.PluginPanelFocused() || pf.PluginPanelStandsDown("File.Delete") {
		t.Fatal("the stand-down outlived the plugin panel")
	}
}

func TestIsFilePanelScopedAction(t *testing.T) {
	for name, want := range map[string]bool{
		"File.Delete":             true,
		"file.edit":               true,
		"File.View:SomeCond":      true,
		"Panel.SelectGroup":       true,
		"Panel.InvertSelection":   true,
		"Panel.Toggle":            false,
		"Panel.InfoPanel":         false,
		"App.Help":                false,
		"Plugin.Command.anything": false,
	} {
		if got := panel.IsFilePanelScopedAction(name); got != want {
			t.Errorf("IsFilePanelScopedAction(%q) = %v, want %v", name, got, want)
		}
	}
}
