package svcmgr

import (
	"errors"
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
