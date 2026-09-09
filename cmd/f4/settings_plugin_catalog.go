package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/vtui"
	"sort"
)

type catalogSettingsProvider struct {
	draft  *f4settings.Draft
	closed bool
}

func (p *catalogSettingsProvider) rows(items []PlugRingItem) []f4settings.Record {
	installed := GetInstalledPlugRingItems()
	var rows []f4settings.Record
	for _, item := range items {
		data, _ := json.Marshal(item)
		status := "Not installed"
		if found, ok := installed[item.ID]; ok {
			status = "Installed: " + found.Version
		}
		rows = append(rows, f4settings.Record{ID: "catalog:" + item.ID, Values: map[string]string{"catalog.Name": item.Name, "catalog.Description": item.Description, "catalog.Status": status, "catalog.Version": item.Version, "__item": string(data)}})
	}
	return rows
}
func (p *catalogSettingsProvider) replace(items []PlugRingItem) {
	if p.closed {
		return
	}
	next := f4settings.NewDraft(nil, map[string][]f4settings.Record{"plugins.catalog": p.rows(items)})
	// Existing table rows share these maps, so completed package operations
	// update their status even while the user stays on the same page.
	for _, old := range p.draft.Records["plugins.catalog"] {
		for _, row := range next.Records["plugins.catalog"] {
			if old.ID == row.ID {
				for key, value := range row.Values {
					old.Values[key] = value
				}
			}
		}
	}
	p.draft.Records = next.Records
	p.draft.BaselineRecords = next.BaselineRecords
}
func (p *catalogSettingsProvider) Catalog() f4settings.Catalog {
	fields := []f4settings.Field{}
	for _, pair := range []struct{ id, label, desc string }{{"catalog.Description", "Description", "Catalog description supplied by the publisher."}, {"catalog.Version", "Catalog version", "Version available in the refreshed catalog."}, {"catalog.Status", "Installation status", "Installed version from the local plugin manifest, or not installed."}} {
		f := recordField(pair.id, pair.label, pair.desc, f4settings.String)
		f.Unavailable = "Catalog information"
		fields = append(fields, f)
	}
	col := recordCollection("plugins.catalog", "plugins", "Plugin catalog", "Installed packages are listed initially. Refresh catalog fetches available packages. Install and Remove are explicit operations using applied configuration.", "catalog.Name", fields)
	col.Fixed = true
	col.Ordered = false
	for _, install := range []bool{true, false} {
		label := "Remove"
		if install {
			label = "Install / update"
		}
		col.Actions = append(col.Actions, f4settings.RecordCommand{ID: label, Label: f4settings.Text{English: label}, Description: f4settings.Text{English: "Explicit plugin package operation. Changes are not undone by Cancel."}, RequiresApplied: true, Run: func(ctx context.Context, r f4settings.Record) (map[string]string, error) {
			var item PlugRingItem
			if err := json.Unmarshal([]byte(r.Values["__item"]), &item); err != nil {
				return nil, err
			}
			task, ok := ctx.(*vtui.TaskContext)
			if !ok {
				return nil, fmt.Errorf("interactive operation requires a UI task")
			}
			task.RunOnUI(func() {
				pf := findPanelsFrameAnyScreen()
				if pf == nil {
					vtui.ShowMessage("Plugin package", "Open a panels workspace to install or remove packages.", []string{Msg("vtui.Ok")})
					return
				}
				refresh := func() {
					var items []PlugRingItem
					for _, r := range p.draft.Records["plugins.catalog"] {
						var entry PlugRingItem
						if json.Unmarshal([]byte(r.Values["__item"]), &entry) == nil {
							items = append(items, entry)
						}
					}
					p.replace(items)
				}
				if install {
					actionInstallPlugRingItem(pf, nil, item, refresh)
				} else {
					actionRemovePlugRingItem(pf, nil, item, refresh)
				}
			})
			return nil, nil
		}})
	}
	cmd := f4settings.Command{ID: "catalog.refresh", Category: "plugins", Group: "Plugin catalog", Label: f4settings.Text{English: "Refresh catalog"}, Description: f4settings.Text{English: "Fetch available plugin packages using the applied proxy configuration."}, Background: true, Run: func(ctx context.Context) error {
		items, err := FetchCatalog(ctx)
		if err != nil {
			return err
		}
		task, ok := ctx.(*vtui.TaskContext)
		if !ok {
			return fmt.Errorf("catalog refresh requires a UI task")
		}
		task.RunOnUI(func() { p.replace(items) })
		return nil
	}}
	return f4settings.Catalog{ID: "plugin-catalog", Categories: settingsCategories, Collections: []f4settings.Collection{col}, Commands: []f4settings.Command{cmd}}
}
func (p *catalogSettingsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	p.draft = f4settings.NewDraft(nil, nil)
	p.closed = false
	p.draft.CloseFunc = func() { p.closed = true }
	var items []PlugRingItem
	for _, item := range GetInstalledPlugRingItems() {
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	p.replace(items)
	return p.draft, nil
}
