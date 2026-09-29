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
	if pf.offerIsForTerminal(ev) {
		pf.proxyOfferToTerminal(client, ev, granted.MaxChunk, closeOffer)
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
			reason := far2ldnd.CloseProcessed
			if src.Cancelled() {
				reason = far2ldnd.CloseCancelled
			}
			// On the UI goroutine here: the reply of CLOSE is delivered by the
			// same loop, so it must not be waited for from it.
			go closeOffer(reason)
			vtui.FrameManager.PostTask(func() {
				pf.RefreshAll()
				vtui.FrameManager.Redraw()
			})
		})
	})
}

// offerIsForTerminal reports whether an offer received from the outer terminal
// was dropped on the built-in terminal of these panels and a program in it
// takes drops itself: then the drop is the program's, once, and no panel is
// offered the same files (spec v0.2 § 11). It needs the UI goroutine's view of
// the layout, so it asks for it; a UI that does not answer means "no".
func (pf *PanelsFrame) offerIsForTerminal(ev far2ldnd.Event) bool {
	if pf.TermView == nil || !ev.PositionKnown() {
		return false
	}
	answer := make(chan bool, 1)
	vtui.FrameManager.PostTask(func() {
		tv := pf.TermView
		if tv == nil || !tv.DropBound() {
			answer <- false
			return
		}
		if _, onPanel := pf.resolveDropTarget(int(ev.X), int(ev.Y)); onPanel {
			answer <- false
			return
		}
		x1, y1, x2, y2 := tv.GetPosition()
		answer <- int(ev.X) >= x1 && int(ev.X) <= x2 && int(ev.Y) >= y1 && int(ev.Y) <= y2
	})
	select {
	case ok := <-answer:
		return ok
	case <-time.After(dndOfferTimeout):
		return false
	}
}

// proxyOfferToTerminal hands the offer on to the program in the built-in
// terminal: the child gets an offer of its own whose files are read from the
// outer one on demand (terminal.DNDProxySource), with the event's cell moved
// into the child's coordinates. The outer offer is released when the child's
// ends, or at once when the child cannot take it.
func (pf *PanelsFrame) proxyOfferToTerminal(client *terminal.DNDClient, ev far2ldnd.Event, maxChunk uint32, closeOffer func(uint8)) {
	ctx, cancel := context.WithTimeout(context.Background(), dndOfferTimeout)
	src, err := terminal.NewDNDProxySource(ctx, client, ev.Offer, maxChunk)
	cancel()
	if err != nil {
		vtui.DebugLog("DND: cannot list the offer for the terminal: %v", err)
		closeOffer(far2ldnd.CloseFailed)
		return
	}
	tv := pf.TermView
	x1, y1, _, _ := tv.GetPosition()
	known := ev.Flags&far2ldnd.EventModifiersKnown != 0
	if _, err := tv.OfferDrop(src, int(ev.X)-x1, int(ev.Y)-y1, ev.Modifiers, known); err != nil {
		vtui.DebugLog("DND: the terminal refused the offer: %v", err)
		src.CloseWith(far2ldnd.CloseRejected)
	}
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
