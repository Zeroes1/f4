package panel

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fileops"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/f4/vfs"
)

// fileClipboard is what "Copy files to clipboard" and "Cut files to
// clipboard" leave behind (f4#1767): the files themselves stay where they
// are, and the paste that follows copies or moves them into the directory of
// the active panel, through the same file operations as F5 and F6.
//
// The system clipboard gets the paths of the files as text, so other programs
// see something useful. That text is also how a paste knows the clipboard is
// still ours: when the system clipboard holds anything else by then, the user
// has copied something newer, and the remembered files are dropped instead of
// being pasted over it.
type fileClipboard struct {
	source vfs.VFS
	base   string
	names  []string
	cut    bool
	text   string
}

// runFileClipboardOp starts the copy or move a paste stands for. It is a
// variable so a test can look at the request instead of starting a transfer.
var runFileClipboardOp = func(source, target vfs.VFS, base string, names []string, dest string, move bool, done func()) {
	go fileops.ExecuteFileOpAt(source, target, base, names, dest, move, config.App.DefaultFileOpMode, done)
}

// PanelCanCopyFilesToClipboard reports whether the active panel names
// anything to put on the clipboard. A command line with text in it keeps
// Ctrl+C for itself: that text, not the files, is what the key is aimed at.
func PanelCanCopyFilesToClipboard(pf *PanelsFrame) bool {
	if pf == nil || !pf.ShowPanels {
		return false
	}
	active := pf.GetActivePanel()
	if active == nil || active.Vfs == nil || !pf.CmdLine.IsEmpty() {
		return false
	}
	return len(active.GetSelectedNames()) > 0
}

// ActionCopyFilesToClipboard remembers the selected files of the active
// panel (the one under the cursor when nothing is marked) for a later paste;
// with cut set, the paste moves them instead of copying.
func ActionCopyFilesToClipboard(pf *PanelsFrame, cut bool) bool {
	if pf == nil || !pf.ShowPanels {
		return false
	}
	active := pf.GetActivePanel()
	if active == nil || active.Vfs == nil {
		return false
	}
	names := active.GetSelectedNames()
	if len(names) == 0 {
		return false
	}
	base := active.Vfs.GetPath()
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = active.Vfs.Join(base, name)
	}
	clip := &fileClipboard{
		source: active.Vfs,
		base:   base,
		names:  names,
		cut:    cut,
		text:   strings.Join(paths, "\n"),
	}
	pf.fileClip = clip
	terminal.SetF4Clipboard(clip.text)

	key := "Panel.FileClipboard.CopiedFmt"
	if cut {
		key = "Panel.FileClipboard.CutFmt"
	}
	toast.Show(fmt.Sprintf(i18n.Msg(key), len(names)), 3*time.Second)
	return true
}

// normalizeClipboardText makes two readings of the same clipboard compare
// equal however a terminal or a clipboard tool treated the line breaks and the
// trailing newline.
func normalizeClipboardText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.TrimRight(text, "\r\n")
}

// pasteFileClipboard is the first thing a paste asks, with what the system
// clipboard holds (text) or why it could not be read (readErr). It reports
// whether the paste was spent on the remembered files; false sends the paste
// on to the usual text and image handling.
func (pf *PanelsFrame) pasteFileClipboard(text string, readErr error) bool {
	clip := pf.fileClip
	// Text typed on the command line is what Ctrl+V is aimed at then.
	if clip == nil || !pf.CmdLine.IsEmpty() {
		return false
	}
	// An unreadable clipboard (no clipboard tool, a terminal that refuses)
	// cannot say the files are stale, and the remembered files are still what
	// the user asked for.
	if readErr == nil && normalizeClipboardText(text) != normalizeClipboardText(clip.text) {
		pf.fileClip = nil
		return false
	}
	target := pf.GetActivePanel()
	if target == nil || target.Vfs == nil {
		return false
	}
	dest := target.Vfs.GetPath()
	if dest != "" && !strings.HasSuffix(dest, "/") && !strings.HasSuffix(dest, "\\") {
		sep := "/"
		if _, local := target.Vfs.(*vfs.OSVFS); local && runtime.GOOS == "windows" {
			sep = "\\"
		}
		dest += sep
	}
	if clip.cut {
		// A move takes the files away from where they were remembered, so the
		// clipboard has nothing left to paste again.
		pf.fileClip = nil
	}
	done := func() {
		if !pf.Closed {
			pf.RefreshAll()
		}
	}
	runFileClipboardOp(clip.source, target.Vfs, clip.base, clip.names, dest, clip.cut, done)
	return true
}
