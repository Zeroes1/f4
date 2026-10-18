package netfox

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/unxed/f4/vfs"
)

// startPathProbe is a connection that knows one folder, /srv/data.
type startPathProbe struct {
	*vfs.NullVFS
	path string
}

func (p *startPathProbe) SetPath(s string) error {
	if s != "/srv/data" {
		return errors.New("no such folder")
	}
	p.path = s
	return nil
}

// nz256, f4#1894: a saved connection opens in its folder, as a Far connection
// with a path does.
func TestEnterStartPathOpensTheSavedFolder(t *testing.T) {
	for _, tc := range []struct{ name, saved, want string }{
		{"saved folder", "/srv/data", "/srv/data"},
		{"spaces around it", "  /srv/data ", "/srv/data"},
		{"no folder saved", "", "/home/u"},
		{"a folder that is not there", "/srv/gone", "/home/u"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := &startPathProbe{NullVFS: vfs.NewNullVFS(0), path: "/home/u"}
			enterStartPath(v, NetFoxConfig{Host: "h", Path: tc.saved})
			if v.path != tc.want {
				t.Errorf("the connection is in %q, want %q", v.path, tc.want)
			}
		})
	}
}

func TestNetFoxConfigKeepsThePath(t *testing.T) {
	manager := NewNetFoxVFS(filepath.Join(t.TempDir(), "NetFox.json"))
	if err := manager.SaveConfig("site", NetFoxConfig{Type: "sftp", Host: "h", Path: "/srv/data"}); err != nil {
		t.Fatal(err)
	}
	cfg, ok := netFoxConfigAt(t.Context(), &netFoxVFSWrapper{NetFoxVFS: manager}, "site")
	if !ok || cfg.Path != "/srv/data" {
		t.Fatalf("the saved connection reads back as %+v (%v), want Path /srv/data", cfg, ok)
	}
}
