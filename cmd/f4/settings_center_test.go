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
	"github.com/unxed/vtinput"
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
		c.SetFocusedItem(c.page)
		c.Show(scr)
		x, y, _, _ = match.control.GetPosition()
		if got := scr.GetCell(x, y).Attributes; got != vtui.Palette[vtui.ColDialogSelectedButton] {
			t.Fatalf("focused theme %d got %x", iteration, got)
		}
		c.ResizeConsole(130, 35)
		c.SetPosition(0, 0, 129, 34)
		scr.AllocBuf(130, 35)
		c.Show(scr)
		if c.help.X1 <= c.page.X2 {
			t.Fatal("wide explanation pane overlaps settings")
		}
		c.ResizeConsole(80, 25)
		scr.AllocBuf(80, 25)
	}
}

func TestSettingsCenterCompactCheckboxesAndResizableLayout(t *testing.T) {
	d, _ := (coreSettingsProvider{}).Begin(context.Background())
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (coreSettingsProvider{}).Catalog(), draft: d}})
	c.ResizeConsole(180, 60)
	if c.X1 != 45 || c.X2 != 134 || c.Y2-c.Y1+1 != 45 || !c.ShowZoom {
		t.Fatalf("unexpected initial bounds: %d,%d–%d,%d", c.X1, c.Y1, c.X2, c.Y2)
	}
	c.selectCategory("panels")
	for _, row := range c.page.rows {
		if row.field.Kind != f4settings.Boolean {
			continue
		}
		b, ok := row.control.(*settingsCheckbox)
		if !ok || len(row.label) != 0 || row.height != len(b.lines) || strings.Join(b.lines, " ") != row.field.Label.Resolve(AppConfig.Language, Msg) {
			t.Fatalf("checkbox %s has a redundant label or spacing", row.field.ID)
		}
		before := d.Values[row.field.ID]
		b.Toggle()
		if d.Values[row.field.ID] == before {
			t.Fatal("checkbox did not edit draft")
		}
		break
	}
	c.ChangeSize(130, 45)
	c.syncWindowBounds()
	if c.help.X1 <= c.page.X2 || c.page.X1 <= c.X1 {
		t.Fatal("resize did not lay out columns")
	}
	palette := append([]uint64(nil), vtui.Palette...)
	defer copy(vtui.Palette, palette)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(180, 60)
	for iteration := 0; iteration < 2; iteration++ {
		for j, id := range []int{vtui.ColDialogBox, vtui.ColDialogBoxTitle, vtui.ColDialogEditSelected, vtui.ColDialogSelectedButton} {
			vtui.Palette[id] = uint64(0x31 + j + iteration*16)
		}
		c.SetFocusedItem(c.page)
		c.query = ""
		c.updateMatches()
		c.Show(scr)
		for _, x := range []int{c.sidebar.X2 + 1, c.help.X1 - 1, c.page.X1} {
			if scr.GetCell(x, c.page.Y1).Attributes != vtui.Palette[vtui.ColDialogBox] {
				t.Fatal("separator/group border did not follow palette")
			}
		}
		row := settingsCategoryRow{center: c, category: f4settings.Category{ID: "panels"}}
		if row.GetCellAttr(0, 0) != settingsInactiveCategoryAttr(vtui.Palette[vtui.ColDialogText]) {
			t.Fatal("inactive category selection missing")
		}
		c.SetFocusedItem(c.sidebar)
		if row.GetCellAttr(0, vtui.Palette[vtui.ColDialogSelectedButton]) != vtui.Palette[vtui.ColDialogSelectedButton] {
			t.Fatal("active category selection lost")
		}
		c.query = "no-such-setting-xyz"
		c.updateMatches()
		c.Show(scr)
		if scr.GetCell(c.page.X1, c.page.Y1).Attributes != vtui.DimColor(vtui.Palette[vtui.ColDialogBox]) {
			t.Fatal("unmatched group border not dimmed")
		}
	}
	c.ResizeConsole(80, 25)
	if c.X1 < 0 || c.Y1 < 0 || c.X2 >= 80 || c.Y2 >= 25 {
		t.Fatal("resize left window outside screen")
	}
	c.ProcessMouse(&vtinput.InputEvent{MouseX: int16(c.X2), MouseY: int16(c.Y2), KeyDown: true, ButtonState: vtinput.FromLeft1stButtonPressed})
	c.ProcessMouse(&vtinput.InputEvent{MouseX: int16(c.X1 + 71), MouseY: int16(c.Y1 + 21), ButtonState: vtinput.FromLeft1stButtonPressed})
	c.ProcessMouse(&vtinput.InputEvent{})
	if c.resizing || c.X2-c.X1+1 != 72 || c.Y2-c.Y1+1 != 22 {
		t.Fatal("mouse resizing into content was intercepted")
	}
}

func TestSettingsCenterExclusivePaneFocus(t *testing.T) {
	d, _ := (coreSettingsProvider{}).Begin(context.Background())
	defer d.Close()
	c := newSettingsCenter([]*settingsSession{{catalog: (coreSettingsProvider{}).Catalog(), draft: d}})
	c.ResizeConsole(130, 35)
	c.SetPosition(0, 0, 129, 34)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(130, 35)
	palette := append([]uint64(nil), vtui.Palette...)
	defer copy(vtui.Palette, palette)
	for iteration := 0; iteration < 2; iteration++ {
		vtui.Palette[vtui.ColDialogText] = vtui.SetRGBBoth(0, 0xeeeeee, uint32(0x201040+iteration*0x102030))
		vtui.Palette[vtui.ColDialogSelectedButton] = vtui.SetRGBBoth(0, 0xffffff, 0x0000ff)
		c.SetFocusedItem(c.sidebar)
		c.category = ""
		c.selectCategory("panels")
		for i, cat := range c.categories {
			if cat.ID == "panels" {
				c.sidebar.SetSelectPos(i)
			}
		}
		c.Show(scr)
		control := c.page.GetFocusedItem()
		x, y, _, _ := control.GetPosition()
		if control.IsFocused() || scr.GetCell(x, y).Attributes != vtui.Palette[vtui.ColDialogText] {
			t.Fatal("inactive content displays keyboard focus")
		}
		c.SetFocusedItem(c.page)
		c.Show(scr)
		if !control.IsFocused() || scr.GetCell(x, y).Attributes != vtui.Palette[vtui.ColDialogSelectedButton] {
			t.Fatal("active content lost keyboard focus")
		}
		categoryY := c.sidebar.Y1 + c.sidebar.SelectPos - c.sidebar.TopPos
		attr := scr.GetCell(c.sidebar.X1, categoryY).Attributes
		bg := vtui.GetRGBBack(attr)
		if attr == vtui.Palette[vtui.ColDialogSelectedButton] || (bg>>16)&255 != (bg>>8)&255 || (bg>>8)&255 != bg&255 {
			t.Fatalf("inactive category isn't neutral gray: %x", attr)
		}
		c.rebuildCategory()
		c.Show(scr)
		if !c.page.GetFocusedItem().IsFocused() {
			t.Fatal("rebuilding active content lost focus")
		}
		c.SetFocusedItem(c.sidebar)
		c.Show(scr)
		if c.page.GetFocusedItem().IsFocused() {
			t.Fatal("returning to sidebar left content focused")
		}
	}
}

func TestSettingsActionCaptionsAppearOnce(t *testing.T) {
	d := f4settings.NewDraft(nil, nil)
	defer d.Close()
	label := "Save applied preferences"
	cat := f4settings.Catalog{ID: "test", Categories: settingsCategories, Commands: []f4settings.Command{{ID: "test.save", Category: "workspaces", Group: "Manual saving", Label: f4settings.Text{English: label}, Description: f4settings.Text{English: "Save the applied configuration."}}}}
	c := newSettingsCenter([]*settingsSession{{catalog: cat, draft: d}})
	c.selectCategory("workspaces")
	c.ResizeConsole(130, 35)
	c.SetPosition(0, 0, 129, 34)
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(130, 35)
	old := append([]uint64(nil), vtui.Palette...)
	defer copy(vtui.Palette, old)
	row := c.page.rows[0]
	if len(row.label) != 0 || row.height != 2 {
		t.Fatal("action has duplicate label or redundant label space")
	}
	for iteration := 0; iteration < 2; iteration++ {
		vtui.Palette[vtui.ColDialogBoxTitle] = uint64(0x15 + iteration*16)
		vtui.Palette[vtui.ColDialogButton] = uint64(0x12 + iteration*16)
		vtui.Palette[vtui.ColDialogSelectedButton] = uint64(0x13 + iteration*16)
		c.SetFocusedItem(c.sidebar)
		c.Show(scr)
		title := c.categoryLabel(c.category)
		titleX := c.page.X1 + (c.page.X2-c.page.X1+1-vtui.StringWidth(title))/2
		for i, ch := range title {
			cell := scr.GetCell(titleX+i, c.sidebar.Y1)
			if rune(cell.Char) != ch || cell.Attributes != vtui.Palette[vtui.ColDialogBoxTitle] {
				t.Fatal("category title is not centered or does not follow the palette")
			}
		}
		x, y, _, _ := row.control.GetPosition()
		if scr.GetCell(x, y).Attributes != vtui.Palette[vtui.ColDialogButton] {
			t.Fatal("normal action palette mismatch")
		}
		c.SetFocusedItem(c.page)
		c.Show(scr)
		if scr.GetCell(x, y).Attributes != vtui.Palette[vtui.ColDialogSelectedButton] {
			t.Fatal("focused action palette mismatch")
		}
		var rendered strings.Builder
		for line := c.page.Y1; line <= c.page.Y2; line++ {
			for col := c.page.X1; col <= c.page.X2; col++ {
				rendered.WriteRune(rune(scr.GetCell(col, line).Char))
			}
			rendered.WriteByte('\n')
		}
		if strings.Count(rendered.String(), label) != 1 {
			t.Fatalf("action caption must render once: %q", rendered.String())
		}
		c.describe(row)
		if !strings.Contains(c.help.text, "Save the applied configuration.") {
			t.Fatal("action explanation lost")
		}
		c.query = label
		c.updateMatches()
		if !row.match {
			t.Fatal("action label no longer searchable")
		}
	}
}
