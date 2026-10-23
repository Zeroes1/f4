package settings

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/f4/internal/theme"
)

type testHost struct{}

func (testHost) SaveSession() error  { return nil }
func (testHost) SessionPath() string { return filepath.Join(config.GetF4ConfigDir(), "session.ini") }
func (testHost) SaveGeometry() error { return nil }
func (testHost) ApplyRuntime(_ config.F4Config, changed []string) {
	for _, id := range changed {
		if id == "ColorStyle" || id == "EnforceColorCorrection" {
			_ = theme.ApplyColorStyle(config.App.ColorStyle)
		}
	}
}
func (testHost) CheckUpdates(context.Context) error                                    { return nil }
func (testHost) GuiBackends() []string                                                 { return []string{"win32", "gogpu", "ebiten", "x11", "wayland"} }
func (testHost) PluginPackage(bool, *panel.PanelsFrame, plughost.PlugRingItem, func()) {}
func InitLang() {
	i18n.InitLang(config.App.Language, config.App.FallbackLanguage, config.LocalLangDir())
}

// The F11 menu's visibility file is found through config.UserConfigDir, which
// on darwin ignores the XDG_CONFIG_HOME that testutil.Main points at the
// suite's own profile. Without the seam TestMain sets, this package and
// internal/panel -- separate processes under `go test ./...` -- wrote and
// removed the developer's real file, and each other's: a darwin cell failed
// TestSettingsCoreRecordProviderPluginMenuVisibilityRoundTrip on an empty list
// read back.
func TestTheSuiteKeepsToItsOwnProfile(t *testing.T) {
	dir := testutil.ConfigDir()
	if dir == "" {
		t.Skip("testutil.Main made no profile directory")
	}
	if path := panel.PluginMenuVisibilityFilePath(); !strings.HasPrefix(path, dir+string(filepath.Separator)) {
		t.Fatalf("the F11 menu visibility file is %s, outside the suite's own profile %s", path, dir)
	}
}

func TestMain(m *testing.M) {
	os.Exit(testutil.Main(m, func() {
		config.Executable = os.Executable
		// os.UserConfigDir ignores XDG_CONFIG_HOME on darwin, so the seam is
		// what keeps the suite off the developer's real profile there.
		if dir := testutil.ConfigDir(); dir != "" {
			config.UserConfigDir = func() (string, error) { return dir, nil }
		}
		config.CachedF4ConfigDir = testutil.ConfigDir()
		config.ConfigDirOnce.Do(func() {})
		Configure(testHost{})
		theme.SetDefaultF4Palette()
		InitLang()
	}, nil))
}
