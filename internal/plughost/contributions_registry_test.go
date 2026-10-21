package plughost

import (
	"context"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

// stubApplication is a plughost.Application that records what the host asked
// of the application and answers from fixed values.
type stubApplication struct {
	stale bool

	opened []string

	overwriteChoice   int
	overwriteRemember bool
	askedPath         string
	overwriteCtx      context.Context

	errorChoice int
	askedOp     string
	askedErr    error
	errorCtx    context.Context
}

func (a *stubApplication) Current() vfs.App { return nil }

func (a *stubApplication) OpenPanelProvider(app vfs.App, providerID string) {
	a.opened = append(a.opened, providerID)
}

func (a *stubApplication) IsStale(app vfs.App) bool { return a.stale }

func (a *stubApplication) SetupUI() {}

func (a *stubApplication) SetClipboard(text string) {}

func (a *stubApplication) RunSemanticAction(action map[string]any) bool { return false }

func (a *stubApplication) AskOverwrite(ctx context.Context, destPath string, src, dst vfs.VFSItem, anchor vtui.Frame) (int, bool) {
	a.overwriteCtx = ctx
	a.askedPath = destPath
	return a.overwriteChoice, a.overwriteRemember
}

func (a *stubApplication) AskError(ctx context.Context, op string, err error, anchor vtui.Frame) int {
	a.errorCtx = ctx
	a.askedOp = op
	a.askedErr = err
	return a.errorChoice
}

// useStubApplication installs app as the host application for one test.
func useStubApplication(t *testing.T, app Application) {
	t.Helper()
	old := App
	App = app
	t.Cleanup(func() { App = old })
}

func TestUnregisterFuncRunsTheTeardownOnce(t *testing.T) {
	calls := 0
	registration := NewUnregisterFunc(func() { calls++ })
	registration.Unregister()
	registration.Unregister()
	if calls != 1 {
		t.Fatalf("teardown ran %d times, want exactly once", calls)
	}

	var none *UnregisterFunc
	none.Unregister() // a plugin may hand back a nil registration

	NewUnregisterFunc(nil).Unregister() // and a registration without a teardown is harmless
}

func TestRegisterPluginCommandRejectsInvalidCommands(t *testing.T) {
	run := func(vfs.App) {}
	cases := []struct {
		name    string
		command vfs.PluginCommand
		want    string
	}{
		{"empty id", vfs.PluginCommand{ID: "  ", Label: "Label", Run: run}, "ID is empty"},
		{"blank label", vfs.PluginCommand{ID: "cov.invalid.label", Label: " \t", Run: run}, "empty label"},
		{"no handler", vfs.PluginCommand{ID: "cov.invalid.run", Label: "Label"}, "no handler"},
		{"bad location", vfs.PluginCommand{ID: "cov.invalid.location", Label: "Label", Run: run, Location: vfs.PluginCommandLocation(9)}, "invalid location"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			before := PluginCommandRegistryGeneration()
			registration, err := RegisterPluginCommand(c.command)
			if err == nil {
				registration.Unregister()
				t.Fatalf("RegisterPluginCommand accepted %+v", c.command)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to mention %q", err, c.want)
			}
			if registration != nil {
				t.Errorf("a rejected command returned a registration: %v", registration)
			}
			if PluginCommandRegistryGeneration() != before {
				t.Error("a rejected command changed the registry generation")
			}
		})
	}
}

func TestRegisterPluginCommandDuplicateAndGeneration(t *testing.T) {
	command := vfs.PluginCommand{ID: "cov.Generation", Label: "Generation", Run: func(vfs.App) {}}

	start := PluginCommandRegistryGeneration()
	registration, err := RegisterPluginCommand(command)
	if err != nil {
		t.Fatalf("RegisterPluginCommand: %v", err)
	}
	t.Cleanup(registration.Unregister)
	afterRegister := PluginCommandRegistryGeneration()
	if afterRegister <= start {
		t.Fatalf("generation after register = %d, want it above %d", afterRegister, start)
	}

	// IDs are compared case-insensitively, so this is the same command.
	clash := command
	clash.ID = " COV.generation "
	if dup, err := RegisterPluginCommand(clash); err == nil {
		dup.Unregister()
		t.Fatal("a command with the same ID in another case was accepted")
	} else if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("duplicate error = %q, want it to say already registered", err)
	}
	if got := PluginCommandRegistryGeneration(); got != afterRegister {
		t.Fatalf("a refused duplicate moved the generation from %d to %d", afterRegister, got)
	}

	registration.Unregister()
	afterUnregister := PluginCommandRegistryGeneration()
	if afterUnregister <= afterRegister {
		t.Fatalf("generation after unregister = %d, want it above %d", afterUnregister, afterRegister)
	}
	registration.Unregister()
	if got := PluginCommandRegistryGeneration(); got != afterUnregister {
		t.Fatalf("a second Unregister moved the generation from %d to %d", afterUnregister, got)
	}
	if _, ok := PluginCommandByID("cov.generation"); ok {
		t.Error("the command is still registered after Unregister")
	}

	// The ID is free again, and the new registration is not undone by the
	// stale handle of the old one.
	again, err := RegisterPluginCommand(command)
	if err != nil {
		t.Fatalf("re-registering a released ID: %v", err)
	}
	t.Cleanup(again.Unregister)
	registration.Unregister()
	if _, ok := PluginCommandByID("cov.generation"); !ok {
		t.Error("the stale registration handle removed its successor")
	}
}

func TestExecutePluginCommandHonoursVisibleEnabledAndStaleApp(t *testing.T) {
	stub := &stubApplication{}
	useStubApplication(t, stub)

	var runs int
	visible, enabled := true, true
	command := vfs.PluginCommand{
		ID:       "cov.execute",
		Label:    "Execute",
		Location: vfs.PluginCommandConfig,
		Run:      func(vfs.App) { runs++ },
		Visible:  func(vfs.App) bool { return visible },
		Enabled:  func(vfs.App) bool { return enabled },
	}
	registration, err := RegisterPluginCommand(command)
	if err != nil {
		t.Fatalf("RegisterPluginCommand: %v", err)
	}
	t.Cleanup(registration.Unregister)

	if !ExecutePluginCommand(vfs.PluginCommandConfig, " COV.Execute ", nil) || runs != 1 {
		t.Fatalf("a visible, enabled command did not run (runs=%d)", runs)
	}
	if ExecutePluginCommand(vfs.PluginCommandPanel, "cov.execute", nil) || runs != 1 {
		t.Errorf("a command ran from the wrong location (runs=%d)", runs)
	}

	enabled = false
	if ExecutePluginCommand(vfs.PluginCommandConfig, "cov.execute", nil) || runs != 1 {
		t.Errorf("a disabled command ran (runs=%d)", runs)
	}
	enabled = true

	visible = false
	if ExecutePluginCommand(vfs.PluginCommandConfig, "cov.execute", nil) || runs != 1 {
		t.Errorf("a hidden command ran (runs=%d)", runs)
	}
	visible = true

	stub.stale = true
	if ExecutePluginCommand(vfs.PluginCommandConfig, "cov.execute", nil) || runs != 1 {
		t.Errorf("a command ran against a stale app object (runs=%d)", runs)
	}
	stub.stale = false

	if ExecutePluginCommand(vfs.PluginCommandConfig, "cov.no-such-command", nil) {
		t.Error("an unknown command id was reported as executed")
	}
}

func TestPluginCommandsSnapshotFiltersByLocationAndVisibility(t *testing.T) {
	visible := false
	register := func(command vfs.PluginCommand) {
		t.Helper()
		command.Run = func(vfs.App) {}
		registration, err := RegisterPluginCommand(command)
		if err != nil {
			t.Fatalf("RegisterPluginCommand(%s): %v", command.ID, err)
		}
		t.Cleanup(registration.Unregister)
	}
	register(vfs.PluginCommand{ID: "cov.snap.panel", Label: "Panel", Location: vfs.PluginCommandPanel})
	register(vfs.PluginCommand{ID: "cov.snap.config", Label: "Config", Location: vfs.PluginCommandConfig})
	register(vfs.PluginCommand{ID: "cov.snap.hidden", Label: "Hidden", Location: vfs.PluginCommandPanel, Visible: func(vfs.App) bool { return visible }})

	ids := func(location vfs.PluginCommandLocation) []string {
		var out []string
		for _, command := range PluginCommandsSnapshot(location, nil) {
			if strings.HasPrefix(command.ID, "cov.snap.") {
				out = append(out, command.ID)
			}
		}
		return out
	}
	if got := ids(vfs.PluginCommandPanel); len(got) != 1 || got[0] != "cov.snap.panel" {
		t.Fatalf("panel commands = %v, want only the visible panel command", got)
	}
	if got := ids(vfs.PluginCommandConfig); len(got) != 1 || got[0] != "cov.snap.config" {
		t.Fatalf("config commands = %v", got)
	}

	visible = true
	if got := ids(vfs.PluginCommandPanel); len(got) != 2 || got[0] != "cov.snap.panel" || got[1] != "cov.snap.hidden" {
		t.Fatalf("panel commands once visible = %v, want registration order", got)
	}
}

func TestRegisterPanelProviderRejectsIncompleteProviders(t *testing.T) {
	open := func(vfs.PanelContext) (vfs.PanelController, error) { return nil, nil }
	cases := []struct {
		name     string
		provider vfs.PanelProvider
		want     string
	}{
		{"empty id", vfs.PanelProvider{ID: " ", Title: "Title", Open: open}, "ID is empty"},
		{"empty title", vfs.PanelProvider{ID: "cov-panel-notitle", Title: "  ", Open: open}, "empty title"},
		{"no open handler", vfs.PanelProvider{ID: "cov-panel-noopen", Title: "Title"}, "no open handler"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			registration, err := RegisterPanelProvider(c.provider)
			if err == nil {
				registration.Unregister()
				t.Fatalf("RegisterPanelProvider accepted %+v", c.provider)
			}
			if !strings.Contains(err.Error(), c.want) {
				t.Errorf("error = %q, want it to mention %q", err, c.want)
			}
			if _, ok := LookupPanelProvider(c.provider.ID); ok && strings.TrimSpace(c.provider.ID) != "" {
				t.Error("a rejected provider was left in the registry")
			}
		})
	}
}

func TestRegisterPanelProviderLifecycle(t *testing.T) {
	stub := &stubApplication{}
	useStubApplication(t, stub)

	open := func(vfs.PanelContext) (vfs.PanelController, error) { return nil, nil }
	registration, err := RegisterPanelProvider(vfs.PanelProvider{ID: " Cov-Panel ", Title: " Cov Panel ", Description: "desc", Open: open})
	if err != nil {
		t.Fatalf("RegisterPanelProvider: %v", err)
	}
	t.Cleanup(registration.Unregister)

	found, ok := LookupPanelProvider("  COV-PANEL")
	if !ok || found.ID != "Cov-Panel" || found.Title != "Cov Panel" {
		t.Fatalf("LookupPanelProvider = %+v, %v; want the trimmed provider found case-insensitively", found, ok)
	}

	// The provider is also a regular command, titled by the provider.
	command, ok := PluginCommandByID("panel.cov-panel")
	if !ok {
		t.Fatal("the provider's command was not registered")
	}
	if command.Label != "Cov Panel" || command.Location != vfs.PluginCommandPanel || command.Description != "desc" {
		t.Errorf("provider command = %+v", command)
	}
	if got := strings.Join(command.SearchTerms, ","); got != "open,panel,plugin,Cov-Panel" {
		t.Errorf("search terms = %q", got)
	}

	command.Run(nil)
	if len(stub.opened) != 1 || stub.opened[0] != "cov-panel" {
		t.Fatalf("running the command opened %v, want the lower-cased registry id", stub.opened)
	}
	App = nil
	command.Run(nil) // without a host application the command does nothing
	App = stub

	if dup, err := RegisterPanelProvider(vfs.PanelProvider{ID: "COV-panel", Title: "Other", Open: open}); err == nil {
		dup.Unregister()
		t.Fatal("a second provider with the same id was accepted")
	} else if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("duplicate error = %q", err)
	}

	registration.Unregister()
	if _, ok := LookupPanelProvider("cov-panel"); ok {
		t.Error("the provider is still registered after Unregister")
	}
	if _, ok := PluginCommandByID("panel.cov-panel"); ok {
		t.Error("the provider's command outlived the provider")
	}
}

func TestRegisterPanelProviderRollsBackWhenItsCommandCannotRegister(t *testing.T) {
	// Another plugin already owns the command id the provider would use.
	blocker, err := RegisterPluginCommand(vfs.PluginCommand{ID: "panel.cov-rollback", Label: "Blocker", Run: func(vfs.App) {}})
	if err != nil {
		t.Fatalf("RegisterPluginCommand: %v", err)
	}
	t.Cleanup(blocker.Unregister)

	open := func(vfs.PanelContext) (vfs.PanelController, error) { return nil, nil }
	registration, err := RegisterPanelProvider(vfs.PanelProvider{ID: "cov-rollback", Title: "Rollback", Open: open})
	if err == nil {
		registration.Unregister()
		t.Fatal("RegisterPanelProvider succeeded although its command id was taken")
	}
	if !strings.Contains(err.Error(), "command") {
		t.Errorf("error = %q, want it to name the failed command registration", err)
	}
	if _, ok := LookupPanelProvider("cov-rollback"); ok {
		t.Error("the provider stayed registered without its command")
	}
}
