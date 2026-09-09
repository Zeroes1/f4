package main

import (
	"context"
	"fmt"
	"github.com/unxed/f4/internal/netproxy"
	"github.com/unxed/f4/sdk/f4settings"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

type settingsOperationsProvider struct{}

func saveAppliedConfiguration() error {
	path := getUserConfigIniPath()
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	ini := ParseIni(strings.NewReader(string(serializeSettingsConfig(AppConfig))))
	for section, values := range ini.data {
		data = updateIniValues(data, section, values)
	}
	return writeFileAtomically(path, data, 0600)
}
func (settingsOperationsProvider) Catalog() f4settings.Catalog {
	cat := f4settings.Catalog{ID: "operations", Categories: settingsCategories}
	add := func(id, category, group, label, desc string, background bool, run func(context.Context) error) {
		cat.Commands = append(cat.Commands, f4settings.Command{ID: id, Category: category, Group: group, Label: f4settings.Text{English: label}, Description: f4settings.Text{English: desc}, Background: background, Run: run})
	}
	add("save.preferences", "workspaces", "Manual saving", "Save applied preferences", "Write the applied configuration even when automatic saving is disabled.", false, func(context.Context) error { return saveAppliedConfiguration() })
	add("save.session", "workspaces", "Manual saving", "Save session", "Save workspaces, panel state and remembered operation inputs using the applied path-restoration policy.", false, func(context.Context) error { return saveSessionFileError(getSessionIniPath(), true, true) })
	add("save.geometry", "workspaces", "Manual saving", "Save window geometry", "Capture and save the current graphical window dimensions and position independently of other settings.", false, func(context.Context) error {
		captureCurrentWindowSize()
		captureCurrentWindowPosition()
		path := getUserConfigIniPath()
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return writeFileAtomically(path, updateIniValues(data, "Appearance", guiWindowValues()), 0600)
	})
	add("colors.export", "appearance", "Theme", "Export applied colors", "Write the complete applied palette to farcolors.ini in the current configuration directory.", false, func(context.Context) error { return ExportColors(userColorOverridesPath()) })
	add("syntax.reload", "syntax", "Colorer", "Reload schemas", "Drop cached Colorer sessions, regions and scheme so subsequent highlighting loads applied configuration.", false, func(context.Context) error {
		ResetColorerSessions()
		ResetColorerRegions()
		ResetColorerScheme()
		return nil
	})
	add("syntax.download", "syntax", "Colorer", "Download schemas", "Download and validate the Colorer schema archive, then install it at the applied configuration directory.", true, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "GET", colorerDownloadURL, nil)
		if err != nil {
			return err
		}
		resp, err := netproxy.HTTPClient(0).Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return settingsError("schema download: HTTP %d", resp.StatusCode)
		}
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxColorerDownload+1))
		if err != nil {
			return err
		}
		if len(data) > maxColorerDownload {
			return settingsError("schema archive is too large")
		}
		return installColorerSchemas(data, ColorerConfigsDir(), ctx)
	})
	add("updates.check", "updates", "Updates", "Check now", "Check the applied release channel now. An available update is offered as a separate installation operation.", true, checkSettingsUpdates)
	for _, enable := range []bool{true, false} {
		for _, move := range []bool{false, true} {
			target := systemProfileDir()
			name := "user profile"
			if enable {
				target = portableProfileDir()
				name = "portable profile"
			}
			verb := "Copy"
			if move {
				verb = "Move"
			}
			id := fmt.Sprintf("profile.%t.%t", enable, move)
			add(id, "startup", "Profile transfer", verb+" to "+name, verb+" the applied profile from %s to %s and select it for the next launch. Restart is required. This operation is not undone by Cancel.", false, func(context.Context) error {
				dlg := vtui.ShowMessage(settingsPhrase("Profile transfer"), fmt.Sprintf(settingsPhrase(verb+" profile to:\n%s?\nRestart is required after completion."), target), []string{settingsPhrase(verb), Msg("vtui.Cancel")})
				dlg.OnResult = func(code int) {
					if code != 0 {
						return
					}
					vtui.RunAsync(func(task *vtui.TaskContext) {
						err := applyPortableMode(currentPortableIniPath(), enable, move)
						task.RunOnUI(func() {
							message := settingsPhrase("Profile transferred. Restart f4 to use it.")
							if err != nil {
								message = err.Error()
							}
							vtui.ShowMessage(settingsPhrase("Profile transfer"), message, []string{Msg("vtui.Ok")})
						})
					})
				}
				return nil
			})
			cat.Commands[len(cat.Commands)-1].Description.Args = []any{GetF4ConfigDir(), target}
		}
	}
	// Legacy external plugins keep explicit launchers, resolved against the live registry.
	for _, cmd := range pluginCommandsSnapshot(vfs.PluginCommandConfig, findPanelsFrameAnyScreen()) {
		if _, bundled := bundledSettingsCommands[strings.ToLower(cmd.ID)]; bundled {
			continue
		}
		id := cmd.ID
		add("legacy."+id, "plugins", "Legacy configuration", "Legacy configuration: "+plainLabel(pluginCommandDisplayLabel(cmd)), "This external plugin has not contributed settings metadata. Opens its own configuration interface using applied preferences.", false, func(context.Context) error {
			pf := findPanelsFrameAnyScreen()
			if pf == nil {
				return settingsError("this legacy plugin requires an open panels workspace")
			}
			if !executeRegisteredPluginCommand(vfs.PluginCommandConfig, id, pf) {
				return settingsError("plugin is no longer available")
			}
			return nil
		})
		cat.Commands[len(cat.Commands)-1].Label = f4settings.Text{English: "Legacy configuration: %s", Args: []any{plainLabel(pluginCommandDisplayLabel(cmd))}}
	}
	for _, item := range []struct{ id, label, value string }{{"profile.path", "Current configuration directory", GetF4ConfigDir()}, {"profile.ini", "Main settings file", getUserConfigIniPath()}, {"profile.session", "Session file", getSessionIniPath()}, {"profile.portable", "Portable profile directory", portableProfileDir()}, {"profile.system", "User profile directory", systemProfileDir()}} {
		f := f4settings.Scalar(item.id, "startup", "Configuration locations", item.label, "Resolved configuration location for the running process. Profile transfers take effect on restart.", f4settings.Path)
		f.Unavailable = "Informational location"
		f.Default = item.value
		cat.Fields = append(cat.Fields, f)
	}
	status := f4settings.Scalar("updates.status", "updates", "Updates", "Last startup/manual check", "Timestamp of the latest attempted update check. Frequency is evaluated on application startup, not by a continuously running timer.", f4settings.String)
	status.Default = "Never"
	if AppConfig.LastUpdateCheck != 0 {
		status.Default = time.Unix(AppConfig.LastUpdateCheck, 0).Format(time.RFC1123)
	}
	status.Unavailable = "Status"
	cat.Fields = append(cat.Fields, status)
	for i := range cat.Commands {
		cmd := &cat.Commands[i]
		switch {
		case strings.HasPrefix(cmd.ID, "profile.") || strings.HasPrefix(cmd.ID, "legacy."):
			cmd.Requires = []string{"*"}
		case cmd.ID == "colors.export":
			cmd.Requires = []string{"ColorStyle", "EnforceColorCorrection"}
		case strings.HasPrefix(cmd.ID, "syntax."):
			cmd.Requires = []string{"EditorColorerCatalog", "EditorColorerScheme", "ProxyMode", "ProxyHost", "ProxyPort", "ProxyUser", "ProxyPass"}
		case cmd.ID == "updates.check":
			cmd.Requires = []string{"UpdateChannel", "ProxyMode", "ProxyHost", "ProxyPort", "ProxyUser", "ProxyPass"}
		}
	}
	return cat
}
func (p settingsOperationsProvider) Begin(context.Context) (*f4settings.Draft, error) {
	values := map[string]string{}
	for _, f := range p.Catalog().Fields {
		values[f.ID] = f.Default
	}
	return f4settings.NewDraft(values, nil), nil
}

var bundledSettingsCommands = map[string]string{"visren.configure": "operations", "f4.envman.configure": "terminal", "f4.mediainfo.configure": "metadata"}

func settingsRunOnUI(ctx context.Context, run func()) {
	if task, ok := ctx.(*vtui.TaskContext); ok {
		done := make(chan struct{})
		task.RunOnUI(func() { defer close(done); run() })
		<-done
	} else {
		run()
	}
}
func checkSettingsUpdates(ctx context.Context) error {
	var before, after F4Config
	settingsRunOnUI(ctx, func() { before = AppConfig; after = before; after.LastUpdateCheck = time.Now().Unix() })
	if err := writeSettingsCandidate(before, after); err != nil {
		return err
	}
	settingsRunOnUI(ctx, func() { AppConfig.LastUpdateCheck = after.LastUpdateCheck })
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	candidate, err := fetchUpdateCandidate(bounded, after.UpdateChannel)
	if err != nil {
		return err
	}
	settingsRunOnUI(ctx, func() {
		if !candidate.needsUpdate {
			vtui.ShowMessage(settingsPhrase("Updates"), settingsPhrase("You are using the latest version."), []string{Msg("vtui.Ok")})
			return
		}
		pf := findPanelsFrameAnyScreen()
		if pf == nil {
			vtui.ShowMessage(settingsPhrase("Updates"), fmt.Sprintf(settingsPhrase("Update available: %s. Open a panels workspace to install it."), candidate.displayVersion), []string{Msg("vtui.Ok")})
			return
		}
		dlg := vtui.ShowMessage(settingsPhrase("Updates"), fmt.Sprintf(settingsPhrase("Download and install %s?"), candidate.displayVersion), []string{settingsPhrase("Install"), Msg("vtui.Cancel")})
		dlg.OnResult = func(code int) {
			if code == 0 {
				performUpdate(pf, candidate)
			}
		}
	})
	return nil
}
