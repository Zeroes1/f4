package main

// The old action names remain bindable and become category deep links.
var settingsDeepLinks = map[string]string{
	"settings.language": "appearance", "settings.helplanguage": "appearance", "settings.panel": "panels", "settings.editor": "editor", "settings.viewer": "editor", "settings.colorer": "syntax", "settings.appearance": "appearance", "settings.startup": "startup", "settings.portable": "startup", "settings.confirmations": "operations", "settings.mousewheel": "keyboard", "settings.pathhints": "navigation", "settings.hotkeys": "keyboard", "settings.autoupdate": "updates", "settings.proxy": "network", "settings.pluginconfiguration": "plugins", "settings.plugins": "plugins", "settings.mackeyboard": "keyboard", "editor.settings": "editor", "viewer.settings": "editor", "panel.fileassociations": "associations", "app.plugring": "plugins", "ai.setup": "ai",
}

func init() {
	RegisterAction(Action{Name: "Settings.Open", Area: "Common", Label: "Settings", LabelKey: "SettingsCenter.Title", Description: "Open the searchable Settings Center", MenuPath: "Options", Handler: func() bool { return openSettingsCenter("") }})
	for _, cat := range settingsCategories {
		id := cat.ID
		RegisterAction(Action{Name: "Settings.Category." + id, Area: "Common", Label: cat.Label.English, Description: "Open Settings Center at " + cat.Label.English, Handler: func() bool { return openSettingsCenter(id) }})
	}
}
