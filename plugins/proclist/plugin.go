// Package proclist is f4's built-in ProcList plugin: a live list of running
// processes shown as a panel, in the spirit of FAR Manager 3's ProcList
// plugin (Plist.cpp/Pclass.cpp), used as a reference while scoping f4#312.
//
// v1 (this package, f4#312 part 1 of 4) is deliberately narrow: Linux only,
// view-only (PID, name, memory, CPU%), refreshed a few times a second.
// FAR3's ProcList also offers process management (F8 kill, Shift-F1/F2
// priority; f4#312 part 3) and rich metrics/handles/remote view built on
// WMI and undocumented NT APIs that have no portable equivalent; the owner
// confirmed (f4#312) v1 should not attempt those, and cross-platform support
// (Windows via Toolhelp/NtQuerySystemInformation, macOS/*BSD via
// sysctl(KERN_PROC), none of it through WMI) is part 2 of the plan.
//
// It is also f4's first consumer of vfs.PanelProvider/PanelController
// (vfs/contributions.go, internal/plughost/panel_providers.go): a plugin
// panel that owns its own drawing and key handling, rather than emulating a
// process list through the VFS provider API, which has no natural mapping
// for "a directory entry is a running process".
package proclist

import (
	"errors"
	"fmt"
	"sync"

	"github.com/unxed/f4/vfs"
)

// panelProviderID is also the ID plughost.RegisterPanelProvider derives its
// auto-generated "Open ProcList" command ID from: "panel." + this ID,
// lowercased (internal/plughost/panel_providers.go). internal/app's own
// menu row and hotkey (proclist_actions.go) call that derived command by ID,
// duplicated there as a constant for the same reason sqlite_actions.go
// duplicates plugins/sqlite's command ID rather than importing it.
const panelProviderID = "f4.proclist"

// Plugin exposes the process list as an in-process f4 panel plugin.
type Plugin struct {
	mu           sync.Mutex
	registration vfs.Registration
	initialized  bool
}

// NewPlugin constructs the built-in ProcList plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (p *Plugin) GetName() string { return "ProcList" }

// Init registers the panel provider. On a platform Supported() reports
// false for, it does nothing and returns nil: v1 is Linux-only by design
// (f4#312 part 1 of 4), which is not a load failure elsewhere -- the same
// way plugins/ios's core_access_stub.go leaves a capability quietly absent
// on a platform it does not cover yet.
func (p *Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("ProcList: nil host API")
	}
	if !Supported() {
		return nil
	}

	host, ok := api.(vfs.PanelContributionHost)
	if !ok {
		return errors.New("ProcList: host does not support panel contributions")
	}

	p.mu.Lock()
	if p.initialized {
		p.mu.Unlock()
		return errors.New("ProcList: plugin is already initialized")
	}
	p.mu.Unlock()

	registration, err := host.RegisterPanelProvider(vfs.PanelProvider{
		ID:          panelProviderID,
		Title:       "ProcList",
		Description: "Live list of running processes: PID, name, memory and CPU%",
		Open:        newProcListPanel,
	})
	if err != nil {
		return fmt.Errorf("ProcList: register panel provider: %w", err)
	}

	p.mu.Lock()
	p.registration = registration
	p.initialized = true
	p.mu.Unlock()
	return nil
}

func (p *Plugin) Close() error {
	p.mu.Lock()
	registration := p.registration
	p.registration = nil
	p.initialized = false
	p.mu.Unlock()
	if registration != nil {
		registration.Unregister()
	}
	return nil
}
