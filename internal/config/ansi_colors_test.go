package config

import "testing"

func TestParseAnsiColor(t *testing.T) {
	for in, want := range map[string]uint32{"#3465a4": 0x3465A4, "3465A4": 0x3465A4, " #000000 ": 0} {
		if got, ok := ParseAnsiColor(in); !ok || got != want {
			t.Errorf("ParseAnsiColor(%q) = %06X, %v; want %06X", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "#fff", "#12345", "#1234567", "#12345g", "blue"} {
		if _, ok := ParseAnsiColor(in); ok {
			t.Errorf("ParseAnsiColor(%q) accepted a bad color", in)
		}
	}
}

func TestNormalizeAnsiColorFallsBackToTheDefaultOfThatIndex(t *testing.T) {
	if got := NormalizeAnsiColor("#abcdef", 3); got != "#ABCDEF" {
		t.Errorf("a good color = %q, want #ABCDEF", got)
	}
	if got := NormalizeAnsiColor("nonsense", 7); got != "#D3D7CF" {
		t.Errorf("a bad color = %q, want the default of white, #D3D7CF", got)
	}
}

func TestAnsiColorsDefaultToTheBuiltInPalette(t *testing.T) {
	if got := DefaultConfig().AnsiColors(); got != DefaultAnsiColors {
		t.Errorf("default colors = %06X, want %06X", got, DefaultAnsiColors)
	}
}

func TestAnsiColorsAreStoredInSettings(t *testing.T) {
	cfg := WithOption(DefaultConfig(), "Panel", "AnsiColorBlue", "#6699ff")
	if cfg.AnsiColorBlue != "#6699FF" || cfg.AnsiColors()[4] != 0x6699FF {
		t.Fatalf("AnsiColorBlue = %q, colors[4] = %06X; want #6699FF", cfg.AnsiColorBlue, cfg.AnsiColors()[4])
	}
	found := false
	for _, option := range Options(cfg) {
		if option.Section == "Panel" && option.Key == "AnsiColorBlue" {
			found = option.Value == "#6699FF"
		}
	}
	if !found {
		t.Error("AnsiColorBlue = #6699FF is not among the written settings")
	}
	if bad := WithOption(cfg, "Panel", "AnsiColorBlue", "bright"); bad.AnsiColorBlue != "#000080" {
		t.Errorf("a bad value kept %q, want the default #000080", bad.AnsiColorBlue)
	}
	if got := len(AnsiColorKeys); got != 16 {
		t.Fatalf("%d color keys", got)
	}
	for i, key := range AnsiColorKeys {
		if AnsiColorIndex(key) != i {
			t.Errorf("AnsiColorIndex(%s) = %d, want %d", key, AnsiColorIndex(key), i)
		}
	}
	if AnsiColorIndex("PromptFormat") != -1 {
		t.Error("PromptFormat is not a color key")
	}
}
