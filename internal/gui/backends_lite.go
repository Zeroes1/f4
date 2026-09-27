//go:build lite

package gui

import "strings"

// BackendBuilt reports whether this binary carries the named GUI backend.
//
// The lite build keeps the backends that draw with nothing but the display
// server's own protocol: X11 and Wayland, and Win32 GDI on Windows. It is
// built with vtui's vtui_noebiten and vtui_nogogpu tags, which leave
// Ebitengine and gogpu, and the GPU stack behind them, out of the binary
// (f4#1178). liteguard.go refuses a lite build without those tags. External
// UI plugins (qt, ext:) run as their own processes and link nothing here,
// so they stay available.
func BackendBuilt(backend string) bool {
	switch strings.ToLower(strings.TrimSpace(backend)) {
	case "gogpu", "ebiten":
		return false
	}
	return true
}
