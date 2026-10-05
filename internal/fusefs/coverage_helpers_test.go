package fusefs

import (
	"os"
	"testing"
)

type titledFakeVFS struct {
	*fakeVFS
	title string
}

func (v *titledFakeVFS) GetTitle() string { return v.title }

func TestDescribeSourceUsesExplicitSourceOrTitle(t *testing.T) {
	v := &titledFakeVFS{fakeVFS: newFakeVFS(false), title: "Archive"}
	if got := describeSource("  shown source  ", v, "/root"); got != "  shown source  " {
		t.Fatalf("explicit source = %q, want it preserved", got)
	}
	if got := describeSource("", v, "/root"); got != "Archive/root" {
		t.Fatalf("titled source = %q, want %q", got, "Archive/root")
	}
	v.title = ""
	if got := describeSource("", v, "/root"); got != "/root" {
		t.Fatalf("untitled source = %q, want root path", got)
	}
}

func TestMountRootUsesRuntimeDirectory(t *testing.T) {
	old, had := os.LookupEnv("XDG_RUNTIME_DIR")
	t.Cleanup(func() {
		if had {
			_ = os.Setenv("XDG_RUNTIME_DIR", old)
		} else {
			_ = os.Unsetenv("XDG_RUNTIME_DIR")
		}
	})
	if err := os.Setenv("XDG_RUNTIME_DIR", "/run/user/test"); err != nil {
		t.Fatal(err)
	}
	if got := MountRoot(); got != "/run/user/test/f4/mnt" {
		t.Fatalf("MountRoot() = %q, want %q", got, "/run/user/test/f4/mnt")
	}
}
