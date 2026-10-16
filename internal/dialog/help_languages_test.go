package dialog

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHelpSearchDirsResolvesExecutableSymlink(t *testing.T) {
	root := t.TempDir()
	installDir := filepath.Join(root, "install")
	linkDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(installDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(linkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(installDir, "f4")
	if err := os.WriteFile(target, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(linkDir, "f4")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	dirs := helpSearchDirs(link, filepath.Join(root, "profile", "help"))
	if got, want := dirs[0], filepath.Join(installDir, "help"); got != want {
		t.Fatalf("executable help directory = %q, want resolved target directory %q", got, want)
	}
}
