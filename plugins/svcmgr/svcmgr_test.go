package svcmgr

import (
	"errors"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestStateName(t *testing.T) {
	want := map[uint32]string{
		stateStopped: "Stopped", stateStartPending: "Starting", stateStopPending: "Stopping",
		stateRunning: "Running", stateContinuePending: "Continuing", statePausePending: "Pausing",
		statePaused: "Paused", 42: "42",
	}
	for state, name := range want {
		if got := stateName(state); got != name {
			t.Errorf("stateName(%d) = %q, want %q", state, got, name)
		}
	}
}

func TestServiceRowCells(t *testing.T) {
	r := serviceRow{svc: service{Name: "Spooler", Display: "Print Spooler", State: stateRunning, PID: 1234}}
	got := []string{r.GetCellText(colName), r.GetCellText(colDisplay), r.GetCellText(colState), r.GetCellText(colPID), r.GetCellText(99)}
	want := []string{"Spooler", "Print Spooler", "Running", "1234", ""}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("cell %d = %q, want %q", i, got[i], want[i])
		}
	}
	if got := (serviceRow{svc: service{Name: "x"}}).GetCellText(colPID); got != "" {
		t.Errorf("a stopped service shows PID %q, want none", got)
	}
}

func fakeList(services *[]service, err *error) func() ([]service, error) {
	return func() ([]service, error) { return *services, *err }
}

func openFake(t *testing.T, services *[]service, err *error) *servicesPanel {
	t.Helper()
	c, openErr := newServicesPanel(vfs.PanelContext{Bounds: [4]int{0, 0, 59, 19}}, fakeList(services, err))
	if openErr != nil {
		t.Fatal(openErr)
	}
	return c.(*servicesPanel)
}

// TestPanelListsServicesAndKeepsTheCursorOnRefresh: the rows come from the
// list, F5 reloads it, and the cursor stays on the same service when the list
// changes around it.
func TestPanelListsServicesAndKeepsTheCursorOnRefresh(t *testing.T) {
	services := []service{
		{Name: "Alpha", Display: "Alpha svc", State: stateRunning, PID: 7},
		{Name: "Beta", Display: "Beta svc", State: stateStopped},
	}
	var listErr error
	p := openFake(t, &services, &listErr)
	if p.table.ItemCount != 2 {
		t.Fatalf("rows = %d, want 2", p.table.ItemCount)
	}
	p.selectByName("beta") // case-insensitive, as Windows service names are
	if got := p.GetSelectedName(); got != "Beta" {
		t.Fatalf("cursor on %q, want Beta", got)
	}

	services = []service{
		{Name: "Aardvark", Display: "A first one", State: stateRunning},
		{Name: "Alpha", Display: "Alpha svc", State: stateStopped},
		{Name: "Beta", Display: "Beta svc", State: stateRunning, PID: 9},
	}
	p.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5})
	if p.table.ItemCount != 3 {
		t.Fatalf("rows after F5 = %d, want 3", p.table.ItemCount)
	}
	if got := p.GetSelectedName(); got != "Beta" {
		t.Errorf("cursor after F5 on %q, want Beta", got)
	}

	// A failing reload keeps the old rows.
	listErr = errors.New("boom")
	p.refresh()
	if p.table.ItemCount != 3 {
		t.Errorf("rows after a failed reload = %d, want the old 3", p.table.ItemCount)
	}
}

func TestPanelOpenFailsWhenTheListFails(t *testing.T) {
	var none []service
	err := errors.New("no manager")
	if _, openErr := newServicesPanel(vfs.PanelContext{}, fakeList(&none, &err)); openErr == nil {
		t.Fatal("opening the panel succeeded although the list failed")
	}
}

func TestPanelGeometryFocusAndDraw(t *testing.T) {
	var services []service
	var err error
	p := openFake(t, &services, &err)
	if p.GetSelectedName() != "" {
		t.Error("an empty list has a selected name")
	}
	p.SetPosition(1, 2, 40, 10)
	if x1, y1, x2, y2 := p.GetPosition(); x1 != 1 || y1 != 2 || x2 != 40 || y2 != 10 {
		t.Errorf("position = %d,%d,%d,%d", x1, y1, x2, y2)
	}
	p.SetFocus(true)
	if !p.IsFocused() {
		t.Error("SetFocus(true) left the panel unfocused")
	}
	p.SetFocus(false)
	p.SetContext(vfs.PanelContext{})
	p.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(60, 20)
	p.Show(scr)
	if err := p.Close(); err != nil {
		t.Error(err)
	}
}

type fakeRegistration struct{ unregistered bool }

func (r *fakeRegistration) Unregister() { r.unregistered = true }

type fakePanelHost struct {
	vfs.HostAPI
	provider vfs.PanelProvider
	reg      *fakeRegistration
}

func (h *fakePanelHost) RegisterPanelProvider(p vfs.PanelProvider) (vfs.Registration, error) {
	h.provider = p
	h.reg = &fakeRegistration{}
	return h.reg, nil
}

func withSupported(t *testing.T, on bool) {
	t.Helper()
	old := supported
	supported = on
	t.Cleanup(func() { supported = old })
}

// TestPluginRegistersOnlyWhereSupported: off Windows Init registers nothing
// and succeeds; where supported it registers the provider once and Close
// unregisters it.
func TestPluginRegistersOnlyWhereSupported(t *testing.T) {
	p := NewPlugin()
	if p.GetName() != "SvcMgr" {
		t.Errorf("name = %q", p.GetName())
	}
	if err := p.Init(nil); err == nil {
		t.Error("nil host accepted")
	}

	withSupported(t, false)
	host := &fakePanelHost{}
	if err := p.Init(host); err != nil || host.reg != nil {
		t.Fatalf("unsupported Init: err = %v, registered = %v", err, host.reg != nil)
	}

	withSupported(t, true)
	if err := p.Init(struct{ vfs.HostAPI }{}); err == nil {
		t.Error("a host without panel contributions accepted")
	}
	if err := p.Init(host); err != nil {
		t.Fatal(err)
	}
	if host.provider.ID != panelProviderID || host.provider.Open == nil {
		t.Errorf("provider = %+v", host.provider)
	}
	if err := p.Init(host); err == nil {
		t.Error("second Init accepted")
	}
	if err := p.Close(); err != nil || !host.reg.unregistered {
		t.Errorf("Close: err = %v, unregistered = %v", err, host.reg.unregistered)
	}
}

func TestListServicesOffWindows(t *testing.T) {
	if Supported() {
		t.Skip("this OS has a service manager")
	}
	if _, err := listServices(); !errors.Is(err, errUnsupported) {
		t.Errorf("listServices error = %v, want errUnsupported", err)
	}
}

type fakeController struct {
	calls []string
	err   error
}

func (c *fakeController) Start(n string) error  { c.calls = append(c.calls, "start "+n); return c.err }
func (c *fakeController) Stop(n string) error   { c.calls = append(c.calls, "stop "+n); return c.err }
func (c *fakeController) Pause(n string) error  { c.calls = append(c.calls, "pause "+n); return c.err }
func (c *fakeController) Resume(n string) error { c.calls = append(c.calls, "resume "+n); return c.err }
func (c *fakeController) SetStartType(n string, t uint32, delayed bool) error {
	c.calls = append(c.calls, "starttype "+n+" "+startTypeName(t, delayed))
	return c.err
}

func TestPanelActionsActOnTheServiceUnderTheCursor(t *testing.T) {
	services := []service{
		{Name: "Run", Display: "A", State: stateRunning, PID: 3},
		{Name: "Stopped", Display: "B", State: stateStopped},
		{Name: "Paused", Display: "C", State: statePaused, PID: 4},
	}
	var listErr error
	p := openFake(t, &services, &listErr)
	ctl := &fakeController{}
	p.ctl = ctl

	p.selectByName("Run")
	p.pauseOrResume()
	p.selectByName("Paused")
	p.pauseOrResume()
	p.selectByName("Stopped")
	p.start()
	p.run(p.ctl.Stop)
	p.pauseOrResume() // a stopped service: nothing is sent
	want := []string{"pause Run", "resume Paused", "start Stopped", "stop Stopped"}
	if len(ctl.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", ctl.calls, want)
	}
	for i := range want {
		if ctl.calls[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, ctl.calls[i], want[i])
		}
	}

	// A failing action is reported and the list is still reloaded.
	ctl.err = errors.New("access denied")
	before := len(ctl.calls)
	services = append(services, service{Name: "New", Display: "D", State: stateRunning})
	p.start()
	if len(ctl.calls) != before+1 || p.table.ItemCount != 4 {
		t.Errorf("after a failed action: calls %d (want %d), rows %d (want 4)", len(ctl.calls), before+1, p.table.ItemCount)
	}
}

func TestPanelActionsNeedAService(t *testing.T) {
	var services []service
	var err error
	p := openFake(t, &services, &err)
	ctl := &fakeController{}
	p.ctl = ctl
	p.start()
	p.pauseOrResume()
	p.confirmStop()
	if len(ctl.calls) != 0 || p.hasSelected() {
		t.Errorf("an empty list ran %v", ctl.calls)
	}
	if got := len(p.PanelKeys()); got != 6 {
		t.Errorf("panel keys = %d, want 6", got)
	}
}

func TestPlatformControllerOffWindows(t *testing.T) {
	if Supported() {
		t.Skip("this OS has a service manager")
	}
	c := platformController{}
	for i, err := range []error{c.Start("x"), c.Stop("x"), c.Pause("x"), c.Resume("x"), c.SetStartType("x", startManual, false)} {
		if !errors.Is(err, errUnsupported) {
			t.Errorf("action %d error = %v, want errUnsupported", i, err)
		}
	}
}

func TestStartTypeName(t *testing.T) {
	cases := []struct {
		t       uint32
		delayed bool
		want    string
	}{
		{startBoot, false, "Boot"}, {startSystem, false, "System"}, {startAuto, false, "Automatic"},
		{startAuto, true, "Automatic (delayed start)"}, {startManual, false, "Manual"},
		{startDisabled, false, "Disabled"}, {9, false, "9"},
	}
	for _, c := range cases {
		if got := startTypeName(c.t, c.delayed); got != c.want {
			t.Errorf("startTypeName(%d, %v) = %q, want %q", c.t, c.delayed, got, c.want)
		}
	}
}

type fakeDetailer struct {
	d   serviceDetails
	err error
}

func (f fakeDetailer) Details(string) (serviceDetails, error) { return f.d, f.err }

func TestDetailsTextAndEnter(t *testing.T) {
	text := detailsText(service{Name: "Spooler", Display: "Print Spooler", State: stateRunning},
		serviceDetails{StartType: startAuto, Delayed: true, Account: "LocalSystem", BinaryPath: `C:\x\spool.exe`, Description: "Prints.", DependsOn: []string{"RPCSS", "http"},
			Recovery: []recoveryAction{{recoverRestart, 60}, {recoverNone, 0}}})
	for _, want := range []string{"Spooler", "Print Spooler", "Running", "Automatic (delayed start)", "LocalSystem", `C:\x\spool.exe`, "Prints.", "RPCSS, http", "1. Restart the service after 60 s; 2. Do nothing"} {
		if !strings.Contains(text, want) {
			t.Errorf("details text lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(detailsText(service{Name: "a"}, serviceDetails{}), "\n\n") {
		t.Error("an empty description left a blank paragraph")
	}

	services := []service{{Name: "A", Display: "A", State: stateStopped}}
	var listErr error
	p := openFake(t, &services, &listErr)
	p.det = fakeDetailer{err: errors.New("denied")}
	p.showDetails() // a failure is a toast, not a crash
	p.det = fakeDetailer{d: serviceDetails{StartType: startManual}}
	p.showDetails()
	var none []service
	empty := openFake(t, &none, &listErr)
	empty.showDetails()
}

func TestPlatformDetailerOffWindows(t *testing.T) {
	if Supported() {
		t.Skip("this OS has a service manager")
	}
	if _, err := (platformDetailer{}).Details("x"); !errors.Is(err, errUnsupported) {
		t.Errorf("Details error = %v, want errUnsupported", err)
	}
}

func TestSetStartTypeAppliesToTheServiceUnderTheCursor(t *testing.T) {
	services := []service{{Name: "Svc", Display: "S", State: stateStopped}}
	var listErr error
	p := openFake(t, &services, &listErr)
	ctl := &fakeController{}
	p.ctl = ctl
	for _, st := range startTypeChoices {
		p.setStartType(st)
	}
	want := "starttype Svc Automatic|starttype Svc Automatic (delayed start)|starttype Svc Manual|starttype Svc Disabled"
	if got := strings.Join(ctl.calls, "|"); got != want {
		t.Errorf("calls = %q, want %q", got, want)
	}
	p.chooseStartType() // without a frame manager nothing opens
}

func TestRecoveryName(t *testing.T) {
	cases := map[recoveryAction]string{
		{recoverNone, 0}: "Do nothing", {recoverRestart, 0}: "Restart the service",
		{recoverReboot, 30}: "Restart the computer after 30 s", {recoverCommand, 5}: "Run a program after 5 s",
		{9, 0}: "9",
	}
	for a, want := range cases {
		if got := recoveryName(a); got != want {
			t.Errorf("recoveryName(%+v) = %q, want %q", a, got, want)
		}
	}
}
