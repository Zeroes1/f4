package panel

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

// bookmarkAddressVFS is a file system with an address of its own for
// bookmarks (vfs.BookmarkPathProvider), the way a NetFox session has: its
// GetPath is the folder on the server, which says nothing about the server.
type bookmarkAddressVFS struct {
	*vfs.NullVFS
	address string
	ok      bool
	cursor  string
}

func (v *bookmarkAddressVFS) BookmarkPath(cursor string) (string, bool) {
	v.cursor = cursor
	return v.address, v.ok
}

// A bookmark (Ctrl+Shift+<digit>, Ins in the bookmarks dialog) and a new
// drive-menu link (Ins in the drive menu) store the address the file system
// gives for the panel's location, and fall back to its path when it gives
// none.
func TestBookmarksStoreTheAddressTheFileSystemGives(t *testing.T) {
	remote := &bookmarkAddressVFS{NullVFS: vfs.NewNullVFS(0), address: "net://work/var/log", ok: true}
	fsp := &FileSystemPanel{Vfs: remote}
	fsp.Entries = []*FileEntry{{VFSItem: vfs.VFSItem{Name: "work"}}}
	pf := &PanelsFrame{}
	pf.Panels[0] = fsp

	if got := pf.BookmarkForPanel(fsp); got != (Bookmark{Path: "net://work/var/log"}) {
		t.Errorf("BookmarkForPanel() = %#v, want the address net://work/var/log", got)
	}
	if remote.cursor != "work" {
		t.Errorf("the file system was asked with the cursor on %q, want the entry under the cursor, work", remote.cursor)
	}
	if got := pf.driveBookmarkDefaultPath(0); got != "net://work/var/log" {
		t.Errorf("the path a new link offers = %q, want net://work/var/log", got)
	}

	// The file system declines: its path, as for any other.
	remote.ok = false
	if got := pf.BookmarkForPanel(fsp).Path; got != remote.GetPath() {
		t.Errorf("BookmarkForPanel().Path = %q after the file system declined, want its path %q", got, remote.GetPath())
	}
	if got := pf.driveBookmarkDefaultPath(0); got != remote.GetPath() {
		t.Errorf("the path a new link offers = %q after the file system declined, want %q", got, remote.GetPath())
	}

	// A file system without an address of its own, and a panel without an
	// entry under the cursor.
	plain := &FileSystemPanel{Vfs: vfs.NewNullVFS(0)}
	pf.Panels[1] = plain
	if got := pf.BookmarkForPanel(plain).Path; got != plain.Vfs.GetPath() {
		t.Errorf("BookmarkForPanel().Path = %q for a plain file system, want its path %q", got, plain.Vfs.GetPath())
	}
	if got := bookmarkPathOf(nil); got != "" {
		t.Errorf("bookmarkPathOf(nil) = %q, want \"\"", got)
	}
}
