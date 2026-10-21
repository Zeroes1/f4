package panel

import (
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// A drop on the command line is not a file operation either: the dropped
// paths are typed into it, each quoted the way Ctrl+F quotes the current
// file, for the command being written (#2049). Nothing is copied or moved,
// and the source is told so with copy: a move would have it delete the
// originals.

// cmdLineDropTarget answers a drag over the command line. handled is false
// when the command line is not on screen or the pointer is not on it.
func (pf *PanelsFrame) cmdLineDropTarget(ev *vtui.DragEvent) (action vtui.DropAction, handled bool) {
	cl := pf.CmdLine
	if cl == nil || !cl.IsVisible() {
		return vtui.DropNone, false
	}
	x1, y1, x2, y2 := cl.GetPosition()
	if ev.X < x1 || ev.X > x2 || ev.Y < y1 || ev.Y > y2 {
		return vtui.DropNone, false
	}
	action = cmdLineDropAction(ev.Allowed)
	if ev.Phase != vtui.DragDrop {
		pf.showDropHighlight(dropTargetInfo{}, false, ev.Tracked && action != vtui.DropNone)
		return action, true
	}
	if action == vtui.DropNone {
		return action, true
	}
	vtui.DebugLog("DND: %d path(s) dropped on the command line", len(ev.Payload.Paths))
	pf.insertDroppedPaths(ev.Payload.Paths)
	return action, true
}

// cmdLineDropAction is what the source is told a drop on the command line
// does: copy, or link where the source does not offer copy. Never move.
func cmdLineDropAction(allowed vtui.DropAction) vtui.DropAction {
	switch {
	case allowed.Has(vtui.DropCopy):
		return vtui.DropCopy
	case allowed.Has(vtui.DropLink):
		return vtui.DropLink
	}
	return vtui.DropNone
}

// insertDroppedPaths types the dropped paths into the command line at the
// cursor, separated by spaces, and gives the command line the focus where
// the panels may hold it (search-first navigation).
func (pf *PanelsFrame) insertDroppedPaths(paths []string) {
	inserted := false
	for _, raw := range paths {
		p := NormalizeExternalDropPath(raw)
		if p == "" {
			continue
		}
		pf.InsertPathToCmdLine(p)
		inserted = true
	}
	if !inserted {
		return
	}
	pf.SetCommandLineFocus(true)
	vtui.FrameManager.Redraw()
}

// drawCmdLineDropHighlight shows the command line as the drop target, in the
// cursor's colours, as a folder row is shown.
func (pf *PanelsFrame) drawCmdLineDropHighlight(scr *vtui.ScreenBuf) {
	if !pf.cmdLineDrop || pf.CmdLine == nil || !pf.CmdLine.IsVisible() {
		return
	}
	x1, y1, x2, y2 := pf.CmdLine.GetPosition()
	scr.ApplyColor(x1, y1, x2, y2, vtui.Palette[theme.ColPanelCursor])
}
