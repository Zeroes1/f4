package panel

import (
	"testing"

	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

func TestShowGroupMenuActions(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	fp := groupTestPanel(t)
	fp.GroupBy = GroupName
	fp.GroupFoldersSeparately = false
	fp.X1, fp.X2 = 0, 79
	oldRunAction := RunAction
	groupSettingsOpened := false
	RunAction = func(name string) bool {
		if name == "Panel.GroupSettings" {
			groupSettingsOpened = true
		}
		return true
	}
	t.Cleanup(func() { RunAction = oldRunAction })

	fp.ShowGroupMenu()
	menu, ok := vtui.FrameManager.GetTopFrame().(*vtui.VMenu)
	if !ok {
		t.Fatalf("top frame = %T, want grouping menu", vtui.FrameManager.GetTopFrame())
	}
	if got, want := len(menu.Items), len(GroupModes)+3; got != want {
		t.Fatalf("menu items = %d, want %d", got, want)
	}

	menu.OnAction(0)
	if fp.GroupBy != GroupModes[0].Mode || fp.GroupReverse || fp.GroupFoldersSeparately {
		t.Fatalf("group mode action = mode %v reverse=%t folders=%t", fp.GroupBy, fp.GroupReverse, fp.GroupFoldersSeparately)
	}
	menu.OnAction(len(GroupModes))
	if !fp.GroupReverse {
		t.Fatal("reverse action did not toggle reverse grouping")
	}
	menu.OnAction(len(GroupModes) + 1)
	if !fp.GroupFoldersSeparately {
		t.Fatal("folder-separation action did not toggle folder grouping")
	}
	menu.OnAction(len(GroupModes) + 2)
	if !groupSettingsOpened {
		t.Fatal("settings action did not run Panel.GroupSettings")
	}
}

func TestShowGroupMenuWithoutFrameManagerIsInert(t *testing.T) {
	old := vtui.FrameManager
	vtui.FrameManager = nil
	t.Cleanup(func() { vtui.FrameManager = old })
	(&FileSystemPanel{}).ShowGroupMenu()
}
