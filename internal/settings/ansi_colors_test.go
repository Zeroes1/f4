package settings

import (
	"testing"

	"github.com/unxed/f4/internal/config"
)

func TestAnsiColorSettingsAreValidatedAndNormalized(t *testing.T) {
	cfg := config.DefaultConfig()
	if err := setCoreSetting(&cfg, "AnsiColorBlue", "#6699ff"); err != nil || cfg.AnsiColorBlue != "#6699FF" {
		t.Fatalf("a good color: %q, %v; want #6699FF", cfg.AnsiColorBlue, err)
	}
	if err := setCoreSetting(&cfg, "AnsiColorBlue", "bright"); err == nil || cfg.AnsiColorBlue != "#6699FF" {
		t.Fatalf("a bad color: %q, %v; want an error and the old value", cfg.AnsiColorBlue, err)
	}
	if err := setCoreSetting(&cfg, "AnsiColorBlue", ""); err != nil || cfg.AnsiColorBlue != "#000080" {
		t.Fatalf("an empty field: %q, %v; want the default #000080", cfg.AnsiColorBlue, err)
	}
}

func TestAnsiColorFieldsAreInTheTerminalCategory(t *testing.T) {
	found := map[string]bool{}
	for _, f := range (coreSettingsProvider{}).Catalog().Fields {
		if config.AnsiColorIndex(f.ID) >= 0 {
			found[f.ID] = f.Category == "terminal" && f.Timing == "live"
		}
	}
	for _, key := range config.AnsiColorKeys {
		if !found[key] {
			t.Errorf("%s is not a live Terminal & environment setting", key)
		}
	}
}
