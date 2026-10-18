package config

import (
	"fmt"
	"strings"
)

// TerminalColorKeys are the settings.ini keys (section Panel) and the
// F4Config field names of the colours of the embedded terminal itself
// (f4#2071): the default text and background, the caret and the mouse
// selection. Each one is #RRGGBB, or empty to keep what the theme draws.
var TerminalColorKeys = [5]string{
	"TerminalForeground", "TerminalBackground", "TerminalCursorColor",
	"TerminalSelectionText", "TerminalSelectionColor",
}

// TerminalColorKey reports whether id is one of TerminalColorKeys.
func TerminalColorKey(id string) bool {
	for _, key := range TerminalColorKeys {
		if key == id {
			return true
		}
	}
	return false
}

// NormalizeTerminalColor returns the canonical #RRGGBB of s, or "" when s is
// empty or not a colour.
func NormalizeTerminalColor(s string) string {
	if rgb, ok := ParseAnsiColor(s); ok {
		return FormatAnsiColor(rgb)
	}
	return ""
}

// TerminalColors are the colours of the embedded terminal cfg asks for; the
// ok half of a pair is false where the field is empty.
type TerminalColors struct {
	Foreground, Background, Cursor, SelectionForeground, SelectionBackground uint32
	HasForeground, HasBackground, HasCursor, HasSelectionFg, HasSelectionBg  bool
}

// TerminalColors reads the five terminal colour fields of cfg.
func (cfg F4Config) TerminalColors() TerminalColors {
	var c TerminalColors
	c.Foreground, c.HasForeground = ParseAnsiColor(cfg.TerminalForeground)
	c.Background, c.HasBackground = ParseAnsiColor(cfg.TerminalBackground)
	c.Cursor, c.HasCursor = ParseAnsiColor(cfg.TerminalCursorColor)
	c.SelectionForeground, c.HasSelectionFg = ParseAnsiColor(cfg.TerminalSelectionText)
	c.SelectionBackground, c.HasSelectionBg = ParseAnsiColor(cfg.TerminalSelectionColor)
	return c
}

// terminalColorFields points at the five fields of cfg, in TerminalColorKeys order.
func terminalColorFields(cfg *F4Config) [5]*string {
	return [5]*string{
		&cfg.TerminalForeground, &cfg.TerminalBackground, &cfg.TerminalCursorColor,
		&cfg.TerminalSelectionText, &cfg.TerminalSelectionColor,
	}
}

// loadTerminalColors reads the five colours from the Panel section of ini.
func loadTerminalColors(cfg *F4Config, get func(key, def string) string) {
	for i, p := range terminalColorFields(cfg) {
		*p = NormalizeTerminalColor(get(TerminalColorKeys[i], ""))
	}
}

// writeTerminalColors appends the five colours as settings.ini lines.
func writeTerminalColors(sb *strings.Builder, cfg F4Config) {
	for i, p := range terminalColorFields(&cfg) {
		fmt.Fprintf(sb, "%s = %s\n", TerminalColorKeys[i], NormalizeTerminalColor(*p))
	}
}
