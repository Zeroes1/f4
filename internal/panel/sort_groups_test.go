package panel

import (
	"github.com/unxed/f4/vfs"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/ini"
	"github.com/unxed/f4/internal/theme"
)

func TestSortGroupsReuseHighlightRules(t *testing.T) {
	ini := ini.Parse(strings.NewReader(`[Highlight_4]
Name = Archives
Group = 1
Mask = *.zip, *.7z

[Highlight_5]
Name = Images
Group = 2
Mask = *.png, *.jpg

[Highlight_6]
Name = Unsorted highlight rule
Mask = *.txt
`))
	groups := sortGroupsFromHighlightRules(theme.ParseHighlightRules(ini))
	if len(groups) != 2 {
		t.Fatalf("shared highlight rules produced %d sort groups, want 2", len(groups))
	}
	if groups[0].Name != "Archives" || groups[0].Order != 1 {
		t.Fatalf("first shared group = %#v, want Archives at 1", groups[0])
	}
	if groups[1].Name != "Images" || groups[1].Order != 2 {
		t.Fatalf("second shared group = %#v, want Images at 2", groups[1])
	}

	archive := vfs.VFSItem{Name: "backup.ZIP"}
	if got := groups[0].Filter.Match(&archive); !got {
		t.Fatal("shared highlight matcher did not match archive")
	}
}

func TestSortGroupSetLoadsSharedAndLegacyRules(t *testing.T) {
	ini := ini.Parse(strings.NewReader(`[Highlight_1]
Name = Images
Group = 0
Mask = *.png

[SortGroup_1]
Name = Legacy
Group = 1
Mask = *.legacy
`))
	rules := theme.ParseHighlightRules(ini)
	set := &SortGroupSet{}
	set.LoadFromIni(ini, rules)
	if len(set.Groups) != 2 {
		t.Fatalf("loaded %d sort groups, want shared and legacy rules", len(set.Groups))
	}

	image := vfs.VFSItem{Name: "photo.png"}
	legacy := vfs.VFSItem{Name: "old.legacy"}
	if got := set.GroupOf(&image); got != 0 {
		t.Errorf("shared rule group = %d, want 0", got)
	}
	if got := set.GroupOf(&legacy); got != 1 {
		t.Errorf("legacy rule group = %d, want 1", got)
	}
}
