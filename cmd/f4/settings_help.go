package main

import (
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
)

func installSettingsHelp(language string) {
	catalogs := []f4settings.Catalog{(coreSettingsProvider{}).Catalog(), newCoreRecordSettingsProvider().Catalog(), (aiSettingsProvider{}).Catalog(), (hotkeySettingsProvider{}).Catalog(), (settingsOperationsProvider{}).Catalog(), (pluginSettingsProvider{}).Catalog()}
	settingsProviders.RLock()
	providers := append([]f4settings.Provider(nil), settingsProviders.providers...)
	settingsProviders.RUnlock()
	for _, p := range providers {
		catalogs = append(catalogs, p.Catalog())
	}
	title := (f4settings.Text{English: "Settings", Key: "SettingsCenter.Title"}).Resolve(language, helpMsg)
	index := &vtui.HelpTopic{Name: "SettingsCenter", StickyRows: 1, Lines: []string{title, "", "Search dims unrelated options without hiding or disabling them.", "Apply saves changes. OK saves and closes. Cancel discards later edits.", "Select an option to read its explanation. Settings scroll independently.", ""}}
	for _, category := range settingsCategories {
		index.Lines = append(index.Lines, category.Label.Resolve(language, helpMsg))
		add := func(f f4settings.Field) {
			label := f.Label.Resolve(language, helpMsg)
			description := f.Description.Resolve(language, helpMsg)
			topic := &vtui.HelpTopic{Name: "Setting." + f.ID, StickyRows: 1, Lines: []string{label, ""}}
			topic.Lines = append(topic.Lines, vtui.WrapText(description, generatedHelpLineWidth)...)
			if f.Timing != "" {
				topic.Lines = append(topic.Lines, "", "Takes effect: "+f.Timing)
			}
			if f.Unavailable != "" {
				topic.Lines = append(topic.Lines, vtui.WrapText("Unavailable: "+f.Unavailable, generatedHelpLineWidth)...)
			}
			vtui.GlobalHelpEngine.AddTopic(topic)
			index.Lines = append(index.Lines, "~"+label+"~Setting."+f.ID+"@")
		}
		for _, catalog := range catalogs {
			for _, field := range catalog.Fields {
				if field.Category == category.ID {
					add(field)
				}
			}
			for _, collection := range catalog.Collections {
				if collection.Category == category.ID {
					for _, field := range collection.Fields {
						add(field)
					}
				}
			}
		}
		index.Lines = append(index.Lines, "")
	}
	vtui.GlobalHelpEngine.AddTopic(index)
}
