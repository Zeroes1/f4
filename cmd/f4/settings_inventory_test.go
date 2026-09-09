package main

import (
	"encoding/json"
	"github.com/unxed/f4/sdk/f4settings"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Opt-in exporter keeps descriptions in the application's localization resources.
func TestSettingsExportDescriptions(t *testing.T) {
	dir := os.Getenv("F4_SETTINGS_EXPORT")
	if dir == "" {
		t.Skip("set F4_SETTINGS_EXPORT to regenerate resources")
	}
	catalogs := []f4settings.Catalog{(coreSettingsProvider{}).Catalog(), newCoreRecordSettingsProvider().Catalog(), (hotkeySettingsProvider{}).Catalog(), (pluginSettingsProvider{}).Catalog(), (aiSettingsProvider{}).Catalog(), (settingsOperationsProvider{}).Catalog(), (&catalogSettingsProvider{}).Catalog()}
	entries := map[string]string{"SettingsCenter.Title": "Settings", "SettingsCenter.Search": "Search:", "SettingsCenter.Apply": "&Apply", "SettingsCenter.Previous": "Previous match", "SettingsCenter.Next": "Next match", "SettingsCenter.Enabled": "Enabled", "SettingsCenter.Select": "Select a setting to read what it does.", "Menu.Terminal.Options": "&Options"}
	for _, cat := range settingsCategories {
		entries["SettingsCenter.Category."+cat.ID] = cat.Label.English
	}
	var metadata []map[string]any
	add := func(f f4settings.Field) {
		if f.Label.Key != "" {
			entries[f.Label.Key] = f.Label.English
		}
		if f.Description.Key != "" {
			entries[f.Description.Key] = f.Description.English
		}
		entries["SettingsCenter."+settingsGroupKey(f.Group)] = f.Group
		metadata = append(metadata, map[string]any{"id": f.ID, "category": f.Category, "group": f.Group, "label": f.Label.English, "description": f.Description.English, "kind": f.Kind, "timing": f.Timing, "unavailable": f.Unavailable})
	}
	for _, c := range catalogs {
		for _, f := range c.Fields {
			add(f)
		}
		for _, col := range c.Collections {
			for _, f := range col.Fields {
				f.Category = col.Category
				f.Group = col.Group
				add(f)
			}
		}
	}
	var keys []string
	for k := range entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		if entries[key] != "" {
			b.WriteString(key + "=" + strings.ReplaceAll(entries[key], "\n", "\\n") + "\n")
		}
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings-center.lng"), []byte(b.String()), 0600); err != nil {
		t.Fatal(err)
	}
	data, _ := json.MarshalIndent(metadata, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "settings-center-fields.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
