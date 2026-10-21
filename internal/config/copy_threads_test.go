package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClampCopyThreads(t *testing.T) {
	for in, want := range map[int]int{-5: 1, 0: 1, 1: 1, 2: 2, 16: 16, 17: 16, 1000: 16} {
		if got := ClampCopyThreads(in); got != want {
			t.Errorf("ClampCopyThreads(%d) = %d, want %d", in, got, want)
		}
	}
}

func loadCopyThreads(t *testing.T, panelSection string) int {
	t.Helper()
	ini := filepath.Join(t.TempDir(), "settings.ini")
	if err := os.WriteFile(ini, []byte("[Panel]\n"+panelSection), 0o600); err != nil {
		t.Fatal(err)
	}
	origPaths := GetConfigIniPaths
	oldCfg := App
	defer func() { GetConfigIniPaths = origPaths; App = oldCfg }()
	GetConfigIniPaths = func() []string { return []string{ini} }
	App.CopyThreads = 99
	LoadConfig()
	return App.CopyThreads
}

// An absent, unreadable or out-of-range CopyThreads leaves the single-file
// path in force; only a number from 2 to 16 asks for more.
func TestConfig_CopyThreads(t *testing.T) {
	for _, tc := range []struct {
		line string
		want int
	}{
		{"", 1},
		{"CopyThreads = 1\n", 1},
		{"CopyThreads = 4\n", 4},
		{"CopyThreads = 0\n", 1},
		{"CopyThreads = -3\n", 1},
		{"CopyThreads = 64\n", 16},
		{"CopyThreads = many\n", 1},
	} {
		if got := loadCopyThreads(t, tc.line); got != tc.want {
			t.Errorf("settings %q: CopyThreads = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestConfig_CopyThreadsDefaultsToOne(t *testing.T) {
	if got := loadCopyThreads(t, ""); got != 1 {
		t.Fatalf("CopyThreads without a setting = %d, want 1", got)
	}
}

func TestConfig_CopyThreadsIsSaved(t *testing.T) {
	cfg := App
	cfg.CopyThreads = 4
	if out := string(SerializeSettingsConfig(cfg)); !strings.Contains(out, "CopyThreads = 4\n") {
		t.Fatalf("saved settings do not carry CopyThreads:\n%s", out)
	}
}
