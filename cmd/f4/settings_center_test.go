package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func TestSettingsCatalogComplete(t *testing.T) {
	c := (coreSettingsProvider{}).Catalog()
	if err := f4settings.ValidateCatalog(c); err != nil {
		t.Fatal(err)
	}
	if len(c.Fields) < 120 {
		t.Fatalf("unexpectedly small catalog: %d", len(c.Fields))
	}
	for _, f := range c.Fields {
		if !strings.HasPrefix(f.ID, "DriveMenuOptions.") && !settingsConfigField(reflect.ValueOf(AppConfig), f.ID).IsValid() {
			t.Fatalf("unknown config field %s", f.ID)
		}
		_ = coreSettingValue(AppConfig, f.ID)
	}
}

func TestSettingsApplyPreservesUnknownKeysAndRuntimeChanges(t *testing.T) {
	old := AppConfig
	pathFunc := getUserConfigIniPath
	defer func() { AppConfig = old; getUserConfigIniPath = pathFunc }()
	path := filepath.Join(t.TempDir(), "settings.ini")
	getUserConfigIniPath = func() string { return path }
	if err := os.WriteFile(path, []byte("[FutureFrontend]\nUnknown = preserve\n[Interface]\nGuiBackend = qt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	AppConfig.GuiBackend = "ext:qt"
	d, err := (coreSettingsProvider{}).Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.Values["ShowHiddenFiles"] = map[bool]string{true: "false", false: "true"}[AppConfig.ShowHiddenFiles]
	AppConfig.GuiCols = 143
	if r := d.Commit(context.Background()); len(r.Errors) > 0 {
		t.Fatal(r.Errors)
	}
	if AppConfig.GuiCols != 143 || AppConfig.GuiBackend != "ext:qt" {
		t.Fatal("unrelated configuration overwritten")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "Unknown = preserve") {
		t.Fatal("unknown setting removed")
	}
}

func TestSettingsFailedSaveRetainsDraft(t *testing.T) {
	old := AppConfig
	writer := writeSettingsCandidate
	defer func() { AppConfig = old; writeSettingsCandidate = writer }()
	writeSettingsCandidate = func(F4Config, F4Config) error { return errors.New("read-only disk") }
	d, _ := (coreSettingsProvider{}).Begin(context.Background())
	defer d.Close()
	d.Values["ShowHiddenFiles"] = "true"
	AppConfig.ShowHiddenFiles = false
	d.Baseline["ShowHiddenFiles"] = "false"
	r := d.Commit(context.Background())
	if len(r.Errors) == 0 || !d.Dirty("ShowHiddenFiles") || AppConfig.ShowHiddenFiles {
		t.Fatal("failed save published changes")
	}
}

func TestSettingsCenterRenderThemeAndSearch(t *testing.T) {
	oldPalette := append([]uint64(nil), vtui.Palette...)
	defer copy(vtui.Palette, oldPalette)
	d, _ := (coreSettingsProvider{}).Begin(context.Background())
	c := newSettingsCenter([]*settingsSession{{catalog: (coreSettingsProvider{}).Catalog(), draft: d}})
	defer d.Close()
	c.selectCategory("panels")
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	for iteration := 0; iteration < 2; iteration++ {
		for j, id := range []int{vtui.ColDialogText, vtui.ColDialogSelectedButton, vtui.ColDialogHighlightText, vtui.ColDialogHighlightSelectedButton, vtui.ColDialogBox} {
			vtui.Palette[id] = uint64(0x10 + j + iteration*16)
		}
		c.query = "hidden"
		c.updateMatches()
		c.Show(scr)
		var match, other *settingsRow
		for _, r := range c.page.rows {
			if r.field.ID == "ShowHiddenFiles" {
				match = r
			}
			if r.field.ID == "ShowDirPrefix" {
				other = r
			}
		}
		if match == nil || other == nil || !match.match || other.match || other.control.IsDisabled() {
			t.Fatal("search changed availability or mismatched rows")
		}
		x, y, _, _ := other.control.GetPosition()
		if y <= c.page.Y2 {
			want := vtui.DimColor(vtui.Palette[vtui.ColDialogText])
			if got := scr.GetCell(x, y).Attributes; got != want {
				t.Fatalf("theme %d dim attr %x, want %x", iteration, got, want)
			}
		}
		c.query = ""
		c.updateMatches()
		c.page.SetFocusedItem(match.control)
		c.Show(scr)
		x, y, _, _ = match.control.GetPosition()
		if got := scr.GetCell(x, y).Attributes; got != vtui.Palette[vtui.ColDialogSelectedButton] {
			t.Fatalf("focused theme %d got %x", iteration, got)
		}
		c.ResizeConsole(130, 35)
		scr.AllocBuf(130, 35)
		c.Show(scr)
		if c.help.X1 <= c.page.X2 {
			t.Fatal("wide explanation pane overlaps settings")
		}
		c.ResizeConsole(80, 25)
		scr.AllocBuf(80, 25)
	}
}
