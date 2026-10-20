package panel

import "github.com/unxed/f4/vfs"

// bookmarkPathOf is the address a folder bookmark or a drive-menu link made
// on fsp stores: the one the panel's file system asks for when it has an
// address of its own (vfs.BookmarkPathProvider: a NetFox session, whose
// GetPath is the directory on the server and says nothing about the server),
// its path otherwise. The entry under the cursor goes along, so the list of
// connections can name the connection the cursor is on.
func bookmarkPathOf(fsp *FileSystemPanel) string {
	if fsp == nil || fsp.Vfs == nil {
		return ""
	}
	if provider, ok := fsp.Vfs.(vfs.BookmarkPathProvider); ok {
		if address, ok := provider.BookmarkPath(fsp.GetRawSelectedName()); ok && address != "" {
			return address
		}
	}
	return fsp.Vfs.GetPath()
}
