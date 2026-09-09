package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/plughost"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// HostAPI defines the functions f4 exposes to plugins.
// coreAPI implements vfs.HostAPI.
type coreAPI struct{}

func (c *coreAPI) GetVersion() string {
	return getShortVersionInfo()
}

func (c *coreAPI) Log(msg string) {
	vtui.DebugLog("PLUGIN.LOG: %s", msg)
}

func (c *coreAPI) Message(msg string) {
	vtui.DebugLog("PLUGIN MESSAGE BOX: %s", msg)
	// Safely push to the main UI thread to avoid race conditions from background plugin loads
	vtui.FrameManager.PostTask(func() {
		vtui.ShowMessage(" Plugin Message ", msg, []string{"&Ok"})
	})
}
func (c *coreAPI) RegisterVFSProvider(p vfs.VFSProvider) {
	vtui.DebugLog("CORE: Registering VFS Provider: %s", p.Name())
	vfs.RegisterProvider(p)
}
func (c *coreAPI) RegisterURIProvider(p vfs.URIProvider) error {
	if err := vfs.RegisterURIProvider(p); err != nil {
		return err
	}
	vtui.DebugLog("CORE: Registered URI Provider: %s", p.Scheme())
	return nil
}
func (c *coreAPI) RegisterHighlighter(p vtui.HighlighterProvider) {
	vtui.DebugLog("CORE: Registering Highlighter: %s", p.Name())
	vtui.RegisterHighlighter(p)
}
func (c *coreAPI) RegisterDrive(name string, factory func() vfs.VFS) {
	sysinfo.RegisterDrive(name, factory)
}

func (c *coreAPI) RegisterGlobalHotkey(vk uint16, mods vtinput.ControlKeyState, handler func(app vfs.App)) {
	plughost.RegisterGlobalHotkey(vk, mods, handler)
}

func (c *coreAPI) RegisterPluginMenuItem(label string, handler func(app vfs.App)) {
	plughost.RegisterPluginMenuItem(label, handler)
}
func (c *coreAPI) RunAction(name string) bool {
	resChan := make(chan bool, 1)
	vtui.FrameManager.PostTask(func() {
		resChan <- RunAction(name)
	})
	return <-resChan
}

func (c *coreAPI) RegisterCommandPrefix(id, prefix string, handler func(vfs.App, string)) (vfs.CommandPrefixRegistration, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("command prefix registration ID is empty")
	}
	if handler == nil {
		return nil, fmt.Errorf("command prefix %q has no handler", id)
	}
	normalized, err := panel.NormalizeCommandPrefix(prefix)
	if err != nil {
		return nil, err
	}

	registration := &panel.CommandPrefixRegistration{Id: id, Prefix: normalized, Handler: handler, Active: true}
	panel.CommandPrefixRegistry.Lock()
	defer panel.CommandPrefixRegistry.Unlock()
	if _, exists := panel.CommandPrefixRegistry.ByID[id]; exists {
		return nil, fmt.Errorf("command prefix registration %q already exists", id)
	}
	if owner, exists := panel.CommandPrefixRegistry.ByPrefix[normalized]; normalized != "" && exists {
		return nil, fmt.Errorf("command prefix %q is already registered by %q", prefix, owner.Id)
	}
	panel.CommandPrefixRegistry.ByID[id] = registration
	if normalized != "" {
		panel.CommandPrefixRegistry.ByPrefix[normalized] = registration
	}
	return registration, nil
}
