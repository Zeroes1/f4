package config

import (
	"fmt"
	"strconv"
	"strings"
)

// AnsiColorKeys are the settings.ini keys (section Panel) and the F4Config
// field names of the 16 basic ANSI colours, in palette order: the eight
// normal colours first, then their bright variants (f4#2071).
var AnsiColorKeys = [16]string{
	"AnsiColorBlack", "AnsiColorRed", "AnsiColorGreen", "AnsiColorYellow",
	"AnsiColorBlue", "AnsiColorMagenta", "AnsiColorCyan", "AnsiColorWhite",
	"AnsiColorBrightBlack", "AnsiColorBrightRed", "AnsiColorBrightGreen", "AnsiColorBrightYellow",
	"AnsiColorBrightBlue", "AnsiColorBrightMagenta", "AnsiColorBrightCyan", "AnsiColorBrightWhite",
}

// DefaultAnsiColors is the palette f4 loads into the terminal when the user
// has not chosen colours: the xterm table, except for the three entries the
// f4 theme sets itself (black, white and bright black).
var DefaultAnsiColors = [16]uint32{
	0x2E3436, 0x800000, 0x008000, 0x808000, 0x000080, 0x800080, 0x008080, 0xD3D7CF,
	0x555753, 0xFF0000, 0x00FF00, 0xFFFF00, 0x0000FF, 0xFF00FF, 0x00FFFF, 0xFFFFFF,
}

// FormatAnsiColor writes rgb the way the settings keep it: #RRGGBB.
func FormatAnsiColor(rgb uint32) string {
	return fmt.Sprintf("#%06X", rgb&0xFFFFFF)
}

// ParseAnsiColor reads #RRGGBB (the # is optional, case does not matter).
func ParseAnsiColor(s string) (uint32, bool) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 0, false
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

// NormalizeAnsiColor returns the canonical #RRGGBB of s, or that of the
// default colour number index when s is empty or not a colour.
func NormalizeAnsiColor(s string, index int) string {
	if rgb, ok := ParseAnsiColor(s); ok {
		return FormatAnsiColor(rgb)
	}
	return FormatAnsiColor(DefaultAnsiColors[index])
}

// ansiColorFields points at the 16 colour fields of cfg, in palette order.
func ansiColorFields(cfg *F4Config) [16]*string {
	return [16]*string{
		&cfg.AnsiColorBlack, &cfg.AnsiColorRed, &cfg.AnsiColorGreen, &cfg.AnsiColorYellow,
		&cfg.AnsiColorBlue, &cfg.AnsiColorMagenta, &cfg.AnsiColorCyan, &cfg.AnsiColorWhite,
		&cfg.AnsiColorBrightBlack, &cfg.AnsiColorBrightRed, &cfg.AnsiColorBrightGreen, &cfg.AnsiColorBrightYellow,
		&cfg.AnsiColorBrightBlue, &cfg.AnsiColorBrightMagenta, &cfg.AnsiColorBrightCyan, &cfg.AnsiColorBrightWhite,
	}
}

// AnsiColors is the 16-colour palette cfg asks for. A colour that is empty or
// not #RRGGBB is the default one.
func (cfg F4Config) AnsiColors() [16]uint32 {
	fields := ansiColorFields(&cfg)
	var out [16]uint32
	for i, p := range fields {
		rgb, ok := ParseAnsiColor(*p)
		if !ok {
			rgb = DefaultAnsiColors[i]
		}
		out[i] = rgb
	}
	return out
}

// loadAnsiColors reads the 16 colours from the Panel section of ini.
func loadAnsiColors(cfg *F4Config, get func(key, def string) string) {
	for i, p := range ansiColorFields(cfg) {
		*p = NormalizeAnsiColor(get(AnsiColorKeys[i], FormatAnsiColor(DefaultAnsiColors[i])), i)
	}
}

// writeAnsiColors appends the 16 colours as settings.ini lines.
func writeAnsiColors(sb *strings.Builder, cfg F4Config) {
	for i, p := range ansiColorFields(&cfg) {
		fmt.Fprintf(sb, "%s = %s\n", AnsiColorKeys[i], NormalizeAnsiColor(*p, i))
	}
}

// AnsiColorIndex is the palette index of the setting id, or -1 if id is not
// one of AnsiColorKeys.
func AnsiColorIndex(id string) int {
	for i, key := range AnsiColorKeys {
		if key == id {
			return i
		}
	}
	return -1
}
