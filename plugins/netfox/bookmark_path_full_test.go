//go:build !lite

package netfox

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

// The session kinds of the full build carry their bookmark address into
// their clones too: the panel clones a file system for the other panel and
// for a background task, and a bookmark made from such a view is the same
// bookmark.
func TestFullBuildSessionsCarryTheirBookmarkAddressIntoClones(t *testing.T) {
	sessions := map[string]vfs.VFS{
		"sftp": &SFTPVFS{path: "/srv", codepage: "65001", shared: &sftpConnectionRefs{refs: 1}},
		"ftp":  &FTPVFS{cwd: "/srv"},
		"smb":  &smbVFS{cwd: "/share/srv", session: &smbSession{refs: 1}},
	}
	for kind, session := range sessions {
		provider, ok := session.(vfs.BookmarkPathProvider)
		if !ok {
			t.Fatalf("%s: %T is no vfs.BookmarkPathProvider", kind, session)
		}
		if got, ok := provider.BookmarkPath(""); ok {
			t.Errorf("%s: a session opened from nowhere has a bookmark address %q, want none", kind, got)
		}
		openedFromConnection(session, "work")
		want := "net://work" + session.GetPath()
		if got, ok := provider.BookmarkPath(""); !ok || got != want {
			t.Errorf("%s: BookmarkPath() = %q, %v; want %q", kind, got, ok, want)
		}
		clone, ok := session.Clone().(vfs.BookmarkPathProvider)
		if !ok {
			t.Fatalf("%s: the clone %T is no vfs.BookmarkPathProvider", kind, session.Clone())
		}
		if got, ok := clone.BookmarkPath(""); !ok || got != want {
			t.Errorf("%s: the clone's BookmarkPath() = %q, %v; want %q", kind, got, ok, want)
		}
	}
}
