package proclist

import (
	"testing"

	"github.com/unxed/f4/vfs"
)

type fakeRegistration struct{ unregistered bool }

func (r *fakeRegistration) Unregister() { r.unregistered = true }

// fakePanelHost implements vfs.PanelContributionHost (and embeds a nil
// vfs.HostAPI, matching the fake host pattern plugins/sqlite/plugin_test.go
// already uses for its own vfs.ContributionHost double).
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

// bareHost implements vfs.HostAPI without vfs.PanelContributionHost, so
// Init's type assertion on it fails on a platform where Supported() is true.
type bareHost struct{ vfs.HostAPI }

func TestInitRejectsNilHostAPI(t *testing.T) {
	if err := NewPlugin().Init(nil); err == nil {
		t.Fatal("Init(nil) should fail")
	}
}

func TestInitOnUnsupportedPlatformDoesNothing(t *testing.T) {
	if Supported() {
		t.Skip("this build supports ProcList; see TestInitRegistersPanelProviderWhenSupported")
	}
	host := &fakePanelHost{}
	if err := NewPlugin().Init(host); err != nil {
		t.Fatalf("Init returned an error on an unsupported platform: %v", err)
	}
	if host.provider.ID != "" {
		t.Fatalf("Init registered a panel provider on an unsupported platform: %#v", host.provider)
	}
}

func TestInitRegistersPanelProviderWhenSupported(t *testing.T) {
	if !Supported() {
		t.Skip("this build does not support ProcList; see TestInitOnUnsupportedPlatformDoesNothing")
	}

	if err := NewPlugin().Init(&bareHost{}); err == nil {
		t.Fatal("Init should reject a host without panel contribution support")
	}

	host := &fakePanelHost{}
	plugin := NewPlugin()
	if err := plugin.Init(host); err != nil {
		t.Fatal(err)
	}
	if host.provider.ID != panelProviderID || host.provider.Title == "" || host.provider.Open == nil {
		t.Fatalf("panel provider metadata = %#v", host.provider)
	}

	if err := plugin.Init(host); err == nil {
		t.Fatal("a second Init on an already-initialized plugin should fail")
	}

	controller, err := host.provider.Open(vfs.PanelContext{})
	if err != nil {
		t.Fatalf("Open returned an error: %v", err)
	}
	defer controller.Close()

	if err := plugin.Close(); err != nil {
		t.Fatal(err)
	}
	if host.reg == nil || !host.reg.unregistered {
		t.Fatal("Close did not unregister the panel provider")
	}
}
