package theme

import (
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestApplyAnsiPaletteUsesTheChosenColorsAndLetsGoOfThem(t *testing.T) {
	saved := config.App
	t.Cleanup(func() { config.App = saved; SetDefaultF4Palette() })

	config.App = config.DefaultConfig()
	SetDefaultF4Palette()
	ApplyAnsiPalette()
	for i := 0; i < 16; i++ {
		if vtui.ThemePalette[i] != config.DefaultAnsiColors[i] {
			t.Errorf("default color %d = %06X, want %06X", i, vtui.ThemePalette[i], config.DefaultAnsiColors[i])
		}
	}

	config.App.AnsiColorBlue = "#6699FF"
	config.App.AnsiColorBrightBlack = "#808080"
	SetDefaultF4Palette()
	ApplyAnsiPalette()
	if vtui.ThemePalette[4] != 0x6699FF || vtui.ThemePalette[8] != 0x808080 {
		t.Errorf("blue = %06X, bright black = %06X; want 6699FF and 808080", vtui.ThemePalette[4], vtui.ThemePalette[8])
	}
	if vtui.ThemePalette[1] != config.DefaultAnsiColors[1] {
		t.Errorf("red = %06X, want the default %06X", vtui.ThemePalette[1], config.DefaultAnsiColors[1])
	}

	config.App.AnsiColorBlue = "#000080"
	SetDefaultF4Palette()
	ApplyAnsiPalette()
	if vtui.ThemePalette[4] != 0x000080 {
		t.Errorf("blue after going back to the default = %06X, want 000080", vtui.ThemePalette[4])
	}
}
