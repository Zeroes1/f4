package netfox

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

// The address of a saved connection, net://<name>/<folder>: what a folder
// bookmark or a drive-menu link made on a NetFox panel stores, and what the
// net:// URI provider opens.

func TestNetConnectionAddressRoundTrips(t *testing.T) {
	for _, name := range []string{"work", "my server", "a/b", "50%", "сервер", "x%2Fy", "host:22"} {
		address := netConnectionAddress(name)
		if !strings.HasPrefix(address, "net://") {
			t.Errorf("address of %q = %q, want a net:// one", name, address)
		}
		got, folder, err := parseNetURI(address)
		if err != nil || got != name || folder != "" {
			t.Errorf("parse(%q) = %q, %q, %v; want %q, \"\", nil", address, got, folder, err, name)
		}
		got, folder, err = parseNetURI(address + "/var/log")
		if err != nil || got != name || folder != "/var/log" {
			t.Errorf("parse(%q/var/log) = %q, %q, %v; want %q, /var/log, nil", address, got, folder, err, name)
		}
	}
}

// Only "/" and "%" are escaped: a Cyrillic or a spaced name reads in the
// bookmarks dialog as it was typed in the manager.
func TestNetConnectionAddressKeepsTheNameReadable(t *testing.T) {
	for name, want := range map[string]string{
		"мой сервер": "net://мой сервер",
		"a/b":        "net://a%2Fb",
		"50%":        "net://50%25",
	} {
		if got := netConnectionAddress(name); got != want {
			t.Errorf("address of %q = %q, want %q", name, got, want)
		}
	}
}

func TestParseNetURI(t *testing.T) {
	for _, tc := range []struct {
		raw, name, folder string
		wantErr           bool
	}{
		{raw: "net://"},
		{raw: "net:///"},
		{raw: "NET://work", name: "work"},
		{raw: "net://work/", name: "work", folder: "/"},
		{raw: "net://work/var/log", name: "work", folder: "/var/log"},
		{raw: "net://work/my docs/x", name: "work", folder: "/my docs/x"},
		// A plain "%" typed by hand is taken as it reads.
		{raw: "net://100%srv/x", name: "100%srv", folder: "/x"},
		{raw: "net:///var/log", wantErr: true},
		{raw: "sftp://host/", wantErr: true},
		{raw: "work", wantErr: true},
		{raw: "", wantErr: true},
	} {
		name, folder, err := parseNetURI(tc.raw)
		if (err != nil) != tc.wantErr {
			t.Errorf("parse(%q) error = %v, want error: %v", tc.raw, err, tc.wantErr)
			continue
		}
		if name != tc.name || folder != tc.folder {
			t.Errorf("parse(%q) = %q, %q; want %q, %q", tc.raw, name, folder, tc.name, tc.folder)
		}
	}
}

// testConnectionManager is a connection manager over a file of its own
// holding the given connections, the way the plugin opens the real one.
func testConnectionManager(t *testing.T, configs map[string]NetFoxConfig) func() vfs.VFS {
	t.Helper()
	store := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	for name, cfg := range configs {
		if err := store.SaveConfig(name, cfg); err != nil {
			t.Fatal(err)
		}
	}
	return func() vfs.VFS { return &netFoxVFSWrapper{NetFoxVFS: store.Clone().(*NetFoxVFS)} }
}

func TestNetURIProviderOpensTheConnectionManager(t *testing.T) {
	provider := &netURIProvider{manager: testConnectionManager(t, nil)}
	if got := provider.Scheme(); got != "net" {
		t.Fatalf("Scheme() = %q, want net", got)
	}
	for _, raw := range []string{"net://", "net:///"} {
		v, err := provider.OpenURI(context.Background(), nil, raw)
		if err != nil {
			t.Fatalf("OpenURI(%q): %v", raw, err)
		}
		if _, ok := v.(*netFoxVFSWrapper); !ok || v.GetPath() != "net://" {
			t.Fatalf("OpenURI(%q) = %T at %q, want the connection manager at net://", raw, v, v.GetPath())
		}
	}
}

func TestNetURIProviderNamesAConnectionItCannotOpen(t *testing.T) {
	provider := &netURIProvider{manager: testConnectionManager(t, map[string]NetFoxConfig{
		"odd": {Type: "no-such-type", Host: "h"},
	})}
	for _, tc := range []struct{ raw, name string }{
		{"net://gone/var/log", `"gone"`},
		{"net://odd", `"odd"`},
	} {
		_, err := provider.OpenURI(context.Background(), nil, tc.raw)
		if err == nil || !strings.Contains(err.Error(), tc.name) {
			t.Errorf("OpenURI(%q) error = %v, want one naming %s", tc.raw, err, tc.name)
		}
	}
}

// bookmarkTestProvider opens the saved connections of type "bookmark-test"
// without a server, as the real providers open theirs: the manager is the
// parent, the connection's name is the bookmark base.
type bookmarkTestProvider struct{}

func (*bookmarkTestProvider) Name() string  { return "bookmark-test" }
func (*bookmarkTestProvider) Priority() int { return 100 }
func (*bookmarkTestProvider) CanOpen(ctx context.Context, parent vfs.VFS, pth string) bool {
	cfg, ok := netFoxConfigAt(ctx, parent, pth)
	return ok && cfg.Type == "bookmark-test"
}
func (*bookmarkTestProvider) Open(ctx context.Context, parent vfs.VFS, pth string) (vfs.VFS, error) {
	cfg, ok := netFoxConfigAt(ctx, parent, pth)
	if !ok {
		return nil, os.ErrInvalid
	}
	v := &bookmarkTestVFS{NullVFS: vfs.NewNullVFS(0), parent: parent, missing: cfg.Options["Missing"]}
	enterStartPath(v, cfg)
	openedFromConnection(v, parent.Base(pth))
	return v, nil
}

// bookmarkTestVFS is the session such a connection opens: every folder is
// there but the one the connection says is missing.
type bookmarkTestVFS struct {
	*vfs.NullVFS
	parent       vfs.VFS
	bookmarkBase string
	missing      string
}

func (v *bookmarkTestVFS) ParentVFS() vfs.VFS          { return v.parent }
func (v *bookmarkTestVFS) setBookmarkBase(base string) { v.bookmarkBase = base }
func (v *bookmarkTestVFS) BookmarkPath(string) (string, bool) {
	return netfoxBookmarkPath(v.bookmarkBase, v.GetPath())
}
func (v *bookmarkTestVFS) SetPath(p string) error {
	if p == v.missing {
		return os.ErrNotExist
	}
	return v.NullVFS.SetPath(p)
}

func TestNetURIProviderFollowsABookmarkToItsFolder(t *testing.T) {
	provider := &bookmarkTestProvider{}
	vfs.RegisterProvider(provider)
	t.Cleanup(func() { vfs.UnregisterProvider(provider) })
	var toasts []string
	old := startPathToast
	startPathToast = func(m string) { toasts = append(toasts, m) }
	t.Cleanup(func() { startPathToast = old })

	uri := &netURIProvider{manager: testConnectionManager(t, map[string]NetFoxConfig{
		"my server": {Type: "bookmark-test", Host: "h", Path: "/home/u", Options: map[string]string{"Missing": "/gone"}},
	})}
	open := func(raw string) vfs.VFS {
		t.Helper()
		toasts = nil
		v, err := uri.OpenURI(context.Background(), nil, raw)
		if err != nil {
			t.Fatalf("OpenURI(%q): %v", raw, err)
		}
		if _, ok := v.ParentVFS().(*netFoxVFSWrapper); !ok {
			t.Fatalf("OpenURI(%q) opened the session over %T, want the connection manager, so that .. leaves to it", raw, v.ParentVFS())
		}
		return v
	}

	// The folder of the bookmark, and the address that bookmarks it again.
	v := open("net://my server/var/log")
	if got := v.GetPath(); got != "/var/log" {
		t.Errorf("the session is in %q, want /var/log", got)
	}
	if got, ok := v.(vfs.BookmarkPathProvider).BookmarkPath(""); !ok || got != "net://my server/var/log" {
		t.Errorf("the session's bookmark address = %q, %v; want net://my server/var/log", got, ok)
	}
	if len(toasts) != 0 {
		t.Errorf("toasts %q, want none", toasts)
	}

	// No folder: wherever the connection opens, its start folder here.
	if v := open("net://my server"); v.GetPath() != "/home/u" || len(toasts) != 0 {
		t.Errorf("the connection alone opened in %q with toasts %q, want /home/u and none", v.GetPath(), toasts)
	}
	if v := open("net://my server/"); v.GetPath() != "/" {
		t.Errorf("net://my server/ opened in %q, want the root", v.GetPath())
	}

	// A folder that is gone leaves the session where the connection opened
	// it, and a toast says so: the connection itself is fine.
	v = open("net://my server/gone")
	if got := v.GetPath(); got != "/home/u" {
		t.Errorf("the session is in %q, want the start folder /home/u", got)
	}
	if len(toasts) != 1 || !strings.Contains(toasts[0], "/gone") || !strings.Contains(toasts[0], "/home/u") {
		t.Errorf("toasts %q, want one naming /gone and /home/u", toasts)
	}
}

// The manager's own part of the address: its SetPath accepts its root and
// nothing else, so a net://<name> address reaches the URI provider instead
// of being swallowed as a folder the manager is "already in".
func TestNetFoxVFSSetPathAcceptsOnlyItsRoot(t *testing.T) {
	v := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	for _, root := range []string{"", "/", ".", "net://", "net:/", "net:"} {
		if err := v.SetPath(root); err != nil {
			t.Errorf("SetPath(%q) = %v, want nil", root, err)
		}
	}
	for _, folder := range []string{"net://work", "net://work/var/log", "net:/work", "work", "/work"} {
		err := v.SetPath(folder)
		if !errors.Is(err, os.ErrNotExist) {
			t.Errorf("SetPath(%q) = %v, want os.ErrNotExist", folder, err)
		}
	}
	if got := v.GetPath(); got != "net://" {
		t.Errorf("GetPath() = %q after the refusals, want net://", got)
	}
}

func TestNetFoxVFSBookmarkPathIsTheConnectionUnderTheCursor(t *testing.T) {
	store := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	if err := store.SaveConfig("work", NetFoxConfig{Type: "sftp", Host: "h"}); err != nil {
		t.Fatal(err)
	}
	// The panel holds the wrapper, so it is the wrapper that must answer.
	var manager vfs.BookmarkPathProvider = &netFoxVFSWrapper{NetFoxVFS: store}
	for cursor, want := range map[string]string{
		"work":             "net://work",
		"":                 "net://",
		"..":               "net://",
		"<Add connection>": "net://",
		"unknown":          "net://",
	} {
		if got, ok := manager.BookmarkPath(cursor); !ok || got != want {
			t.Errorf("BookmarkPath(%q) = %q, %v; want %q", cursor, got, ok, want)
		}
	}
}
