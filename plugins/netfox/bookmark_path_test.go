package netfox

import (
	"net/url"
	"testing"
)

func TestNetfoxBookmarkPath(t *testing.T) {
	for _, tc := range []struct {
		base, folder, want string
		ok                 bool
	}{
		{base: "", folder: "/x"},
		{base: "", folder: ""},
		{base: "net://work", folder: "", want: "net://work", ok: true},
		{base: "net://work", folder: "/", want: "net://work/", ok: true},
		{base: "net://work", folder: "/var/log", want: "net://work/var/log", ok: true},
		{base: "sftp://u@h:22", folder: "var", want: "sftp://u@h:22/var", ok: true},
	} {
		got, ok := netfoxBookmarkPath(tc.base, tc.folder)
		if ok != tc.ok || got != tc.want {
			t.Errorf("netfoxBookmarkPath(%q, %q) = %q, %v; want %q, %v", tc.base, tc.folder, got, ok, tc.want, tc.ok)
		}
	}
}

// A session opened from a URI is bookmarked under that URI without its
// password: a bookmark file must never carry one.
func TestURIBookmarkBaseDropsThePassword(t *testing.T) {
	for raw, want := range map[string]string{
		"sftp://alice:secret@host:2222/home/alice": "sftp://alice@host:2222",
		"sftp://host/x":                     "sftp://host",
		"scp://alice@host/":                 "scp://alice@host",
		"smb://DOMAIN;bob:pw@nas/share/dir": "smb://DOMAIN;bob@nas",
		"smb://nas":                         "smb://nas",
	} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", raw, err)
		}
		if got := uriBookmarkBase(u); got != want {
			t.Errorf("uriBookmarkBase(%q) = %q, want %q", raw, got, want)
		}
	}
}

// A session reports the folder on the server under the address it was
// opened from, and a clone of it keeps that address. FISH+ is in every
// build; a struct literal stands for a live session.
func TestFishSessionReportsTheAddressItWasOpenedFrom(t *testing.T) {
	v := &FishVFS{path: "/var/log"}
	if got, ok := v.BookmarkPath(""); ok {
		t.Fatalf("a session opened from nowhere has a bookmark address %q, want none", got)
	}
	openedFromConnection(v, "")
	if got, ok := v.BookmarkPath(""); ok {
		t.Fatalf("an empty connection name gave the bookmark address %q, want none", got)
	}

	openedFromConnection(v, "work")
	if got, ok := v.BookmarkPath("ignored"); !ok || got != "net://work/var/log" {
		t.Errorf("BookmarkPath() = %q, %v; want net://work/var/log", got, ok)
	}
	if got, ok := v.CloneForParent(nil).BookmarkPath(""); !ok || got != "net://work/var/log" {
		t.Errorf("the clone's BookmarkPath() = %q, %v; want net://work/var/log", got, ok)
	}

	u, err := url.Parse("sftp://alice:secret@host:2222/home/alice")
	if err != nil {
		t.Fatal(err)
	}
	fromURI := &FishVFS{path: "/home/alice"}
	openedFromURI(fromURI, u)
	if got, ok := fromURI.BookmarkPath(""); !ok || got != "sftp://alice@host:2222/home/alice" {
		t.Errorf("BookmarkPath() of a URI-opened session = %q, %v; want sftp://alice@host:2222/home/alice", got, ok)
	}
}
