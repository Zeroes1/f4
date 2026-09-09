package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

type coreSettingsProvider struct {
	// Shared by the draft and renderer for one opening; refreshed next time.
	catalog *f4settings.Catalog
}

func settingsConfigField(v reflect.Value, path string) reflect.Value {
	for _, part := range strings.Split(path, ".") {
		if v.Kind() == reflect.Array {
			i, _ := strconv.Atoi(part)
			v = v.Index(i)
		} else {
			v = v.FieldByName(part)
		}
	}
	return v
}
func coreSettingValue(cfg F4Config, id string) string {
	if strings.HasPrefix(id, "DriveMenuOptions.") {
		bit, _ := strconv.Atoi(strings.TrimPrefix(id, "DriveMenuOptions."))
		return strconv.FormatBool(cfg.DriveMenuOptions&(1<<bit) != 0)
	}
	if id == "ConsoleMode" {
		return consoleViewStyleOf(ShellModeConfig{cfg.ConsoleMode, cfg.ConsoleOverlayUI})
	}
	v := settingsConfigField(reflect.ValueOf(cfg), id)
	switch v.Kind() {
	case reflect.Bool:
		return strconv.FormatBool(v.Bool())
	case reflect.Int:
		return strconv.FormatInt(v.Int(), 10)
	default:
		return v.String()
	}
}
func setCoreSetting(cfg *F4Config, id, value string) error {
	if strings.HasPrefix(id, "DriveMenuOptions.") {
		bit, _ := strconv.Atoi(strings.TrimPrefix(id, "DriveMenuOptions."))
		if value == "true" {
			cfg.DriveMenuOptions |= 1 << bit
		} else {
			cfg.DriveMenuOptions &^= 1 << bit
		}
		return nil
	}
	v := settingsConfigField(reflect.ValueOf(cfg).Elem(), id)
	switch v.Kind() {
	case reflect.Bool:
		n, err := strconv.ParseBool(value)
		if err != nil {
			return err
		}
		v.SetBool(n)
	case reflect.Int:
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return err
		}
		v.SetInt(n)
	default:
		v.SetString(value)
	}
	return nil
}

func (p coreSettingsProvider) Catalog() f4settings.Catalog {
	if p.catalog != nil {
		return *p.catalog
	}
	fields := coreSettingsFields()
	for _, area := range []string{"Panel", "Editor", "Viewer", "Menu", "Table"} {
		for _, direction := range []string{"Up", "Down"} {
			id := "Wheel" + area + direction
			fields = append(fields, f4settings.Field{ID: id, Category: "keyboard", Group: "Mouse wheel", Label: f4settings.Text{English: area + " wheel " + strings.ToLower(direction)}, Description: f4settings.Text{English: "Number of " + strings.ToLower(area) + " rows per " + strings.ToLower(direction) + "ward wheel notch. Zero follows the system setting."}, Kind: f4settings.Integer, Timing: "live"})
		}
	}
	for i, label := range []string{"Command history timestamps", "Folder history timestamps", "Viewer/editor history timestamps"} {
		fields = append(fields, f4settings.Field{ID: fmt.Sprintf("HistoryShowTimes.%d", i), Category: "history", Group: "Presentation", Label: f4settings.Text{English: label}, Description: f4settings.Text{English: "Choose the timestamp presentation independently for this history."}, Kind: f4settings.ChoiceKind, Choices: settingsChoices("0:Date and time;1:Date;2:None"), Timing: "new history dialogs"})
	}
	driveDescriptions := []string{"Add the drive-kind column.", "Display the filesystem volume label.", "This stored compatibility flag currently has no consumer.", "Display filesystem type.", "Display total and free capacity columns.", "Use fractional human-readable capacity instead of integer binary units.", "Display the mount target when different from the drive path.", "Include registered plugin drives and providers.", "Order plugin drive rows by their assigned shortcut.", "Include removable drives.", "Include optical drives.", "Include mapped and network drives.", "This stored compatibility flag has no consumer. SUBST detection runs independently.", "Include saved bookmarks and links."}
	for i, spec := range driveMenuOptionSpecs {
		f := f4settings.Field{ID: fmt.Sprintf("DriveMenuOptions.%d", i), Category: "drives", Group: "Drive entries", Label: f4settings.Text{English: strings.TrimPrefix(spec.label, "Drive."), Key: spec.label}, Description: f4settings.Text{English: driveDescriptions[i]}, Kind: f4settings.Boolean, Timing: "next drive menu"}
		if i == 2 || i == 12 {
			f.Unavailable = driveDescriptions[i]
		}
		fields = append(fields, f)
	}
	for i := range fields {
		f := &fields[i]
		switch f.ID {
		case "GuiFont":
			f.Kind = f4settings.ChoiceKind
			f.AllowCustom = true
			installed := discoverInstalledGuiFonts(AppConfig.Language)
			values := guiFontChoicesFromInstalled(AppConfig.GuiFont, installed)
			labels := guiFontDisplayValuesFromInstalled(values, installed)
			for i, value := range values {
				f.Choices = append(f.Choices, f4settings.Choice{Value: value, Label: f4settings.Text{English: labels[i], Literal: true}})
			}
		case "ColorStyle":
			f.Kind = f4settings.ChoiceKind
			for _, style := range AvailableColorStyles() {
				f.Choices = append(f.Choices, f4settings.Choice{Value: style.Name, Label: f4settings.Text{English: style.Name, Literal: true}})
			}
		case "Language":
			f.Kind = f4settings.ChoiceKind
			for _, l := range listAvailableUILanguages() {
				f.Choices = append(f.Choices, f4settings.Choice{Value: l.code, Label: f4settings.Text{English: l.name, Literal: true}})
			}
		case "HelpLanguage":
			f.Kind = f4settings.ChoiceKind
			for _, l := range listAvailableHelpLanguages() {
				f.Choices = append(f.Choices, f4settings.Choice{Value: l.code, Label: f4settings.Text{English: l.name, Literal: true}})
			}
		case "GuiBackend":
			f.Kind = f4settings.ChoiceKind
			f.Choices = settingsChoices(":Automatic")
			for _, b := range startupGuiBackends {
				f.Choices = append(f.Choices, f4settings.Choice{Value: b, Label: f4settings.Text{English: b, Literal: true}})
			}
		case "EditorDefaultCodePage", "ViewerDefaultCodePage":
			f.Kind = f4settings.ChoiceKind
			for _, cp := range vfs.AvailableCodepages {
				f.Choices = append(f.Choices, f4settings.Choice{Value: strconv.Itoa(cp.ID), Label: f4settings.Text{English: vfs.CodepageMenuLabel(cp), Literal: true}})
			}
		case "EditorColorerScheme":
			f.Kind = f4settings.ChoiceKind
			f.Choices = settingsChoices(":Built-in default")
		}
		if f.Label.Key == "" {
			f.Label.Key = "SettingsCenter." + f.ID + ".Label"
		}
		if f.Description.Key == "" {
			f.Description.Key = "SettingsCenter." + f.ID + ".Description"
		}
		f.Aliases = append(f.Aliases, "settings", f.ID)
		// Preserve discoverability for users who remember the former dialog.
		f.Aliases = append(f.Aliases, map[string]string{
			"appearance": "Appearance settings Language Help language",
			"workspaces": "Appearance settings Panel settings Auto save Details",
			"panels":     "Panel settings Additional settings Details",
			"drives":     "Drive menu options F9",
			"navigation": "Panel settings Additional settings Path hints",
			"operations": "Confirmations Compare folders Panel settings",
			"editor":     "Editor settings Viewer settings Colorer settings",
			"syntax":     "Colorer settings Editor settings",
			"keyboard":   "Mouse wheel Hotkey configuration Exact hit",
			"terminal":   "Panel settings Additional settings",
			"updates":    "Auto update settings",
		}[f.Category])
	}
	return f4settings.Catalog{ID: "core", Categories: settingsCategories, Fields: fields, Background: true}
}
func settingsChoices(s string) []f4settings.Choice {
	var result []f4settings.Choice
	for _, part := range strings.Split(s, ";") {
		p := strings.SplitN(part, ":", 2)
		result = append(result, f4settings.Choice{Value: p[0], Label: f4settings.Text{English: p[1]}})
	}
	return result
}

var writeSettingsCandidate = func(before, after F4Config) error {
	path := getUserConfigIniPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return writeFileAtomically(path, serializeSettingsConfig(after), 0600)
	}
	if err != nil {
		return err
	}
	oldIni := ParseIni(strings.NewReader(string(serializeSettingsConfig(before))))
	newIni := ParseIni(strings.NewReader(string(serializeSettingsConfig(after))))
	for section, values := range newIni.data {
		changes := map[string]string{}
		for key, value := range values {
			if oldIni.GetString(section, key, "") != value {
				changes[key] = value
			}
		}
		if len(changes) > 0 {
			data = updateIniValues(data, section, changes)
		}
	}
	return writeFileAtomically(path, data, 0600)
}

func (p coreSettingsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	catalog := p.Catalog()
	values := map[string]string{}
	for _, f := range catalog.Fields {
		values[f.ID] = coreSettingValue(AppConfig, f.ID)
	}
	d := f4settings.NewDraft(values, nil)
	palette := append([]uint64(nil), vtui.Palette...)
	previewed := false
	d.PreviewFunc = func(d *f4settings.Draft) error {
		if d.Values["ColorStyle"] == d.Baseline["ColorStyle"] && !previewed {
			return nil
		}
		if err := ApplyColorStyle(d.Values["ColorStyle"]); err != nil {
			return err
		}
		previewed = true
		return nil
	}
	d.CloseFunc = func() {
		if previewed {
			_ = ApplyColorStyle(AppConfig.ColorStyle)
			copy(vtui.Palette, palette)
		}
	}
	d.ValidateFunc = func(d *f4settings.Draft) map[string]error {
		errors := map[string]error{}
		for _, f := range catalog.Fields {
			if !d.Dirty(f.ID) {
				continue
			}
			value := d.Values[f.ID]
			if f.Unavailable != "" {
				errors[f.ID] = settingsError("%s", f.Unavailable)
				continue
			}
			if strings.ContainsAny(value, "\r\n") {
				errors[f.ID] = settingsError("value must be on one line")
			}
			if f.Kind == f4settings.Boolean {
				if _, err := strconv.ParseBool(value); err != nil {
					errors[f.ID] = settingsError("choose enabled or disabled")
				}
			}
			if f.Kind == f4settings.ChoiceKind && !f.AllowCustom && f.ID != "EditorColorerScheme" {
				known := false
				for _, choice := range f.Choices {
					if choice.Value == value {
						known = true
						break
					}
				}
				if !known && !(f.ID == "GuiBackend" && (value == "qt" || strings.HasPrefix(value, "ext:"))) {
					errors[f.ID] = settingsError("choose an available value")
				}
			}
			if f.Kind == f4settings.Integer {
				n, err := strconv.Atoi(value)
				min := 0
				if f.ID == "EditorTabSize" || f.ID == "PathHintTimeout" || f.ID == "PathHintMaxVisible" || f.ID == "Compare.MaxDepth" || f.ID == "GuiFontSize" {
					min = 1
				}
				if f.ID == "HistoryDirsPrefixLen" {
					min = 4
				}
				if err != nil || n < min {
					errors[f.ID] = settingsError("enter an integer of at least %d", min)
				}
				if f.ID == "Compare.MaxDepth" && n > 99 {
					errors[f.ID] = settingsError("maximum depth is 99")
				}
			}
			if current := coreSettingValue(AppConfig, f.ID); current != d.Baseline[f.ID] && current != value {
				errors[f.ID] = settingsError("this setting changed outside Settings Center; reopen to load its current value")
			}
		}
		return errors
	}
	d.CommitFunc = func(ctx context.Context, d *f4settings.Draft) f4settings.Result {
		if err := ctx.Err(); err != nil {
			return f4settings.Result{Errors: map[string]error{"core": err}}
		}
		onUI := func(run func()) {
			if task, ok := ctx.(*vtui.TaskContext); ok {
				done := make(chan struct{})
				task.RunOnUI(func() { defer close(done); run() })
				<-done
			} else {
				run()
			}
		}
		var before, candidate F4Config
		var failures map[string]error
		changed := d.Changed()
		onUI(func() {
			failures = d.Validate()
			before = AppConfig
			candidate = before
			for _, id := range changed {
				if err := setCoreSetting(&candidate, id, d.Values[id]); err != nil {
					failures[id] = err
				}
			}
			candidate.AutoSaveSettings = candidate.AutoSaveDialogSettings || candidate.AutoSavePanelSettings || candidate.AutoSaveCurrentPanel || candidate.AutoSaveGUIWindow
			if d.Dirty("ExternalEditorConsole") {
				candidate.ExternalEditorCommand = candidate.ExternalEditorConsole
			}
		})
		if len(failures) > 0 {
			return f4settings.Result{Errors: failures}
		}
		if err := ctx.Err(); err != nil {
			return f4settings.Result{Errors: map[string]error{"core": err}}
		}
		if err := writeSettingsCandidate(before, candidate); err != nil {
			return f4settings.Result{Errors: map[string]error{"core": err}}
		}
		onUI(func() {
			previous := AppConfig
			for _, id := range changed {
				_ = setCoreSetting(&AppConfig, id, d.Values[id])
			}
			syncAutoSaveMaster()
			if d.Dirty("ExternalEditorConsole") {
				AppConfig.ExternalEditorCommand = AppConfig.ExternalEditorConsole
			}
			applySettingsRuntime(previous, changed)
			palette = append(palette[:0], vtui.Palette...)
			previewed = false
		})
		return f4settings.Result{Applied: changed}
	}

	return d, nil
}

func applySettingsRuntime(before F4Config, changed []string) {
	ApplyProxySettings()
	applyWheelSettings()
	applyPathHintSettings()
	vtui.ManageCursorStyle = !AppConfig.KeepTerminalCursor
	for _, id := range changed {
		if id == "ColorStyle" || id == "EnforceColorCorrection" {
			_ = ApplyColorStyle(AppConfig.ColorStyle)
		}
		if strings.HasPrefix(id, "EditorColorer") {
			ResetColorerSessions()
			ResetColorerRegions()
			ResetColorerScheme()
		}
	}
	if before.Language != AppConfig.Language || before.HelpLanguage != AppConfig.HelpLanguage || before.UseLocalLanguageFiles != AppConfig.UseLocalLanguageFiles {
		InitLang()
		InitHelpSystem()
	}
	if fm := vtui.FrameManager; fm != nil {
		mode := vtui.WorkspaceCtrlTabDirect
		if AppConfig.CtrlTabShowsMenu {
			mode = vtui.WorkspaceCtrlTabMenu
		}
		fm.ConfigureWorkspaceTabs(vtui.WorkspaceTabMode(AppConfig.WorkspaceTabMode), mode)
		fm.ConfigureWorkspaceTabOverlay(AppConfig.WorkspaceTabsOverlay)
		fm.ConfigureWorkspaceAltNumberSwitch(AppConfig.AltNumberSwitchesTabs)
		if before.WorkspaceTabNumbering != AppConfig.WorkspaceTabNumbering && AppConfig.WorkspaceTabNumbering == WorkspaceTabNumbersOrder {
			renumberWorkspaceScreens()
		}
		for _, screen := range fm.Screens {
			for _, frame := range screen.Frames {
				if pf, ok := frame.(*PanelsFrame); ok && !pf.closed {
					pf.cmdLine.Edit.PathHintsEnabled = AppConfig.CommandLineAutoComplete
					if before.NavigationMode != AppConfig.NavigationMode {
						pf.applyNavigationMode()
					}
					pf.ResizeConsole(pf.lastW, pf.lastH)
					pf.RefreshAll()
				}
			}
		}
		fm.HardRefresh()
	}
}

// Enumerating names must not instantiate the highlighting WASM engine just to
// open Settings. HRD metadata is declared in the Colorer catalog itself.
func settingsColorerSchemes() []ColorerScheme {
	return settingsColorerSchemesAt(ColorerConfigsDir())
}
func settingsColorerSchemesAt(directory string) []ColorerScheme {
	file, err := os.Open(filepath.Join(directory, "base", "catalog.xml"))
	if err != nil {
		return nil
	}
	defer file.Close()
	decoder := xml.NewDecoder(io.LimitReader(file, 4<<20))
	var schemes []ColorerScheme
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := token.(xml.StartElement)
		if !ok || start.Name.Local != "hrd" {
			continue
		}
		attrs := map[string]string{}
		for _, a := range start.Attr {
			attrs[a.Name.Local] = a.Value
		}
		if attrs["class"] == "rgb" && attrs["name"] != "" {
			schemes = append(schemes, ColorerScheme{Name: attrs["name"], Description: attrs["description"]})
		}
	}
	return schemes
}
