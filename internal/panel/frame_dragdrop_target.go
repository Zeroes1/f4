package panel

import (
	"fmt"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtui"
)

// Where dropped files go (#2049). A drop over a folder row used to go into
// that folder whatever the user could see, and no backend showed the folder
// while the files were dragged, so a drop "into the panel" quietly landed in
// whichever folder happened to be under the pointer.
//
// Where the backend follows the drag (vtui.DragEvent.Tracked: OLE on
// Windows, XDND on X11, gogpu with drag callbacks on macOS and Wayland), the
// target is highlighted as the pointer moves - the folder row, or the panel
// frame for the folder the panel shows - and the drop goes where the
// highlight is. Where the backend learns of nothing but the drop, nothing was
// highlighted, and a drop over a folder row goes where DropOnFolder says:
// into the folder the panel shows by default, into the row's folder, or
// after asking. A drop on the command line puts the paths there instead
// (frame_dragdrop_cmdline.go).

// setDropHighlight shows where a drop at the current point would land, and
// clears it from every other panel and the command line. ok false clears it
// everywhere.
func (pf *PanelsFrame) setDropHighlight(info dropTargetInfo, ok bool) {
	pf.showDropHighlight(info, ok, false)
}

func (pf *PanelsFrame) clearDropHighlight() {
	pf.showDropHighlight(dropTargetInfo{}, false, false)
}

// showDropHighlight puts the drop highlight on the folder info resolved to
// (ok) or on the command line (cmdLine, frame_dragdrop_cmdline.go), and
// clears it from everywhere else.
func (pf *PanelsFrame) showDropHighlight(info dropTargetInfo, ok, cmdLine bool) {
	changed := pf.cmdLineDrop != cmdLine
	pf.cmdLineDrop = cmdLine
	for i, p := range pf.Panels {
		fsp, isFS := p.(*FileSystemPanel)
		if !isFS || fsp == nil {
			continue
		}
		want := dropHighlight{}
		if ok && i == info.panelIdx && pf.AltPanels[i] == nil {
			want = dropHighlight{active: true, entry: info.entryIdx}
		}
		if fsp.drop != want {
			fsp.drop = want
			changed = true
		}
	}
	if changed && vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

// chooseDropDir decides which folder a drop resolved to info goes into, on
// the UI goroutine, and passes it to proceed - at once, or once the user has
// answered. cancel is called instead when the user declines; it may be nil.
func (pf *PanelsFrame) chooseDropDir(info dropTargetInfo, tracked bool, proceed func(dir string), cancel func()) {
	if info.entryIdx < 0 || tracked {
		proceed(info.Dir)
		return
	}
	current := info.fs.GetPath()
	switch config.NormalizeDropOnFolder(config.App.DropOnFolder) {
	case config.DropOnFolderFolder:
		proceed(info.Dir)
	case config.DropOnFolderAsk:
		folder := info.Dir
		dlg := vtui.ShowMessage(i18n.Msg("DragDrop.Title"),
			fmt.Sprintf(i18n.Msg("DragDrop.AskFolder"), folder, current),
			[]string{i18n.Msg("DragDrop.IntoFolder"), i18n.Msg("DragDrop.IntoCurrent"), i18n.Msg("DragDrop.Cancel")})
		dlg.OnResult = func(code int) {
			switch code {
			case 0:
				proceed(folder)
			case 1:
				proceed(current)
			default:
				if cancel != nil {
					cancel()
				}
			}
		}
	default:
		proceed(current)
	}
}

// announceDrop says where dropped files are going, so that no drop lands
// anywhere without a word (#2049). The operation's own progress and
// questions follow it.
func announceDrop(groups []DropSourceGroup, dstDir string, isMove bool) {
	n := 0
	for _, g := range groups {
		n += len(g.Names)
	}
	key := "DragDrop.Copying"
	if isMove {
		key = "DragDrop.Moving"
	}
	toast.Show(fmt.Sprintf(i18n.Msg(key), n, dstDir), 3*time.Second)
}
