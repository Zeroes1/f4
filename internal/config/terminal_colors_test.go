package config

import "testing"

func TestTerminalColorsAreStoredAndEmptyMeansTheTheme(t *testing.T) {
	if c := DefaultConfig().TerminalColors(); c.HasForeground || c.HasBackground || c.HasCursor || c.HasSelectionFg || c.HasSelectionBg {
		t.Fatalf("the default config has terminal colors: %+v", c)
	}
	cfg := WithOption(DefaultConfig(), "Panel", "TerminalBackground", "#0a0b0c")
	if cfg.TerminalBackground != "#0A0B0C" {
		t.Fatalf("TerminalBackground = %q, want #0A0B0C", cfg.TerminalBackground)
	}
	if c := cfg.TerminalColors(); !c.HasBackground || c.Background != 0x0A0B0C || c.HasForeground {
		t.Fatalf("TerminalColors() = %+v", c)
	}
	found := false
	for _, option := range Options(cfg) {
		if option.Section == "Panel" && option.Key == "TerminalBackground" {
			found = option.Value == "#0A0B0C"
		}
	}
	if !found {
		t.Error("TerminalBackground = #0A0B0C is not among the written settings")
	}
	if bad := WithOption(cfg, "Panel", "TerminalBackground", "dark"); bad.TerminalBackground != "" {
		t.Errorf("a bad value kept %q, want an empty field", bad.TerminalBackground)
	}
	for _, key := range TerminalColorKeys {
		if !TerminalColorKey(key) {
			t.Errorf("TerminalColorKey(%s) = false", key)
		}
	}
	if TerminalColorKey("AnsiColorBlack") {
		t.Error("AnsiColorBlack is not a terminal color key")
	}
}
