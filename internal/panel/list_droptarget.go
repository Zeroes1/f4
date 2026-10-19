package panel

import (
	"github.com/unxed/f4/internal/theme"
	"github.com/unxed/vtui"
)

// dropHighlight is what files dragged over the panel would be dropped into
// (#2049): nothing, the folder the panel shows, or the folder row at entry.
// It is set only while the backend follows the drag, so whatever it shows is
// on screen before the drop and is where the drop goes.
type dropHighlight struct {
	active bool
	entry  int // index in Entries of the folder row, or -1 for the panel's folder
}

// drawDropHighlight shows the drop target: the folder row under the pointer
// in the cursor's colours, or, when the files would go into the folder the
// panel shows, the whole panel frame.
func (fp *FileSystemPanel) drawDropHighlight(scr *vtui.ScreenBuf) {
	if !fp.drop.active {
		return
	}
	attr := vtui.Palette[theme.ColPanelCursor]
	if fp.drop.entry < 0 {
		scr.ApplyColor(fp.X1, fp.Y1, fp.X2, fp.Y1, attr)
		scr.ApplyColor(fp.X1, fp.Y2, fp.X2, fp.Y2, attr)
		scr.ApplyColor(fp.X1, fp.Y1, fp.X1, fp.Y2, attr)
		scr.ApplyColor(fp.X2, fp.Y1, fp.X2, fp.Y2, attr)
		return
	}
	if fp.drop.entry >= len(fp.Entries) {
		return
	}
	target := fp.Entries[fp.drop.entry]
	fp.visibleNameCells(func(entry *FileEntry, x, y, width int) {
		if entry == target {
			scr.ApplyColor(x, y, x+width-1, y, attr)
		}
	})
}
