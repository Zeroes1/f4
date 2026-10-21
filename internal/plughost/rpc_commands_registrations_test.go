package plughost

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"github.com/vmihailenco/msgpack/v5"
)

// orderedRegistration logs its teardown into a shared slice so a test can see
// the order a session unwinds in.
type orderedRegistration struct {
	name string
	log  *[]string
}

func (r *orderedRegistration) Unregister() { *r.log = append(*r.log, r.name) }

// contributionHostFake is a host with the command contribution API that
// records what a plugin registered through it.
type contributionHostFake struct {
	*luaTestHostAPI
	commands      []vfs.PluginCommand
	registrations []*rpcCoverageRegistration
	failWith      error
}

func (h *contributionHostFake) RegisterQuickViewProvider(vfs.QuickViewProvider) (vfs.Registration, error) {
	return nil, errors.New("not supported by the fake")
}

func (h *contributionHostFake) RegisterPluginCommand(command vfs.PluginCommand) (vfs.Registration, error) {
	if h.failWith != nil {
		return nil, h.failWith
	}
	registration := &rpcCoverageRegistration{}
	h.commands = append(h.commands, command)
	h.registrations = append(h.registrations, registration)
	return registration, nil
}

func (h *contributionHostFake) RegisterCommandPrefix(id, prefix string, handler func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	return nil, errors.New("not supported by the fake")
}

func (h *contributionHostFake) RegisterMacroCallProvider(vfs.MacroCallProvider) (vfs.Registration, error) {
	return nil, errors.New("not supported by the fake")
}

// failingPluginTransport answers every call with an error.
type failingPluginTransport struct {
	calls []string
}

func (f *failingPluginTransport) Call(method string, params, result any) error {
	f.calls = append(f.calls, method)
	return errors.New("plugin is gone")
}

// runNotifyingTransport reports every call it forwards on done. A command's
// Run hands Plugin.RunCommand to its own goroutine (startRPCCommand), so the
// UI stays free for the host prompts the command may ask for; a test reads
// what the inner transport recorded only after done says the call is over.
type runNotifyingTransport struct {
	inner PluginTransport
	done  chan string
}

func (n *runNotifyingTransport) Call(method string, params, result any) error {
	err := n.inner.Call(method, params, result)
	n.done <- method
	return err
}

func newRunNotifyingTransport(inner PluginTransport) *runNotifyingTransport {
	return &runNotifyingTransport{inner: inner, done: make(chan string, 1)}
}

func (n *runNotifyingTransport) wait(t *testing.T) {
	t.Helper()
	select {
	case <-n.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the command never reached the plugin transport")
	}
}

// activeVFSApp is a vfs.App whose only live part is the active panel's VFS.
type activeVFSApp struct {
	active vfs.VFS
}

func (a *activeVFSApp) GetActivePanelVFS() vfs.VFS  { return a.active }
func (a *activeVFSApp) GetPassivePanelVFS() vfs.VFS { return nil }
func (a *activeVFSApp) GetSelectedNames() []string  { return nil }
func (a *activeVFSApp) GetSelectedName() string     { return "" }
func (a *activeVFSApp) RefreshAll()                 {}
func (a *activeVFSApp) SetPendingSelection(name string) {
}
func (a *activeVFSApp) RunProgressTask(title, startMsg string, forked bool, worker func(ctx context.Context, update func(msg string, percent int)) error, onComplete func(err error)) {
}
func (a *activeVFSApp) RunAdvancedProgressTask(title string, forked bool, worker func(ctx context.Context, reporter vfs.TaskReporter) error, onComplete func(err error)) {
}
func (a *activeVFSApp) Message(title, msg string, buttons []string) int { return 0 }
func (a *activeVFSApp) InputBox(title, prompt, defaultText string, callback func(string)) {
}
func (a *activeVFSApp) Menu(title string, items []string, callback func(int)) {}

func TestPluginInitResponseRejectsUndecodableData(t *testing.T) {
	var response PluginInitResponse
	// 0xc1 is the one byte MessagePack never assigns: neither the legacy
	// drive list nor the structured response can be read from it.
	err := response.UnmarshalMsgpack([]byte{0xc1})
	if err == nil || !strings.Contains(err.Error(), "decode plugin init response") {
		t.Fatalf("UnmarshalMsgpack(garbage) = %v, want a decode error", err)
	}

	var none *PluginInitResponse
	if err := none.UnmarshalMsgpack(nil); err == nil {
		t.Fatal("UnmarshalMsgpack on a nil response did not fail")
	}
}

func TestPluginInitResponseLegacyListReplacesEarlierContent(t *testing.T) {
	data, err := msgpack.Marshal([]string{"A", "B"})
	if err != nil {
		t.Fatal(err)
	}
	response := PluginInitResponse{
		Drives:   []string{"stale"},
		Commands: []PluginCommandDescriptor{{ID: "stale"}},
		Panels:   []PluginPanelDescriptor{{ID: "stale"}},
	}
	if err := response.UnmarshalMsgpack(data); err != nil {
		t.Fatalf("UnmarshalMsgpack: %v", err)
	}
	if len(response.Drives) != 2 || response.Drives[0] != "A" || response.Drives[1] != "B" {
		t.Fatalf("drives = %v, want [A B]", response.Drives)
	}
	if response.Commands != nil || response.Panels != nil {
		t.Fatalf("the legacy format left commands %v and panels %v behind", response.Commands, response.Panels)
	}
}

func TestPluginSessionRegistrationsUnwindInReverseOrder(t *testing.T) {
	var log []string
	registrations := &PluginSessionRegistrations{}

	if registrations.Add(nil) {
		t.Fatal("Add accepted a nil registration")
	}
	if !registrations.Add(&orderedRegistration{name: "first", log: &log}) ||
		!registrations.Add(&orderedRegistration{name: "second", log: &log}) {
		t.Fatal("Add refused a registration on an open session")
	}

	registrations.Unregister()
	if got := strings.Join(log, ","); got != "second,first" {
		t.Fatalf("teardown order = %q, want second,first", got)
	}
	registrations.Unregister()
	if len(log) != 2 {
		t.Fatalf("a second Unregister ran teardowns again: %v", log)
	}

	// A registration that arrives after the session ended is removed at once
	// instead of escaping it.
	if registrations.Add(&orderedRegistration{name: "late", log: &log}) {
		t.Fatal("Add accepted a registration on a closed session")
	}
	if got := strings.Join(log, ","); got != "second,first,late" {
		t.Fatalf("late registration was not torn down immediately: %q", got)
	}

	var none *PluginSessionRegistrations
	none.Unregister()
}

func TestDescriptorFallbackLabel(t *testing.T) {
	cases := []struct {
		name       string
		descriptor PluginCommandDescriptor
		want       string
	}{
		{"plain label wins", PluginCommandDescriptor{Label: "  Open  ", LocalizedLabels: map[string]string{"en": "English"}}, "Open"},
		{"english when no label", PluginCommandDescriptor{LocalizedLabels: map[string]string{"en": " English ", "de": "Deutsch"}}, "English"},
		{"first language alphabetically otherwise", PluginCommandDescriptor{LocalizedLabels: map[string]string{"ru": "Русский", "de": "Deutsch", "fr": " "}}, "Deutsch"},
		{"blank translations are not labels", PluginCommandDescriptor{Label: " ", LocalizedLabels: map[string]string{"en": "", "ru": "  "}}, ""},
		{"nothing at all", PluginCommandDescriptor{}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := descriptorFallbackLabel(c.descriptor); got != c.want {
				t.Errorf("descriptorFallbackLabel = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRPCPluginCommandVisibleFollowsTheActiveDrive(t *testing.T) {
	notes := NewRPCVFS(&recordingPluginTransport{}, "Notes")

	if !rpcPluginCommandVisible(nil, nil) {
		t.Error("a command without drive constraints must be visible everywhere")
	}
	if rpcPluginCommandVisible([]string{"Notes"}, nil) {
		t.Error("a drive-bound command was visible without an app")
	}
	if rpcPluginCommandVisible([]string{"Notes"}, &activeVFSApp{}) {
		t.Error("a drive-bound command was visible on a panel that is not a plugin drive")
	}
	if rpcPluginCommandVisible([]string{"Notes"}, &activeVFSApp{active: (*RPCVFS)(nil)}) {
		t.Error("a drive-bound command was visible on a nil plugin drive")
	}
	if !rpcPluginCommandVisible([]string{"Other", "  nOTES "}, &activeVFSApp{active: notes}) {
		t.Error("the drive name must match case-insensitively and ignoring blanks")
	}
	if rpcPluginCommandVisible([]string{"Other"}, &activeVFSApp{active: notes}) {
		t.Error("a command bound to another drive was visible")
	}
}

func TestRegisterRPCPluginCommandsRejectsBadDeclarations(t *testing.T) {
	back := &recordingPluginTransport{}
	registrations := &PluginSessionRegistrations{}
	good := PluginCommandDescriptor{ID: "ok", Label: "OK"}

	if err := RegisterRPCPluginCommands(newLuaTestHostAPI(), back, "p", nil, registrations); err != nil {
		t.Fatalf("no descriptors must be fine on any host, got %v", err)
	}
	if err := RegisterRPCPluginCommands(newLuaTestHostAPI(), back, "p", []PluginCommandDescriptor{good}, registrations); err == nil ||
		!strings.Contains(err.Error(), "no command contribution API") {
		t.Fatalf("a host without the contribution API: error = %v", err)
	}

	host := &contributionHostFake{luaTestHostAPI: newLuaTestHostAPI()}
	if err := RegisterRPCPluginCommands(host, nil, "p", []PluginCommandDescriptor{good}, registrations); err == nil ||
		!strings.Contains(err.Error(), "transport is nil") {
		t.Fatalf("a nil transport: error = %v", err)
	}

	cases := []struct {
		name       string
		descriptor PluginCommandDescriptor
		want       string
	}{
		{"empty id", PluginCommandDescriptor{ID: "  ", Label: "L"}, "empty ID"},
		{"no label", PluginCommandDescriptor{ID: "nolabel"}, "has no label"},
		{"bad location", PluginCommandDescriptor{ID: "badloc", Label: "L", Location: 7}, "invalid location 7"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := RegisterRPCPluginCommands(host, back, "p", []PluginCommandDescriptor{c.descriptor}, registrations)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("error = %v, want it to mention %q", err, c.want)
			}
		})
	}
	if len(host.commands) != 0 {
		t.Fatalf("rejected declarations still registered %d commands", len(host.commands))
	}
}

func TestRegisterRPCPluginCommandsRegistersAndRunsThroughTheTransport(t *testing.T) {
	host := &contributionHostFake{luaTestHostAPI: newLuaTestHostAPI()}
	back := &recordingPluginTransport{}
	transport := newRunNotifyingTransport(back)
	registrations := &PluginSessionRegistrations{}

	err := RegisterRPCPluginCommands(host, transport, "p", []PluginCommandDescriptor{
		{ID: " run ", Label: "Run", Description: "d", ActiveDrives: []string{"Notes"}},
		{ID: "localized", LocalizedLabels: map[string]string{"ru": "Запуск", "en": "Run EN"}, Location: uint8(vfs.PluginCommandConfig)},
	}, registrations)
	if err != nil {
		t.Fatalf("RegisterRPCPluginCommands: %v", err)
	}
	if len(host.commands) != 2 {
		t.Fatalf("registered %d commands, want 2", len(host.commands))
	}

	first, second := host.commands[0], host.commands[1]
	if first.ID != "run" || first.Label != "Run" || first.Description != "d" || first.Location != vfs.PluginCommandPanel {
		t.Errorf("first command = %+v", first)
	}
	if second.ID != "localized" || second.Label != "Run EN" || second.Location != vfs.PluginCommandConfig {
		t.Errorf("second command = %+v, want the English label as the fallback", second)
	}

	// The first command is bound to a drive, the second is visible anywhere.
	if first.Visible(nil) {
		t.Error("a drive-bound command was visible without an app")
	}
	if !second.Visible(nil) {
		t.Error("a command without drive constraints was hidden")
	}

	first.Run(nil)
	transport.wait(t)
	if len(back.calls) != 1 || back.calls[0] != "Plugin.RunCommand" {
		t.Fatalf("running the command called %v, want Plugin.RunCommand", back.calls)
	}

	// Ending the session removes every command it registered.
	registrations.Unregister()
	for i, registration := range host.registrations {
		if registration.called != 1 {
			t.Errorf("registration %d was torn down %d times, want once", i, registration.called)
		}
	}
}

func TestRegisterRPCPluginCommandsRunSurvivesAFailingPlugin(t *testing.T) {
	host := &contributionHostFake{luaTestHostAPI: newLuaTestHostAPI()}
	back := &failingPluginTransport{}
	transport := newRunNotifyingTransport(back)
	err := RegisterRPCPluginCommands(host, transport, "p", []PluginCommandDescriptor{{ID: "boom", Label: "Boom"}}, &PluginSessionRegistrations{})
	if err != nil {
		t.Fatalf("RegisterRPCPluginCommands: %v", err)
	}
	host.commands[0].Run(nil) // the failure is logged, not raised into the UI
	transport.wait(t)
	if len(back.calls) != 1 {
		t.Fatalf("the failing transport was called %d times, want once", len(back.calls))
	}
}

func TestRegisterRPCPluginCommandsReportsHostAndSessionFailures(t *testing.T) {
	descriptors := []PluginCommandDescriptor{{ID: "cmd", Label: "Cmd"}}

	host := &contributionHostFake{luaTestHostAPI: newLuaTestHostAPI(), failWith: errors.New("id taken")}
	err := RegisterRPCPluginCommands(host, &recordingPluginTransport{}, "p", descriptors, &PluginSessionRegistrations{})
	if err == nil || !strings.Contains(err.Error(), "id taken") || !strings.Contains(err.Error(), `"cmd"`) {
		t.Fatalf("a refusing host: error = %v, want the host's reason and the command id", err)
	}

	// The session ended while the plugin was still declaring commands: the
	// late command is removed again and the load fails.
	host = &contributionHostFake{luaTestHostAPI: newLuaTestHostAPI()}
	closed := &PluginSessionRegistrations{}
	closed.Unregister()
	err = RegisterRPCPluginCommands(host, &recordingPluginTransport{}, "p", descriptors, closed)
	if err == nil || !strings.Contains(err.Error(), "disconnected") {
		t.Fatalf("a closed session: error = %v, want a disconnected error", err)
	}
	if len(host.registrations) != 1 || host.registrations[0].called != 1 {
		t.Fatalf("the late command was not removed: %+v", host.registrations)
	}
}

// capturingHost records the contributions a plugin hands to the host.
type capturingHost struct {
	*luaTestHostAPI
	highlighter vtui.HighlighterProvider
	hotkeyVK    uint16
	hotkeyMods  vtinput.ControlKeyState
	hotkey      func(app vfs.App)
}

func (h *capturingHost) RegisterHighlighter(p vtui.HighlighterProvider) { h.highlighter = p }

func (h *capturingHost) RegisterGlobalHotkey(vk uint16, mods vtinput.ControlKeyState, handler func(app vfs.App)) {
	h.hotkeyVK, h.hotkeyMods, h.hotkey = vk, mods, handler
}

// highlightTransport answers VFS.Highlight from a fixed result.
type highlightTransport struct {
	result HighlightRes
	err    error
	asked  HighlightReq
}

func (h *highlightTransport) Call(method string, params, result any) error {
	if method != "VFS.Highlight" {
		return errors.New("unexpected method " + method)
	}
	h.asked = params.(HighlightReq)
	if h.err != nil {
		return h.err
	}
	*(result.(*HighlightRes)) = h.result
	return nil
}

func TestHostMethodsRegisterAPluginHighlighter(t *testing.T) {
	host := &capturingHost{luaTestHostAPI: newLuaTestHostAPI()}
	transport := &highlightTransport{result: HighlightRes{Attrs: []uint64{1, 2, 3}, Next: "state"}}
	methods := newHostMethods(host, transport, "syntax-plugin", nil)

	callHostMethod(t, methods["Host.RegisterHighlighter"], nil)
	if host.highlighter == nil {
		t.Fatal("the highlighter was not handed to the host")
	}
	if host.highlighter.Name() != "syntax-plugin" || !host.highlighter.Match("any.file", "any content") {
		t.Errorf("provider = %q, match = %v", host.highlighter.Name(), host.highlighter.Match("any.file", "any content"))
	}

	attrs, next := host.highlighter.Create("any.file", "content").Highlight("a line", "prev", 9)
	if len(attrs) != 3 || attrs[2] != 3 || next != "state" {
		t.Fatalf("Highlight = %v, %v", attrs, next)
	}
	if transport.asked.Line != "a line" || transport.asked.Prev != "prev" || transport.asked.Base != 9 {
		t.Fatalf("the plugin was asked %+v", transport.asked)
	}

	transport.err = errors.New("plugin crashed")
	attrs, next = host.highlighter.Create("any.file", "content").Highlight("a line", nil, 0)
	if attrs != nil || next != nil {
		t.Fatalf("a failing plugin must leave the line unhighlighted, got %v, %v", attrs, next)
	}
}

func TestHostMethodsForwardAGlobalHotkeyToThePlugin(t *testing.T) {
	host := &capturingHost{luaTestHostAPI: newLuaTestHostAPI()}
	back := &recordingPluginTransport{}
	methods := newHostMethods(host, back, "hotkey-plugin", nil)

	callHostMethod(t, methods["Host.RegisterGlobalHotkey"], HotkeyReq{VK: 0x75, Mods: 3})
	if host.hotkey == nil || host.hotkeyVK != 0x75 || host.hotkeyMods != vtinput.ControlKeyState(3) {
		t.Fatalf("hotkey = %#x mods %v registered=%v", host.hotkeyVK, host.hotkeyMods, host.hotkey != nil)
	}
	host.hotkey(nil)
	if len(back.calls) != 1 || back.calls[0] != "Plugin.OnHotkey" {
		t.Fatalf("pressing the hotkey called %v, want Plugin.OnHotkey", back.calls)
	}
}

func TestHostMethodsAskTheApplicationAboutOverwritesAndErrors(t *testing.T) {
	stub := &stubApplication{overwriteChoice: 3, overwriteRemember: true, errorChoice: 2}
	useStubApplication(t, stub)
	methods := newHostMethods(newLuaTestHostAPI(), &recordingPluginTransport{}, "p", nil)

	got := callHostMethod(t, methods["Host.AskOverwrite"], AskOverwriteReq{Path: "dest.txt"})
	if got != (AskOverwriteRes{Choice: 3, Remember: true}) {
		t.Fatalf("Host.AskOverwrite = %+v", got)
	}
	if stub.askedPath != "dest.txt" || stub.overwriteCtx == nil {
		t.Fatalf("the application was asked about %q with context %v", stub.askedPath, stub.overwriteCtx)
	}

	if got := callHostMethod(t, methods["Host.AskError"], AskErrorReq{Op: "copy", Err: "disk full"}); got != 2 {
		t.Fatalf("Host.AskError = %v, want the application's choice 2", got)
	}
	if stub.askedOp != "copy" || stub.askedErr == nil || stub.askedErr.Error() != "disk full" || stub.errorCtx == nil {
		t.Fatalf("the application was asked op=%q err=%v ctx=%v", stub.askedOp, stub.askedErr, stub.errorCtx)
	}
}
