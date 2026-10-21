package app

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/editor"
	"github.com/unxed/f4/internal/frameborder"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/vtui"
)

// The optional outer border comes in two widths. A dialog gets the far2l/Far3
// look asked for in f4#1399: three blank cells on the sides, one above and
// below. The menus -- the user menu, the drive chooser behind Alt+F1/Alt+F2,
// the F11 plugins menu of the panels and of the editor -- get the same ring
// with two blank cells on the sides and one above and below, narrower than a
// dialog's wide side wings, which read as a doubled box only on a dialog.
const (
	menuOuterBorderSide   = 2
	dialogOuterBorderSide = 3
)

// RenderDialogOuterBorder optionally reserves an empty border outside the
// border of a covered frame, matching the far2l/Far3 "outer border" look
// requested in f4#1399 (the reporter finds f4's current single-frame look,
// without that extra ring, too close to Dos Navigator's). The ring is blank
// space: a second box drawn on top of the frame's own border would read as a
// doubled frame, which is the look the reporter does not want. It is purely
// cosmetic and off by default.
//
// It runs from vtui's FrameManager.AfterFrameShow, right after the frame
// painted its own border -- the same extension point dialog.RenderHelpFrame
// already uses to add a hint on the Help window's border (see #378) -- so
// the extra ring stays in that frame's place in the paint stack: a frame
// above still covers it, and nothing outside vtui needs to change.
func RenderDialogOuterBorder(scr *vtui.ScreenBuf, frame vtui.Frame) {
	side := outerBorderSideWidth(frame)
	if !config.App.DialogOuterBorder || side == 0 {
		return
	}
	x1, y1, x2, y2 := frame.GetPosition()
	if x2 <= x1 || y2 <= y1 {
		return
	}
	// Sample the frame's own border so the empty ring reuses whatever
	// attribute the frame just drew there -- the normal dialog box colour,
	// a themed variant, or (for IsWarning dialogs) the warning box colour
	// -- instead of hard-coding a palette index. frameborder.Attr keeps the
	// probe on a border cell that is actually inside the buffer: a dialog
	// dragged past the left or the top edge has its top-left corner
	// off-screen, and GetCell answers an off-screen coordinate with a zero
	// cell, which would paint the whole ring in the wrong colour.
	attr, ok := frameborder.Attr(scr, x1, y1, x2, y2)
	if !ok {
		return
	}
	p := vtui.NewPainter(scr)
	// Four strips, not one enclosing rectangle: the latter would erase the
	// frame it decorates. The sides are side cells wide, the top and the
	// bottom one cell tall; each strip covers its corners, so together they
	// form an unbroken ring around the frame.
	p.Fill(x1-side, y1-1, x2+side, y1-1, ' ', attr)
	p.Fill(x1-side, y2+1, x2+side, y2+1, ' ', attr)
	p.Fill(x1-side, y1, x1-1, y2, ' ', attr)
	p.Fill(x2+1, y1, x2+side, y2, ' ', attr)

	// The frame manager lays a frame's shadow down before Show, in the very
	// cells this ring has just claimed, so the ring wipes it out. Lay it
	// again just outside the ring, with the shape vtui uses for a frame --
	// a bottom strip from two cells right of the left edge to two cells
	// right of the right one, one row below, and a two-cell wide right
	// column -- carried over to the ring's own outer rectangle.
	if frame.HasShadow() {
		scr.ApplyShadow(x1-side+2, y2+2, x2+side+2, y2+2)
		scr.ApplyShadow(x2+side+1, y1, x2+side+2, y2+1)
	}
}

// outerBorderSideWidth reports how many cells wide the optional outer border
// is on the sides of frame, and 0 when frame does not get one: ordinary
// popup/context menus and non-modal windows (editor, viewer, panels, ...) are
// never decorated.
//
// GetType and IsModal are vtui.Frame interface methods, so they resolve
// correctly through any number of embedding layers (several f4 dialogs wrap
// *vtui.Window in their own struct, e.g. dialog.FileDialog) without needing
// a concrete type assertion for every wrapper. The menus are the exceptions:
// each is a *vtui.VMenu under the hood whose GetType() reports TypeMenu the
// same as any other menu, so each needs its own concrete check -- against
// panel.UserMenuFrame, panel.DriveMenuFrame, editor.PluginsMenuFrame and the
// plugin-menu marker panel.IsPluginMenuFrame, the wrappers f4 pushes them as.
func outerBorderSideWidth(frame vtui.Frame) int {
	switch frame.(type) {
	case *panel.UserMenuFrame, *panel.DriveMenuFrame, *editor.PluginsMenuFrame:
		return menuOuterBorderSide
	}
	if panel.IsPluginMenuFrame(frame) {
		return menuOuterBorderSide
	}
	if frame.GetType() == vtui.TypeDialog {
		return dialogOuterBorderSide
	}
	return 0
}
