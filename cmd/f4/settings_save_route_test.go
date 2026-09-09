package main

import "testing"

func TestSettingsSaveActionIsCenterDeepLink(t *testing.T) {
	action, ok := GetAction("App.SaveSettings")
	if !ok || !action.HideFromMenu || settingsDeepLinks["app.savesettings"] != "workspaces" {
		t.Fatal("Save Settings must be a hidden Settings Center deep link")
	}
	if len(action.DefaultKeys) != 1 || action.DefaultKeys[0] != "ShiftF9:NoAltScreenApp" {
		t.Fatal("legacy shortcut lost")
	}
	found := map[string]bool{}
	for _, command := range (settingsOperationsProvider{}).Catalog().Commands {
		if command.Category == "workspaces" && command.Group == "Manual saving" {
			found[command.ID] = true
		}
	}
	for _, id := range []string{"save.preferences", "save.session", "save.geometry"} {
		if !found[id] {
			t.Fatalf("missing manual saving command %s", id)
		}
	}
}
