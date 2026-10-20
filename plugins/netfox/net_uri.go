package netfox

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// The address of a saved connection: net://<name>/<folder on the server>.
//
// A folder bookmark or a drive-menu link stores a path and nothing else, and a
// NetFox session's own path (/home/user) says nothing about the server it is
// on. The connection manager, on the other hand, already knows everything a
// reconnect needs -- type, host, port, user, password or key, proxy, codepage
// -- under the name the user gave the connection, so that name is what a
// bookmark remembers: net://work-server/var/log opens the saved connection
// "work-server" and goes to /var/log, whatever its type. net:// alone is the
// connection manager itself, the path NetFoxVFS already reports.
//
// The name takes the place of the host. It is the user's own text, so the two
// characters that would break the address are escaped and nothing else is:
// "/" would end the name early, "%" would be taken for an escape. The folder
// follows verbatim, as the server spells it.
const netURIScheme = "net"

// netURIPrefix is the scheme with its separator, the path of the connection
// manager.
const netURIPrefix = netURIScheme + "://"

// netConnectionAddress is the address of the saved connection name: the base
// a session opened from it builds its bookmark path on (netfoxBookmarkPath).
func netConnectionAddress(name string) string {
	name = strings.ReplaceAll(name, "%", "%25")
	name = strings.ReplaceAll(name, "/", "%2F")
	return netURIPrefix + name
}

// parseNetURI splits a net:// address into the connection name and the folder
// to open; both are "" for the bare net:// of the connection manager. The
// name is unescaped as a URL path segment is; an address typed by hand with a
// plain "%" in the name is taken as it reads.
func parseNetURI(raw string) (name, folder string, err error) {
	scheme, ok := vfs.URIScheme(raw)
	if !ok || scheme != netURIScheme {
		return "", "", fmt.Errorf("NetFox: %q is not a net:// address", raw)
	}
	rest := raw[len(netURIPrefix):]
	name = rest
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		name, folder = rest[:slash], rest[slash:]
	}
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	if name == "" && folder != "" && folder != "/" {
		return "", "", fmt.Errorf("NetFox: %q names no connection", raw)
	}
	if name == "" {
		folder = ""
	}
	return name, folder, nil
}

// netURIProvider opens net:// addresses: the connection manager for the bare
// net://, otherwise the saved connection the address names, through the same
// provider Enter on it in the manager uses, moved to the folder the address
// ends in. The manager reads the connections file at that moment, so a
// connection renamed or deleted since the bookmark was made is reported, not
// guessed at.
type netURIProvider struct {
	// manager opens the connection manager: the one the NetFox drive opens,
	// or a test's over its own file.
	manager func() vfs.VFS
}

func (p *netURIProvider) Scheme() string { return netURIScheme }

func (p *netURIProvider) OpenURI(ctx context.Context, current vfs.VFS, raw string) (vfs.VFS, error) {
	name, folder, err := parseNetURI(raw)
	if err != nil {
		return nil, err
	}
	manager := p.manager()
	if name == "" {
		return manager, nil
	}
	entry := manager.Join(manager.GetPath(), name)
	if _, err := manager.Stat(ctx, entry); err != nil {
		_ = manager.Close()
		return nil, fmt.Errorf("NetFox: no saved connection named %q", name)
	}
	provider := vfs.FindProvider(ctx, manager, entry)
	if provider == nil {
		_ = manager.Close()
		return nil, fmt.Errorf("NetFox: the connection %q is of a type this build cannot open", name)
	}
	session, err := provider.Open(ctx, manager, entry)
	if err != nil {
		_ = manager.Close()
		return nil, err
	}
	if session == nil {
		_ = manager.Close()
		return nil, fmt.Errorf("NetFox: %s returned no file system for %q", provider.Name(), name)
	}
	enterBookmarkFolder(session, folder)
	return session, nil
}

// enterBookmarkFolder moves a freshly opened session into the folder a
// bookmark names, the way enterStartPath moves one into the connection's own
// start folder: a folder that is gone leaves the session where the connection
// opened it and says so in a toast, since the connection itself is fine.
func enterBookmarkFolder(v vfs.VFS, folder string) {
	if folder == "" {
		return
	}
	if err := v.SetPath(folder); err != nil {
		vtui.DebugLog("NETFOX: the bookmarked folder %q: %v", folder, err)
		startPathToast(fmt.Sprintf(vtui.Msg("NetFox.BookmarkFolderMissing"), folder, v.GetPath()))
	}
}
