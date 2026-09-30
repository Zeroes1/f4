package install

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/gui/assets/icon"
)

func TestDataHome(t *testing.T) {
	get := func(v string) func(string) string {
		return func(k string) string {
			if k == "XDG_DATA_HOME" {
				return v
			}
			return ""
		}
	}
	if got := DataHome("/home/u", get("")); got != filepath.Join("/home/u", ".local", "share") {
		t.Errorf("default = %q", got)
	}
	abs := filepath.Join(t.TempDir(), "data")
	if got := DataHome("/home/u", get(abs)); got != abs {
		t.Errorf("XDG_DATA_HOME = %q", got)
	}
	if got := DataHome("/home/u", get("relative/dir")); got != filepath.Join("/home/u", ".local", "share") {
		t.Errorf("a relative XDG_DATA_HOME must be ignored, got %q", got)
	}
}

func TestExecQuote(t *testing.T) {
	cases := map[string]string{
		"/home/u/.local/bin/f4":   "/home/u/.local/bin/f4",
		"/opt/my apps/f4":         `"/opt/my apps/f4"`,
		"/opt/50%/f4":             "/opt/50%%/f4",
		`/opt/a"b/f4`:             `"/opt/a\"b/f4"`,
		"/opt/$HOME/f4":           `"/opt/\$HOME/f4"`,
		`/opt/back` + `\` + `/f4`: `"/opt/back\\\\/f4"`,
	}
	for in, want := range cases {
		if got := execQuote(in); got != want {
			t.Errorf("execQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

// The launcher this package writes must not drift from the one the release
// archives carry: the two differ only in the executable's path.
func TestDesktopEntryMatchesThePackagedOne(t *testing.T) {
	packaged, err := os.ReadFile(filepath.Join("..", "..", "packaging", "linux", DesktopFileName))
	if err != nil {
		t.Fatal(err)
	}
	strip := func(text string) string {
		var keep []string
		for _, line := range strings.Split(text, "\n") {
			if strings.HasPrefix(line, "Exec=") || strings.HasPrefix(line, "TryExec=") {
				continue
			}
			keep = append(keep, line)
		}
		return strings.Join(keep, "\n")
	}
	got := DesktopEntry("/x/f4")
	if strip(got) != strip(string(packaged)) {
		t.Errorf("the generated launcher differs from packaging/linux/%s:\n%s\nvs\n%s", DesktopFileName, got, packaged)
	}
	if !strings.Contains(got, "Exec=/x/f4 --gui\n") || !strings.Contains(got, "TryExec=/x/f4\n") {
		t.Errorf("Exec/TryExec lines missing:\n%s", got)
	}
}

func TestInstallDesktopWritesLauncherAndIcons(t *testing.T) {
	data := t.TempDir()
	written, err := InstallDesktop(data, "/opt/f4 dir/f4")
	if err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(data, "applications", DesktopFileName)
	text, err := os.ReadFile(launcher) // #nosec G304 -- a path inside the test's own temp dir.
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(text), `Exec="/opt/f4 dir/f4" --gui`) || !strings.Contains(string(text), "StartupWMClass=org.unxed.f4") {
		t.Errorf("launcher:\n%s", text)
	}
	for _, rel := range []string{
		filepath.Join("icons", "hicolor", "scalable", "apps", IconName+".svg"),
		filepath.Join("icons", "hicolor", "48x48", "apps", IconName+".png"),
		filepath.Join("icons", "hicolor", "512x512", "apps", IconName+".png"),
	} {
		info, err := os.Stat(filepath.Join(data, rel))
		if err != nil || info.Size() == 0 {
			t.Errorf("%s: %v", rel, err)
		}
	}
	if len(written) != 2+len(pngSizesForTest()) {
		t.Errorf("wrote %d files: %v", len(written), written)
	}
	// Again: the same files, no leftovers.
	if _, err := InstallDesktop(data, "/opt/f4 dir/f4"); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(filepath.Join(data, "applications"))
	if len(entries) != 1 {
		t.Errorf("applications/ holds %d entries after a second install", len(entries))
	}
	if runtime.GOOS != "windows" {
		if info, _ := os.Stat(launcher); info.Mode().Perm() != 0o644 {
			t.Errorf("launcher mode = %v", info.Mode().Perm())
		}
	}
}

func TestInstallDesktopReportsAnUnwritableDirectory(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallDesktop(blocker, "/x/f4"); err == nil {
		t.Error("no error when the data directory is a file")
	}
}

func TestRunDesktopCLIInstallsIntoTheUsersDataDirectory(t *testing.T) {
	if !SupportsDesktop() {
		t.Skip("no freedesktop launchers on this OS")
	}
	home := t.TempDir()
	oldHome, oldGetenv := UserHomeDir, Getenv
	UserHomeDir = func() (string, error) { return home, nil }
	Getenv = func(string) string { return "" }
	t.Cleanup(func() { UserHomeDir, Getenv = oldHome, oldGetenv })
	if code := RunDesktopCLI(filepath.Join(home, "bin", "f4")); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "share", "applications", DesktopFileName)); err != nil {
		t.Error(err)
	}
}

func pngSizesForTest() []int { return icon.PNGSizes }
