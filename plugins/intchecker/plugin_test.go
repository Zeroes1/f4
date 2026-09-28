package intchecker

import (
	"context"
	"errors"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

type hostMock struct {
	legacyLabel   string
	legacyHandler func(vfs.App)
}

func (*hostMock) GetVersion() string                                                  { return "test" }
func (*hostMock) Log(string)                                                          {}
func (*hostMock) Message(string)                                                      {}
func (*hostMock) RegisterHighlighter(vtui.HighlighterProvider)                        {}
func (*hostMock) RegisterVFSProvider(vfs.VFSProvider)                                 {}
func (*hostMock) RegisterURIProvider(vfs.URIProvider) error                           { return nil }
func (*hostMock) RegisterDrive(string, func() vfs.VFS)                                {}
func (*hostMock) RegisterGlobalHotkey(uint16, vtinput.ControlKeyState, func(vfs.App)) {}
func (host *hostMock) RegisterPluginMenuItem(label string, handler func(vfs.App)) {
	host.legacyLabel, host.legacyHandler = label, handler
}
func (*hostMock) RunAction(string) bool { return false }

type registrationMock struct{ unregistered int }

func (r *registrationMock) Unregister() { r.unregistered++ }

type contributionHostMock struct {
	*hostMock
	command      vfs.PluginCommand
	registration *registrationMock
	err          error
}

func (*contributionHostMock) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected quick-view registration")
}

func (host *contributionHostMock) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	host.command = command
	if host.err != nil {
		return nil, host.err
	}
	host.registration = &registrationMock{}
	return host.registration, nil
}

func (*contributionHostMock) RegisterCommandPrefix(string, string, func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	return nil, errors.New("unexpected command-prefix registration")
}

func (*contributionHostMock) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	return nil, errors.New("unexpected macro registration")
}

var _ vfs.HostAPI = (*hostMock)(nil)
var _ vfs.ContributionHost = (*contributionHostMock)(nil)

func TestPluginRegistersPanelCommandAndUnregistersIt(t *testing.T) {
	host := &contributionHostMock{hostMock: &hostMock{}}
	plugin := NewPlugin()
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if host.legacyHandler != nil {
		t.Fatal("rich host also received a legacy menu item")
	}
	command := host.command
	if command.ID != "intchecker.menu" || command.Location != vfs.PluginCommandPanel ||
		command.LabelKey != "IntChecker.Menu" || command.DescriptionKey != "IntChecker.Command.Desc" ||
		command.Run == nil || command.Enabled == nil {
		t.Fatalf("command metadata = %#v", command)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if host.registration.unregistered != 1 || plugin.api != nil || plugin.registration != nil {
		t.Fatalf("Close left state behind: unregistered=%d", host.registration.unregistered)
	}
}

func TestPluginFallsBackToLegacyMenu(t *testing.T) {
	host := &hostMock{}
	if err := NewPlugin().Init(host); err != nil {
		t.Fatal(err)
	}
	if host.legacyLabel == "" || host.legacyHandler == nil {
		t.Fatal("legacy host got no menu item")
	}
}

func TestPluginInitFailsWhenRegistrationFails(t *testing.T) {
	host := &contributionHostMock{hostMock: &hostMock{}, err: errors.New("injected")}
	plugin := NewPlugin()
	if err := plugin.Init(host); err == nil {
		t.Fatal("Init succeeded despite registration failure")
	}
	if plugin.api != nil || host.legacyHandler != nil {
		t.Fatal("failed Init kept state or installed a legacy item")
	}
}

type appMock struct {
	fs       vfs.VFS
	selected []string
	menu     []string
}

func (a *appMock) GetActivePanelVFS() vfs.VFS  { return a.fs }
func (a *appMock) GetPassivePanelVFS() vfs.VFS { return nil }
func (a *appMock) GetSelectedNames() []string  { return a.selected }
func (a *appMock) GetSelectedName() string     { return "" }
func (a *appMock) RefreshAll()                 {}
func (a *appMock) SetPendingSelection(string)  {}
func (a *appMock) RunProgressTask(string, string, bool, func(context.Context, func(string, int)) error, func(error)) {
}
func (a *appMock) RunAdvancedProgressTask(string, bool, func(context.Context, vfs.TaskReporter) error, func(error)) {
}
func (a *appMock) Message(string, string, []string) int          { return 0 }
func (a *appMock) InputBox(string, string, string, func(string)) {}
func (a *appMock) Menu(_ string, items []string, _ func(int))    { a.menu = items }

var _ vfs.App = (*appMock)(nil)

func TestCanRunNeedsAPanelFilesystem(t *testing.T) {
	if canRun(&appMock{}) {
		t.Fatal("enabled without a panel filesystem")
	}
	if !canRun(&appMock{fs: vfs.NewOSVFS(t.TempDir())}) {
		t.Fatal("disabled on a local panel")
	}
}

func TestSelectedFileNamesDropsParentEntry(t *testing.T) {
	got := selectedFileNames(&appMock{selected: []string{"..", "a.txt", "", "b"}})
	if len(got) != 2 || got[0] != "a.txt" || got[1] != "b" {
		t.Fatalf("selectedFileNames = %q", got)
	}
}

func TestMenuOffersGenerateHashes(t *testing.T) {
	app := &appMock{}
	NewPlugin().showMenu(app)
	if len(app.menu) != 1 || app.menu[menuGenerate] != vtui.Msg("IntChecker.Generate") {
		t.Fatalf("menu = %q", app.menu)
	}
}
