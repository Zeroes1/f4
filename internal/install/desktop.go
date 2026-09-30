package install

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/unxed/f4/internal/gui/assets/icon"
)

// Desktop integration (f4#1290). A single-file install has neither the
// .desktop entry nor the icons that the release archives carry, so the window
// of `f4 --gui` shows the window manager's default icon in the task bar. These
// files are written under the user's own XDG data directory: no sudo, and the
// desktop picks them up at once (or after the next login).

const (
	// DesktopFileName is the entry's name; on Wayland the window's application
	// ID (org.unxed.f4) is matched against it.
	DesktopFileName = "org.unxed.f4.desktop"
	// IconName is the Icon= value and the base name of every icon file.
	IconName = "io.github.unxed.f4"
)

// SupportsDesktop reports whether this system uses freedesktop .desktop files.
func SupportsDesktop() bool {
	switch runtime.GOOS {
	case "windows", "darwin", "ios", "android", "plan9", "js", "wasip1":
		return false
	}
	return true
}

// DataHome is the user's XDG data directory: $XDG_DATA_HOME when it is an
// absolute path, ~/.local/share otherwise.
func DataHome(home string, getenv func(string) string) string {
	if dir := getenv("XDG_DATA_HOME"); filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(home, ".local", "share")
}

// execQuote writes path as one argument of an Exec= line, the way the
// Desktop Entry Specification asks: a name with reserved characters is put in
// double quotes, with ", `, $ and \ escaped, and the backslash written four
// times because the string escape is applied first.
func execQuote(path string) string {
	path = strings.ReplaceAll(path, "%", "%%")
	if !strings.ContainsAny(path, " \t\n\"'\\><~|&;$*?#()`") {
		return path
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range path {
		switch r {
		case '\\':
			b.WriteString(`\\\\`)
		case '"', '`', '$':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// DesktopEntry is the launcher text for the f4 executable at exePath. It is the
// entry packaging/linux/org.unxed.f4.desktop ships, with the executable's full
// path (the desktop's PATH need not contain ~/.local/bin).
func DesktopEntry(exePath string) string {
	return "[Desktop Entry]\n" +
		"Version=1.0\n" +
		"Type=Application\n" +
		"Name=f4\n" +
		"GenericName=File Manager\n" +
		"Comment=Far Manager / far2l-style file manager\n" +
		"Exec=" + execQuote(exePath) + " --gui\n" +
		"TryExec=" + exePath + "\n" +
		"Icon=" + IconName + "\n" +
		"StartupWMClass=org.unxed.f4\n" +
		"Terminal=false\n" +
		"Categories=System;FileManager;\n"
}

// writeFileAtomic writes data next to path and renames it into place, so a
// desktop reading the file never sees half of it.
func writeFileAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".f4-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o644); err != nil { // #nosec G302 -- a launcher and icons are meant to be world-readable.
		_ = os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		_ = os.Remove(name)
		return err
	}
	return nil
}

// InstallDesktop writes the launcher for the executable at exePath and the
// application icons under dataHome, and returns the files written. Running it
// again rewrites the same files.
func InstallDesktop(dataHome, exePath string) ([]string, error) {
	var written []string
	put := func(path string, data []byte) error {
		if err := writeFileAtomic(path, data); err != nil {
			return err
		}
		written = append(written, path)
		return nil
	}

	if err := put(filepath.Join(dataHome, "applications", DesktopFileName), []byte(DesktopEntry(exePath))); err != nil {
		return written, err
	}
	svg, err := icon.Files.ReadFile("f4.svg")
	if err != nil {
		return written, err
	}
	if err := put(filepath.Join(dataHome, "icons", "hicolor", "scalable", "apps", IconName+".svg"), svg); err != nil {
		return written, err
	}
	for _, size := range icon.PNGSizes {
		png, err := icon.Files.ReadFile(fmt.Sprintf("generated/f4-%d.png", size))
		if err != nil {
			return written, err
		}
		dir := fmt.Sprintf("%dx%d", size, size)
		if err := put(filepath.Join(dataHome, "icons", "hicolor", dir, "apps", IconName+".png"), png); err != nil {
			return written, err
		}
	}
	return written, nil
}

// RunDesktopCLI serves `f4 --install-desktop`, and the desktop step of
// `f4 --install`: it makes the running f4 known to the desktop (launcher entry
// and icons in the user's XDG data directory), so the task bar shows f4's own
// icon. exePath is the executable the launcher starts. Returns the exit code.
func RunDesktopCLI(exePath string) int {
	if !SupportsDesktop() {
		fmt.Println("f4: desktop integration (a .desktop launcher and icons) is for Linux and BSD desktops; nothing to do here.")
		return 1
	}
	home, err := UserHomeDir()
	if err != nil {
		fmt.Printf("f4: could not determine your home directory: %v\n", err)
		return 1
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	if abs, err := filepath.Abs(exePath); err == nil {
		exePath = abs
	}
	dataHome := DataHome(home, Getenv)
	if _, err := InstallDesktop(dataHome, exePath); err != nil {
		fmt.Printf("f4: could not install the launcher and icons under %s: %v\n", dataHome, err)
		return 1
	}
	fmt.Printf("Installed the launcher %s and the icons under %s.\n",
		filepath.Join(dataHome, "applications", DesktopFileName), filepath.Join(dataHome, "icons", "hicolor"))
	fmt.Println("If the task bar still shows a default icon, close f4 and start it again (or log out and in once).")
	return 0
}
