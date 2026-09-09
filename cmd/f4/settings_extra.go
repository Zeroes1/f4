package main

import (
	"context"
	"fmt"
	"github.com/unxed/f4/sdk/f4settings"
	"os"
	"strings"
)

type aiSettingsProvider struct{}

func (aiSettingsProvider) Catalog() f4settings.Catalog {
	source := "vtvibe.ini"
	for _, name := range []string{"GEMINI_API_KEY", "GOOGLE_API_KEY", "OPENAI_API_KEY"} {
		if strings.TrimSpace(os.Getenv(name)) != "" {
			source = name
			break
		}
	}
	key := f4settings.Scalar("ai.key", "ai", "Credentials", "Saved API key", "Used only when GEMINI_API_KEY, GOOGLE_API_KEY and OPENAI_API_KEY are all empty, in that precedence order. Effective source: %s (if a key is available). The key is stored in the local vtvibe.ini file.", f4settings.Secret)
	key.Description.Args = []any{source}
	return f4settings.Catalog{ID: "ai", Categories: settingsCategories, Background: true, Fields: []f4settings.Field{
		key,
		f4settings.Scalar("ai.model", "ai", "Model", "Model", "Model identifier sent with subsequent AI requests. The configured API endpoint must support it.", f4settings.String),
	}}
}
func (p aiSettingsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	path := vtvibeIniPath()
	ini := LoadIni(path)
	d := f4settings.NewDraft(map[string]string{"ai.key": ini.GetString("general", "key", ""), "ai.model": ini.GetString("general", "model", vtvibeDefaultModel)}, nil)
	d.ValidateFunc = func(d *f4settings.Draft) map[string]error {
		errs := map[string]error{}
		for _, id := range d.Changed() {
			if err := settingsSingleLine(d.Values[id]); err != nil {
				errs[id] = err
			}
		}
		return errs
	}
	d.CommitFunc = func(ctx context.Context, d *f4settings.Draft) f4settings.Result {
		errs := d.Validate()
		if len(errs) > 0 {
			return f4settings.Result{Errors: errs}
		}
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return f4settings.Result{Errors: map[string]error{"ai": err}}
		}
		current := LoadIni(path)
		patch := map[string]string{}
		for _, id := range d.Changed() {
			key := strings.TrimPrefix(id, "ai.")
			fallback := ""
			if key == "model" {
				fallback = vtvibeDefaultModel
			}
			v := current.GetString("general", key, fallback)
			if v != d.Baseline[id] && v != d.Values[id] {
				return f4settings.Result{Errors: map[string]error{id: settingsError("value changed outside Settings Center")}}
			}
			patch[key] = d.Values[id]
		}
		if err := ctx.Err(); err != nil {
			return f4settings.Result{Errors: map[string]error{"ai": err}}
		}
		if err := writeFileAtomically(path, updateIniValues(data, "general", patch), 0600); err != nil {
			return f4settings.Result{Errors: map[string]error{"ai": err}}
		}
		return f4settings.Result{Applied: d.Changed()}
	}
	return d, nil
}

type hotkeySettingsProvider struct{}

func (hotkeySettingsProvider) Catalog() f4settings.Catalog {
	fields := []f4settings.Field{
		recordField("binding.Action", "Action", "Stable action identifier to invoke. Existing action IDs and plugin actions remain supported.", f4settings.ChoiceKind),
		recordField("binding.Key", "Chord", "Far-style chord spelling, such as CtrlShiftF. An empty chord leaves the action unassigned.", f4settings.Chord),
		recordField("binding.Area", "Area", "Input context in which this binding is considered. Common is the fallback for other areas.", f4settings.ChoiceKind),
		recordField("binding.Condition", "Condition", "Optional registered condition checked when dispatching the action. Empty means unconditional.", f4settings.String),
	}
	fields[0].ChoicePresentation = "dropdown"
	fields[2].ChoicePresentation = "dropdown"
	for _, a := range GetActions() {
		fields[0].Choices = append(fields[0].Choices, f4settings.Choice{Value: a.Name, Label: f4settings.Text{English: plainLabel(a.DisplayLabel()) + " (" + a.Name + ")", Literal: true}})
	}
	fields[2].Choices = f4settings.Choices("Shell:Panels", "Terminal:Terminal", "Editor:Editor", "Viewer:Viewer", "Dialog:Dialog", "Menu:Menu", "Disks:Drive chooser", "Common:Common")
	col := recordCollection("bindings", "keyboard", "Key bindings", "Bindings are edited inline. Native frame-owned shortcuts remain outside the editable binding table.", "binding.Action", fields)
	col.Ordered = false
	return f4settings.Catalog{ID: "hotkeys", Categories: settingsCategories, Collections: []f4settings.Collection{col}}
}
func (hotkeySettingsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	hm := GlobalHotkeysMgr
	var rows []f4settings.Record
	if hm != nil {
		for i, r := range buildHotkeyRows(hm) {
			if r.Editable {
				rows = append(rows, f4settings.Record{ID: fmt.Sprintf("binding:%d", i), Values: map[string]string{"binding.Action": r.Action, "binding.Key": r.RawKey, "binding.Area": r.Area, "binding.Condition": r.Condition}})
			}
		}
	}
	d := f4settings.NewDraft(nil, map[string][]f4settings.Record{"bindings": rows})
	build := func(d *f4settings.Draft) (*HotkeyManager, error) {
		if hm == nil {
			return nil, settingsError("keyboard manager is unavailable")
		}
		next := hm.CloneForEdit()
		old := map[string]string{}
		wanted := map[string]string{}
		flatten := func(rows []f4settings.Record, dest map[string]string) error {
			for _, r := range rows {
				v := r.Values
				key := v["binding.Key"]
				if key == "" {
					continue
				}
				area := v["binding.Area"]
				action := v["binding.Action"]
				cond := v["binding.Condition"]
				if strings.ContainsAny(key+area+action+cond, "\r\n=") {
					return settingsError("binding contains an invalid separator")
				}
				id := area + "\x00" + key
				if _, ok := dest[id]; ok {
					return settingsError("duplicate chord %s in %s", key, area)
				}
				if cond != "" {
					action += ":" + cond
				}
				dest[id] = action
			}
			return nil
		}
		if err := flatten(d.BaselineRecords["bindings"], old); err != nil {
			return nil, err
		}
		if err := flatten(d.Records["bindings"], wanted); err != nil {
			return nil, err
		}
		keys := map[string]bool{}
		for k := range old {
			keys[k] = true
		}
		for k := range wanted {
			keys[k] = true
		}
		for id := range keys {
			if old[id] == wanted[id] {
				continue
			}
			area, key, _ := strings.Cut(id, "\x00")
			current := next.Bindings[area][key]
			if current != old[id] && current != wanted[id] {
				return nil, settingsError("binding %s changed outside Settings Center", key)
			}
			if next.Bindings[area] == nil {
				next.Bindings[area] = map[string]string{}
			}
			if wanted[id] == "" {
				delete(next.Bindings[area], key)
			} else {
				next.Bindings[area][key] = wanted[id]
			}
		}
		return next, nil
	}
	d.ValidateFunc = func(d *f4settings.Draft) map[string]error {
		if !d.Dirty("bindings") {
			return nil
		}
		_, err := build(d)
		if err != nil {
			return map[string]error{"bindings": err}
		}
		return nil
	}
	d.CommitFunc = func(ctx context.Context, d *f4settings.Draft) f4settings.Result {
		next, err := build(d)
		if err == nil {
			err = ctx.Err()
		}
		if err == nil {
			err = next.SaveError()
		}
		if err != nil {
			return f4settings.Result{Errors: map[string]error{"bindings": err}}
		}
		hm.ReplaceBindingsFrom(next)
		return f4settings.Result{Applied: []string{"bindings"}}
	}
	return d, nil
}
