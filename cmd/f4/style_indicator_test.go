package main

import (
	"github.com/unxed/vtui"
	"path/filepath"
	"testing"
)

func TestIndicatorBackgroundThemeCompatibility(t *testing.T) {
	saved := append([]uint64(nil), vtui.Palette...)
	defer func() { vtui.Palette = saved }()
	oldConfig := AppConfig
	defer func() { AppConfig = oldConfig }()
	oldPath := userColorOverridesPath
	userColorOverridesPath = func() string { return filepath.Join(t.TempDir(), "none.ini") }
	defer func() { userColorOverridesPath = oldPath }()
	for _, name := range []string{"Modern", "Classic", "Default Dark", "Radiola", "Modern"} {
		if err := ApplyColorStyle(name); err != nil {
			t.Fatal(err)
		}
		attr := vtui.Palette[vtui.ColDialogIndicatorBackground]
		if name == "Modern" {
			_, want := GetColorRGBBoth(vtui.Palette[vtui.ColDialogEdit])
			_, got := GetColorRGBBoth(attr)
			if got != want {
				t.Fatal("Modern indicator must match input background")
			}
		} else if attr != 0 {
			t.Fatalf("%s unexpectedly overrides indicator background", name)
		}
	}
	for _, name := range []string{"Modern", "Classic"} {
		if err := ApplyColorStyle(name); err != nil {
			t.Fatal(err)
		}
		want := vtui.Palette[vtui.ColDialogIndicatorBackground]
		path := filepath.Join(t.TempDir(), "colors.ini")
		if err := ExportColors(path); err != nil {
			t.Fatal(err)
		}
		ini := LoadIni(path)
		vtui.SetDefaultPalette()
		SetDefaultF4Palette()
		InitColors(ini)
		got := vtui.Palette[vtui.ColDialogIndicatorBackground]
		if want == 0 && got != 0 || want != 0 && vtui.GetRGBBack(want) != vtui.GetRGBBack(got) {
			t.Fatal("indicator export did not round-trip")
		}
	}
}
