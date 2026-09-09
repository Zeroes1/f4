package panel

import "github.com/unxed/vtui"

func workspaceHasOpenPanels(screen *vtui.AppScreen) bool {
	if screen == nil {
		return false
	}
	for index := len(screen.Frames) - 1; index >= 0; index-- {
		if panels, ok := screen.Frames[index].(*PanelsFrame); ok && !panels.Closed {
			return true
		}
	}
	return false
}

func IsOnlyPanelsWorkspace(screen *vtui.AppScreen) bool {
	if !workspaceHasOpenPanels(screen) || vtui.FrameManager == nil || len(vtui.FrameManager.Screens) <= 1 {
		return false
	}
	panelsWorkspaces := 0
	for _, candidate := range vtui.FrameManager.Screens {
		if workspaceHasOpenPanels(candidate) {
			panelsWorkspaces++
		}
	}
	return panelsWorkspaces == 1
}
