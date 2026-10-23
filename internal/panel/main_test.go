package panel

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/fusefs"
	"github.com/unxed/f4/internal/terminal"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
)

// The F11 menu's visibility file is found through config.UserConfigDir, which
// on darwin ignores the XDG_CONFIG_HOME that testutil.Main points at the
// suite's own profile. Without the seam installTestSeams sets, this package
// and internal/settings -- separate processes under `go test ./...` -- wrote
// the developer's real file, and each other's.
func TestTheSuiteKeepsToItsOwnProfile(t *testing.T) {
	dir := testutil.ConfigDir()
	if dir == "" {
		t.Skip("testutil.Main made no profile directory")
	}
	if path := PluginMenuVisibilityFilePath(); !strings.HasPrefix(path, dir+string(filepath.Separator)) {
		t.Fatalf("the F11 menu visibility file is %s, outside the suite's own profile %s", path, dir)
	}
}

// TestMain sets up what every panel test needs before any of them draws.
func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, installTestSeams, unmountTestFilesystems))
}

func installTestSeams() {
	// f4 extends vtui's palette past its last index, and any widget drawn with
	// one of the extra colours indexes past the end of the default one. Which
	// test draws first depends on the shuffle seed, so the palette is sized
	// here rather than left to whichever test happens to grow it.
	theme.SetDefaultF4Palette()

	// Unit tests must never hand control to the user's desktop, and no frame
	// may fork the user's shell.
	DefaultExternalUICommandRunner = func(string, []string, string) error { return nil }
	SpawnLocalShellPTY = false

	// Copying files to the clipboard must not write the runner's real one.
	terminal.DisableSystemFileClipboard()

	// os.UserConfigDir ignores XDG_CONFIG_HOME on darwin, so the seam is what
	// keeps the suite off the developer's real profile there.
	if dir := testutil.ConfigDir(); dir != "" {
		config.UserConfigDir = func() (string, error) { return dir, nil }
		config.ResetConfigDirForTest()
	}
}

func unmountTestFilesystems() error {
	return errors.Join(fusefs.UnmountAll()...)
}
