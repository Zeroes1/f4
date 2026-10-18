package settings

import (
	"context"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

func TestDefaultTerminalColorsButtonResetsTheDraft(t *testing.T) {
	core, _ := (coreSettingsProvider{}).Begin(context.Background())
	defer core.Close()
	ops, _ := (settingsOperationsProvider{}).Begin(context.Background())
	defer ops.Close()
	c := newSettingsCenter([]*settingsSession{
		{catalog: (coreSettingsProvider{}).Catalog(), draft: core},
		{catalog: (settingsOperationsProvider{}).Catalog(), draft: ops},
	})
	core.Values["AnsiColorBlue"] = "#6699FF"
	core.Values["TerminalBackground"] = "#101010"
	core.Values["TerminalSelectionColor"] = "#A0B0C0"
	c.selectCategory("terminal")

	var button *vtui.Button
	for _, r := range c.page.rows {
		if b, ok := r.control.(*vtui.Button); ok && b.GetId() == "settings-command:"+terminalColorDefaultsCommand {
			button = b
		}
	}
	if button == nil {
		t.Fatal("no Default terminal colors button on the Terminal & environment page")
	}
	button.OnClick()

	if got := core.Values["AnsiColorBlue"]; got != "#000080" {
		t.Errorf("ANSI blue in the draft = %q, want the default #000080", got)
	}
	for _, key := range config.TerminalColorKeys {
		if got := core.Values[key]; got != "" {
			t.Errorf("%s in the draft = %q, want it empty", key, got)
		}
	}
	for _, r := range c.page.rows {
		if r.field.ID != "AnsiColorBlue" {
			continue
		}
		if e, ok := r.control.(*settingsEdit); !ok || e.GetText() != "#000080" {
			t.Errorf("the ANSI blue field shows %#v, want #000080", r.control)
		}
	}
	if config.App.AnsiColorBlue == "#6699FF" {
		t.Error("the button changed the applied settings; only Apply should")
	}
}
