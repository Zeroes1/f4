package app

import (
	"testing"

	"github.com/unxed/f4/internal/action"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/paneltest"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// TestMenuHonoursEnabled checks the menu half of the mechanism (f4#1356): an
// action with Enabled()==false stays in the menu -- unlike Visible, which
// removes the item -- but its vtui.MenuItem comes back Disabled, which is
// what dims the row and blocks its own activation inside vtui.
func TestMenuHonoursEnabled(t *testing.T) {
	preserveActionRegistry(t)
	enabled := true
	action.RegisterAction(action.Action{
		Name:     "Test.Enabled.MenuItem",
		Area:     "Shell",
		Label:    "Sometimes Enabled",
		MenuPath: "Commands",
		Enabled:  func() bool { return enabled },
		Handler:  func() bool { return true },
	})

	findDisabled := func() (found, disabled bool) {
		for _, m := range BuildMenuBarItems("Shell") {
			for _, it := range m.SubItems {
				if plainMenuText(it.Text) == "Sometimes Enabled" {
					return true, it.Disabled
				}
			}
		}
		return false, false
	}

	found, disabled := findDisabled()
	if !found {
		t.Fatal("an enabled action's menu item is missing")
	}
	if disabled {
		t.Error("an enabled action's menu item must not be Disabled")
	}

	enabled = false
	found, disabled = findDisabled()
	if !found {
		t.Fatal("a disabled action's menu item must stay in the menu, only dimmed")
	}
	if !disabled {
		t.Error("a disabled action's menu item must come back Disabled")
	}
}

// TestRunActionHonoursEnabled checks the hotkey/activation half of the
// mechanism: RunAction refuses to call Handler at all once Enabled() is
// false, regardless of who called it (menu OnClick, a resolved hotkey, the
// key bar) -- this is what replaces the old silent no-op with a command that
// visibly can't be invoked.
func TestRunActionHonoursEnabled(t *testing.T) {
	preserveActionRegistry(t)
	enabled := true
	called := false
	action.RegisterAction(action.Action{
		Name:    "Test.Enabled.RunAction",
		Area:    "Shell",
		Label:   "Test Run",
		Enabled: func() bool { return enabled },
		Handler: func() bool { called = true; return true },
	})

	if !RunAction("Test.Enabled.RunAction") {
		t.Error("an enabled action should run")
	}
	if !called {
		t.Error("an enabled action's Handler should have run")
	}

	called = false
	enabled = false
	if RunAction("Test.Enabled.RunAction") {
		t.Error("a disabled action must not report success")
	}
	if called {
		t.Error("a disabled action's Handler must never run")
	}
}

// TestRunActionWithoutEnabledIsUnaffected makes sure the vast majority of
// actions, which set no Enabled at all, keep running exactly as before.
func TestRunActionWithoutEnabledIsUnaffected(t *testing.T) {
	preserveActionRegistry(t)
	called := false
	action.RegisterAction(action.Action{
		Name:    "Test.Enabled.Unset",
		Area:    "Shell",
		Label:   "No Enabled Hook",
		Handler: func() bool { called = true; return true },
	})

	if !RunAction("Test.Enabled.Unset") {
		t.Error("an action with no Enabled predicate should run")
	}
	if !called {
		t.Error("an action with no Enabled predicate should have its Handler run")
	}
}

// TestActivePanelHasSelectionTarget checks the condition function wired to
// File.Attributes (Ctrl+A), File.Copy (F5), File.Move (F6), File.Delete (F8)
// and File.DeletePermanent (Shift+Del): it must mirror
// FileSystemPanel.GetSelectedNames exactly, since that is the same rule
// actionFileAttributes, actionCopyMove and actionDeleteWithDisposition use to
// decide whether they have anything to act on (f4#1356's silent no-op).
func TestActivePanelHasSelectionTarget(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := paneltest.SetupMockPanelsFrame(t)
	defer pf.Close()
	pf.ResizeConsole(80, 25)
	vtui.FrameManager.Push(pf)

	fsp := pf.GetActivePanel()
	fsp.Vfs = vfs.NewOSVFS(t.TempDir())
	fsp.Entries = []*panel.FileEntry{
		{VFSItem: vfs.VFSItem{Name: "..", IsDir: true}},
		{VFSItem: vfs.VFSItem{Name: "file.txt"}},
	}

	fsp.SetCursorIndex(0)
	if activePanelHasSelectionTarget() {
		t.Error("cursor on \"..\" with nothing marked should have no target")
	}

	fsp.SetCursorIndex(1)
	if !activePanelHasSelectionTarget() {
		t.Error("cursor on a real entry should be a target")
	}

	fsp.SetCursorIndex(0)
	fsp.Entries[1].Selected = true
	if !activePanelHasSelectionTarget() {
		t.Error("an explicitly marked entry should count as a target even with the cursor back on \"..\"")
	}
}

// TestActivePanelHasSelectionTarget_NoPanelsFrame makes sure the predicate
// degrades to "no target" rather than panicking when asked outside any
// panels frame (e.g. a full-screen editor/viewer with no panel behind it in
// this workspace).
func TestActivePanelHasSelectionTarget_NoPanelsFrame(t *testing.T) {
	t.Cleanup(paneltest.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	if activePanelHasSelectionTarget() {
		t.Error("with no panels frame at all, there is no target")
	}
}
