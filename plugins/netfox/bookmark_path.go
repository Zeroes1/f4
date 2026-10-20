package netfox

import (
	"net/url"
	"strings"

	"github.com/unxed/f4/vfs"
)

// A session's address for a folder bookmark or a drive-menu link
// (vfs.BookmarkPathProvider).
//
// A session's GetPath() is the folder on the server; what the panel can come
// back to is that folder under the address the session was opened from: the
// saved connection's (net://<name>, net_uri.go) when it was opened from the
// connection manager, the URI without its password (sftp://user@host:port)
// when it was opened from one. A session opened neither way, such as a test's
// local shell, has no such address and the panel stores its path, as before.

// bookmarkBaseSetter is what every session kind implements, so the provider
// that opened it can say where from.
type bookmarkBaseSetter interface {
	setBookmarkBase(base string)
}

// openedFromConnection marks v as a session of the saved connection name.
func openedFromConnection(v vfs.VFS, name string) {
	if s, ok := v.(bookmarkBaseSetter); ok && name != "" {
		s.setBookmarkBase(netConnectionAddress(name))
	}
}

// openedFromURI marks v as a session opened from u: its address is u up to
// the path, without the password.
func openedFromURI(v vfs.VFS, u *url.URL) {
	if s, ok := v.(bookmarkBaseSetter); ok && u != nil {
		s.setBookmarkBase(uriBookmarkBase(u))
	}
}

// uriBookmarkBase is u without its path and without its password. The user
// keeps its place when there is one: the server may need it to log in, and
// a bookmark must never carry the password.
func uriBookmarkBase(u *url.URL) string {
	base := u.Scheme + "://"
	if u.User != nil && u.User.Username() != "" {
		base += url.User(u.User.Username()).String() + "@"
	}
	return base + u.Host
}

// netfoxBookmarkPath is the bookmark address of a session opened from base
// and standing in folder; ok is false when the session has no base, so the
// panel stores the folder itself.
func netfoxBookmarkPath(base, folder string) (string, bool) {
	if base == "" {
		return "", false
	}
	if folder == "" {
		return base, true
	}
	if !strings.HasPrefix(folder, "/") {
		folder = "/" + folder
	}
	return base + folder, true
}

var (
	_ vfs.BookmarkPathProvider = (*FishVFS)(nil)
	_ vfs.BookmarkPathProvider = (*NetFoxVFS)(nil)
	_ bookmarkBaseSetter       = (*FishVFS)(nil)
)
