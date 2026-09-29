package panel

import (
	"context"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/terminal/far2ldnd"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// The receiving end of far2l DND (unxed/f4#1628, step 5): when the terminal
// this f4 runs in hands it an offer (INPUT_DND), the offered files are copied
// into the panel where the drop happened -- the panel under the pointer, its
// directory under the pointer if there is one, otherwise the active panel --
// by the ordinary file operation, so progress, overwrite questions, the queue
// and cancelling are the ones F5 has. The offer is read through
// DropSourceVFS and is closed when the operation ends, whatever came of it.

// dndOfferTimeout bounds listing one offer; the copy itself is bounded by
// nothing but the user, like any other copy.
const dndOfferTimeout = 30 * time.Second

// ReceiveTerminalOffer handles one INPUT_DND event of client's binding. It
// blocks while the offer is listed and must therefore run off the UI
// goroutine (DNDClient.OnEvent already does); the copy is handed to the UI
// and the file-operation goroutine from here.
func (pf *PanelsFrame) ReceiveTerminalOffer(client *terminal.DNDClient, ev far2ldnd.Event) {
	if pf == nil || client == nil {
		return
	}
	closeOffer := func(reason uint8) {
		ctx, cancel := context.WithTimeout(context.Background(), dndOfferTimeout)
		defer cancel()
		if err := client.Close(ctx, ev.Offer, reason); err != nil {
			vtui.DebugLog("DND: closing the offer failed: %v", err)
		}
	}
	granted, ok := client.Bound()
	if !ok {
		closeOffer(far2ldnd.CloseRejected)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), dndOfferTimeout)
	src, err := NewDropSourceVFS(ctx, client, ev.Offer, granted.MaxChunk)
	cancel()
	if err != nil {
		vtui.DebugLog("DND: cannot list the offer: %v", err)
		closeOffer(far2ldnd.CloseFailed)
		msg := err.Error()
		vtui.FrameManager.PostTask(func() {
			vtui.ShowMessage(" Drag and Drop ", "The dropped files cannot be read:\n\n"+msg, []string{"&Ok"})
		})
		return
	}
	var names []string
	_ = src.ReadDir(context.Background(), "/", func(items []vfs.VFSItem) {
		for _, it := range items {
			names = append(names, it.Name)
		}
	})
	if len(names) == 0 {
		closeOffer(far2ldnd.CloseRejected)
		return
	}
	vtui.FrameManager.PostTask(func() {
		dst, dstDir, ok := pf.nestedDropTarget(ev)
		if !ok {
			vtui.DebugLog("DND: no panel can take the offer")
			go closeOffer(far2ldnd.CloseRejected)
			vtui.ShowMessage(" Drag and Drop ", "There is no panel to drop the files into.", []string{"&Ok"})
			return
		}
		vtui.DebugLog("DND: offer of %d file(s) -> %q", len(names), dstDir)
		go fileops.ExecuteFileOpAt(src, dst, "/", names, dstDir, false, config.App.DefaultFileOpMode, func() {
			closeOffer(far2ldnd.CloseProcessed)
			vtui.FrameManager.PostTask(func() {
				pf.RefreshAll()
				vtui.FrameManager.Redraw()
			})
		})
	})
}

// nestedDropTarget picks the destination of an offered drop, on the UI
// goroutine: the panel under the event's cell when the cell is known and a
// panel that can be written into is there, otherwise the active panel.
func (pf *PanelsFrame) nestedDropTarget(ev far2ldnd.Event) (vfs.VFS, string, bool) {
	if ev.PositionKnown() {
		if info, ok := pf.resolveDropTarget(int(ev.X), int(ev.Y)); ok && VfsAcceptsDrop(info.fs) {
			return info.fs, info.Dir, true
		}
	}
	if fsp := pf.GetActivePanel(); fsp != nil && VfsAcceptsDrop(fsp.Vfs) {
		return fsp.Vfs, fsp.Vfs.GetPath(), true
	}
	return nil, "", false
}

// InstallTerminalOfferHandler makes client hand the offers of its binding to
// the panels: the copy is set up on the UI goroutine and the offer is read
// off it. Nothing happens until something binds the client (DNDClient.Bind);
// until then no terminal has been told that this program takes drops.
func InstallTerminalOfferHandler(client *terminal.DNDClient) {
	if client == nil {
		return
	}
	client.OnEvent = func(ev far2ldnd.Event) {
		vtui.FrameManager.PostTask(func() {
			if pf := FindPanelsFrameAnyScreen(); pf != nil {
				go pf.ReceiveTerminalOffer(client, ev)
			}
		})
	}
}
