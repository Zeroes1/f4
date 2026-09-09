package main

import (
	"fmt"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"strings"
	"testing"
)

func TestSettingsSidebarSearchAndContentSurface(t *testing.T) {
	oldConfig := AppConfig
	defer func() { AppConfig = oldConfig; InitLang() }()
	palette := append([]uint64(nil), vtui.Palette...)
	defer func() { vtui.Palette = palette }()
	for _, language := range []string{"en", "ru"} {
		AppConfig.Language = language
		AppConfig.UseLocalLanguageFiles = false
		InitLang()
		fields := []f4settings.Field{
			f4settings.Scalar("enabled", "startup", "Defaults", "Enabled", "Enable this behavior.", f4settings.Boolean),
			f4settings.Scalar("path", "startup", "Defaults", "Path", "Set path.", f4settings.String),
		}
		d := f4settings.NewDraft(map[string]string{"enabled": "true", "path": "example"}, nil)
		defer d.Close()
		c := newSettingsCenter([]*settingsSession{{catalog: f4settings.Catalog{ID: "test", Categories: settingsCategories, Fields: fields}, draft: d}})
		c.selectCategory("startup")
		scr := vtui.NewSilentScreenBuf()
		for iteration, size := range [][2]int{{80, 25}, {150, 40}, {80, 25}} {
			scr.AllocBuf(size[0], size[1])
			c.SetPosition(0, 0, size[0]-1, size[1]-1)
			for _, slot := range []int{vtui.ColDialogText, vtui.ColDialogBox, vtui.ColDialogBoxTitle} {
				vtui.Palette[slot] = vtui.SetRGBBoth(0, uint32(0xa0b0c0+slot), 0x505050)
			}
			vtui.Palette[ColDialogSettingsBackground] = vtui.SetRGBBack(0, uint32(0x303030+iteration))
			vtui.Palette[vtui.ColDialogEdit] = vtui.SetRGBBoth(0, 0xffffff, 0x101010)
			vtui.Palette[vtui.ColDialogEditUnchanged] = vtui.Palette[vtui.ColDialogEdit]
			vtui.Palette[vtui.ColDialogSelectedButton] = vtui.SetRGBBoth(0, 0xffffff, uint32(0x123456+iteration))
			vtui.Palette[vtui.ColDialogIndicatorBackground] = 0
			sideWidth := c.sidebar.X2 - c.sidebar.X1 + 1
			for _, query := range []string{"", "Enabled", "no-match-xyz", " "} {
				c.search.SetText(query)
				c.search.OnTextChange(query)
				c.SetFocusedItem(c.sidebar)
				c.Show(scr)
				searching := strings.TrimSpace(query) != ""
				if c.sidebar.X2-c.sidebar.X1+1 != sideWidth {
					t.Fatal("search resized sidebar")
				}
				if c.sidebar.Y1 != c.Y1+4 {
					t.Fatal("blank row after search separator")
				}
				if c.sidebar.Y2 != c.apply.Y1-1 || c.help.Y2 != c.apply.Y1-1 {
					t.Fatal("unused space above buttons")
				}
				if c.help.X1 > c.page.X2 && c.help.Y1 != c.Y1+1 {
					t.Fatal("help starts below search label")
				}
				if scr.GetCell(c.sidebar.X2+1, c.Y1+1).Char != '│' || scr.GetCell(c.sidebar.X2+1, c.apply.Y1-1).Char != '│' {
					t.Fatal("column separator does not span the pane")
				}

				if c.previous.IsVisible() != searching || c.next.IsVisible() != searching {
					t.Fatal("search arrows visibility")
				}
				if c.search.X1 != c.sidebar.X1 || c.search.X2 > c.sidebar.X2 || c.search.Y1 >= c.sidebar.Y1 {
					t.Fatal("search not inside sidebar")
				}
				if searching && (c.search.X2 >= c.previous.X1 || c.next.X2 != c.sidebar.X2 || c.next.Y1 != c.search.Y1 || c.next.X2-c.next.X1+1 != 3) {
					t.Fatal("compact search arrows placement")
				}
				var separator strings.Builder
				for x := c.sidebar.X1; x <= c.sidebar.X2; x++ {
					separator.WriteRune(rune(scr.GetCell(x, c.Y1+3).Char))
				}
				if searching {
					count := 0
					if query == "Enabled" {
						count = 1
					}
					if !strings.Contains(separator.String(), fmt.Sprintf(settingsText("Matches", "Matches: %d"), count)) {
						t.Fatalf("missing match count: %s", separator.String())
					}
				} else if strings.ContainsAny(separator.String(), "0123456789") {
					t.Fatal("idle separator contains match count")
				}
				if c.cancel.X2 != c.X2-2 || c.apply.X1 >= c.ok.X1 || c.ok.X1 >= c.cancel.X1 {
					t.Fatal("action alignment/order")
				}
			}
			c.search.SetText("")
			c.search.OnTextChange("")
			checkbox := c.page.rows[1].control
			for _, active := range []bool{false, true} {
				if active {
					c.SetFocusedItem(c.page)
					c.page.SetFocusedItem(checkbox)
				} else {
					c.SetFocusedItem(c.sidebar)
				}
				c.Show(scr)
				x, y, _, _ := checkbox.GetPosition()
				got := scr.GetCell(x+4, y).Attributes
				want := vtui.SetRGBBack(vtui.Palette[vtui.ColDialogText], uint32(0x303030+iteration))
				if active {
					want = vtui.Palette[vtui.ColDialogSelectedButton]
				}
				if got != want {
					t.Fatalf("content/focus palette %x want %x", got, want)
				}
				x, y, _, _ = c.page.rows[2].control.GetPosition()
				if scr.GetCell(x, y).Attributes != vtui.Palette[vtui.ColDialogEdit] {
					t.Fatal("input surface overwritten")
				}
				border := scr.GetCell(c.page.X1, c.Y1+3).Attributes
				if border != vtui.SetRGBBack(vtui.Palette[vtui.ColDialogBox], uint32(0x303030+iteration)) {
					t.Fatal("content border background")
				}
			}
			c.search.OnTextChange("Enabled")
			c.SetFocusedItem(c.search)
			for _, want := range []vtui.UIElement{c.previous, c.next, c.sidebar, c.page, c.apply, c.ok, c.cancel, c.search} {
				c.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_TAB})
				if c.GetFocusedItem() != want {
					t.Fatalf("search tab order got %T want %T", c.GetFocusedItem(), want)
				}
			}
			c.SetFocusedItem(c.next)
			c.search.OnTextChange("")
			if c.GetFocusedItem() != c.search {
				t.Fatal("focus remained on hidden arrow")
			}
		}
	}
}
