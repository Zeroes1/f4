package app

import (
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/vfs"
)

// F3 on a folder sizes it and F4 on one opens its attributes, so the key bar
// says Size and Attr there instead of View and Edit (f4#1794, f4#1795).
func TestKeyBarNamesWhatF3AndF4DoOnAFolder(t *testing.T) {
	pf := seedPanelForRestore(t, []string{"file.txt"})
	fsp := pf.GetActivePanel()
	fsp.Entries = append(fsp.Entries, &panel.FileEntry{VFSItem: vfs.VFSItem{Name: "sub", IsDir: true}})
	fsp.Refresh()

	cases := []struct {
		name           string
		wantF3, wantF4 string
	}{
		{"..", "", ""},
		{"file.txt", "", ""},
		{"sub", i18n.Msg("KeyBar.F3Size"), i18n.Msg("KeyBar.F4Attr")},
	}
	for _, tc := range cases {
		idx := -1
		for i, e := range fsp.Entries {
			if e.Name == tc.name {
				idx = i
			}
		}
		if idx < 0 {
			t.Fatalf("no %q in the panel", tc.name)
		}
		fsp.SetCursorIndex(idx)
		if got := viewKeyBarLabel(); got != tc.wantF3 {
			t.Errorf("F3 label on %q = %q, want %q", tc.name, got, tc.wantF3)
		}
		if got := editKeyBarLabel(); got != tc.wantF4 {
			t.Errorf("F4 label on %q = %q, want %q", tc.name, got, tc.wantF4)
		}
	}
}
