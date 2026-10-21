package cloudfox

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

// RPCPlugin adapts the in-process CloudFox VFS provider (Plugin, ManagerVFS,
// CloudVFS -- none of it changed by this file) to the subprocess RPC
// transport described in docs/PLUGINS.md and implemented by sdk/f4plugin.
//
// It exists because ManagerVFS and CloudVFS already speak the
// host-agnostic, path-addressed vfs.VFS interface (ReadDir/Stat/Open/...
// taking a plain path string); only the RPC edge needs translating. That
// edge is a single flat "drive + path" namespace (sdk/f4plugin.Plugin),
// while CloudFox's own in-process model is two-tier: ManagerVFS lists
// connections, and entering one used to mean the host swapping its active
// vfs.VFS pointer to a distinct CloudVFS (internal/plughost's
// connectionProvider, driven by vfs.App navigation). RPCPlugin re-implements
// that swap itself, keyed by the first path segment: a one-segment path
// ("/GDrive") names the connection entry as ManagerVFS sees it (so
// Remove/Rename there delete or rename the saved connection, matching the
// full build's F8/F6 behaviour, without touching the cloud storage's own
// root folder); anything with a second segment ("/GDrive/Documents/x.txt")
// delegates into that connection's lazily opened, cached CloudVFS session.
//
// Add/edit and vault unlock are available through the F11 menu and command
// palette, using cancellable host prompts with masked credential entry.
// OAuth profiles accept tokens; browser/device-code authorization and advanced
// provider settings remain available through the full build's profile dialog.
type RPCPlugin struct {
	plugin      *Plugin
	prompter    rpcPrompter
	options     Options
	hostProfile bool

	mu      sync.Mutex
	conns   map[string]vfs.VFS
	readers map[uint32]vfs.ReadAtCloser
	writers map[uint32]io.WriteCloser
	nextID  uint32
}

// NewRPCPlugin builds the RPC-facing adapter around a fresh CloudFox Plugin.
// It accepts the same Options as NewPlugin (zero or one value).
func NewRPCPlugin(values ...Options) *RPCPlugin {
	p := &RPCPlugin{
		plugin:  NewPlugin(values...),
		conns:   make(map[string]vfs.VFS),
		readers: make(map[uint32]vfs.ReadAtCloser),
		writers: make(map[uint32]io.WriteCloser),
	}
	p.hostProfile = len(values) == 0 || values[0].ConfigDir == ""
	if len(values) != 0 {
		p.options = values[0]
	}
	if p.plugin.vault != nil && (len(values) == 0 || values[0].PasswordPrompt == nil) {
		p.plugin.vault.prompt = rpcMasterPasswordPrompter{plugin: p}
	}
	return p
}

func (p *RPCPlugin) Init(host *f4plugin.Host) ([]string, error) {
	p.prompter = host
	if p.hostProfile {
		profile, err := host.Profile()
		if err != nil {
			return nil, fmt.Errorf("cloudfox: resolve host profile: %w", err)
		}
		if err := p.useProfile(profile); err != nil {
			return nil, err
		}
	}
	return []string{DriveName}, nil
}

func (p *RPCPlugin) useProfile(profile f4plugin.ProfileInfo) error {
	if profile.Dir == "" {
		return fmt.Errorf("cloudfox: host profile directory is empty")
	}
	options := p.options
	options.ConfigDir = profile.Dir
	options.Portable = profile.Portable
	p.plugin = NewPlugin(options)
	if p.plugin.vault != nil && options.PasswordPrompt == nil {
		p.plugin.vault.prompt = rpcMasterPasswordPrompter{plugin: p}
	}
	return nil
}

// splitConnPath separates the leading path segment (the connection name,
// from the manager's point of view) from whatever follows it, without
// requiring that connection to actually exist. Rename and Remove need this
// existence-free split for the destination side of the call; resolve below
// needs the existence-checked, session-opening version for everything else.
func splitConnPath(raw string) (name, rest string) {
	clean := strings.Trim(strings.ReplaceAll(raw, "\\", "/"), "/")
	if clean == "" || clean == "." {
		return "", ""
	}
	name, rest, _ = strings.Cut(clean, "/")
	return name, rest
}

// resolve maps a flat RPC path onto the vfs.VFS that owns it -- the
// connections list itself for the root, or a lazily opened and cached
// CloudVFS session for a named connection -- plus the remaining path to
// hand that VFS. The remaining path is always relative to the connection's
// own root: this adapter never calls SetPath, so CloudVFS's "current
// location" never moves and resolvePath's relative branch always measures
// from that same root.
func (p *RPCPlugin) resolve(ctx context.Context, raw string) (vfs.VFS, string, error) {
	name, rest := splitConnPath(raw)
	if name == "" {
		return p.plugin.manager(), "", nil
	}

	p.mu.Lock()
	if cached, ok := p.conns[name]; ok {
		p.mu.Unlock()
		return cached, rest, nil
	}
	p.mu.Unlock()

	connections, err := p.plugin.repo.List(ctx)
	if err != nil {
		return nil, "", err
	}
	var (
		connection Connection
		found      bool
	)
	for _, candidate := range connections {
		if candidate.Name == name {
			connection, found = candidate, true
			break
		}
	}
	if !found {
		return nil, "", fmt.Errorf("%w: %s", ErrConnectionNotFound, name)
	}

	opened, err := p.plugin.openConnection(ctx, p.plugin.manager(), connection, "", false)
	if err != nil {
		return nil, "", err
	}

	p.mu.Lock()
	if cached, ok := p.conns[name]; ok {
		// Lost a race with another RPC call opening the same connection.
		// Keep the winner, close the redundant session.
		p.mu.Unlock()
		_ = opened.Close()
		return cached, rest, nil
	}
	p.conns[name] = opened
	p.mu.Unlock()
	return opened, rest, nil
}

func (p *RPCPlugin) forgetConnection(name string) {
	p.mu.Lock()
	opened, ok := p.conns[name]
	if ok {
		delete(p.conns, name)
	}
	p.mu.Unlock()
	if ok {
		_ = opened.Close()
	}
}

func convertItem(item vfs.VFSItem) f4plugin.VFSItem {
	return f4plugin.VFSItem{
		KnownMetadata: uint32(item.KnownMetadata),
		SizeKnown:     item.SizeKnown,
		PhysicalSize:  item.PhysicalSize,
		ATime:         item.ATime,
		CTime:         item.CTime,
		UnixMode:      item.UnixMode,
		Uid:           item.Uid,
		Gid:           item.Gid,
		WinAttrs:      item.WinAttrs,
		NoExtension:   item.NoExtension,
		IsSymlink:     item.IsSymlink,
		ReparseTag:    item.ReparseTag,
		Name:          item.Name,
		Size:          item.Size,
		IsDir:         item.IsDir,
		MTime:         item.MTime,
		Mode:          item.Mode,
		IsExecutable:  item.IsExecutable,
		IsHidden:      item.IsHidden,
	}
}

func (p *RPCPlugin) ReadDir(drive, path string) ([]f4plugin.VFSItem, error) {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return nil, err
	}
	_, isManager := target.(*ManagerVFS)

	var items []f4plugin.VFSItem
	err = target.ReadDir(ctx, rest, func(chunk []vfs.VFSItem) {
		for _, entry := range chunk {
			if isManager && !entry.IsDir {
				// The built-in add row is an in-process semantic action.
				// RPC exposes it as an F11/command-palette command instead.
				continue
			}
			items = append(items, convertItem(entry))
		}
	})
	return items, err
}

func (p *RPCPlugin) Stat(drive, path string) (f4plugin.VFSItem, error) {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return f4plugin.VFSItem{}, err
	}
	item, err := target.Stat(ctx, rest)
	if err != nil {
		return f4plugin.VFSItem{}, err
	}
	return convertItem(item), nil
}

func (p *RPCPlugin) Open(drive, path string) (uint32, int64, error) {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return 0, 0, err
	}
	reader, err := target.Open(ctx, rest)
	if err != nil {
		return 0, 0, err
	}
	p.mu.Lock()
	p.nextID++
	id := p.nextID
	p.readers[id] = reader
	p.mu.Unlock()
	return id, reader.Size(), nil
}

func (p *RPCPlugin) ReadAt(fileID uint32, length int, offset int64) ([]byte, error) {
	p.mu.Lock()
	reader, ok := p.readers[fileID]
	p.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("cloudfox: unknown file handle %d", fileID)
	}
	buf := make([]byte, length)
	n, err := reader.ReadAt(context.Background(), buf, offset)
	return buf[:n], err
}

func (p *RPCPlugin) Create(drive, path string) (uint32, error) {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return 0, err
	}
	writer, err := target.Create(ctx, rest)
	if err != nil {
		return 0, err
	}
	p.mu.Lock()
	p.nextID++
	id := p.nextID
	p.writers[id] = writer
	p.mu.Unlock()
	return id, nil
}

func (p *RPCPlugin) Write(fileID uint32, data []byte) error {
	p.mu.Lock()
	writer, ok := p.writers[fileID]
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("cloudfox: unknown file handle %d", fileID)
	}
	_, err := writer.Write(data)
	return err
}

func (p *RPCPlugin) CloseFile(fileID uint32) error {
	p.mu.Lock()
	reader, isReader := p.readers[fileID]
	delete(p.readers, fileID)
	writer, isWriter := p.writers[fileID]
	delete(p.writers, fileID)
	p.mu.Unlock()
	switch {
	case isReader:
		return reader.Close()
	case isWriter:
		return writer.Close()
	default:
		return fmt.Errorf("cloudfox: unknown file handle %d", fileID)
	}
}

func (p *RPCPlugin) MkDir(drive, path string) error {
	ctx := context.Background()
	target, rest, err := p.resolve(ctx, path)
	if err != nil {
		return err
	}
	return target.MkDir(ctx, rest)
}

// Remove deletes a file or folder inside an opened connection, or -- for a
// bare, one-segment path -- the saved connection itself (ManagerVFS.Remove),
// exactly like selecting the connection's row in the manager panel and
// pressing F8 in the full build. Routing a one-segment path into the opened
// CloudVFS instead would delete the cloud storage's own root folder, which
// is a materially different and far more dangerous operation.
func (p *RPCPlugin) Remove(drive, path string) error {
	ctx := context.Background()
	name, rest := splitConnPath(path)
	if rest == "" {
		if err := p.plugin.manager().Remove(ctx, name); err != nil {
			return err
		}
		p.forgetConnection(name)
		return nil
	}
	target, connRest, err := p.resolve(ctx, path)
	if err != nil {
		return err
	}
	return target.Remove(ctx, connRest)
}

// Rename follows the same one-segment-means-the-manager rule as Remove: a
// bare source name renames the saved connection (ManagerVFS.Rename), rather
// than renaming the cloud storage's own root folder.
func (p *RPCPlugin) Rename(drive, oldPath, newPath string) error {
	ctx := context.Background()
	oldName, oldRest := splitConnPath(oldPath)
	if oldRest == "" {
		newName, _ := splitConnPath(newPath)
		if err := p.plugin.manager().Rename(ctx, oldName, newName); err != nil {
			return err
		}
		p.forgetConnection(oldName)
		return nil
	}
	target, connOldRest, err := p.resolve(ctx, oldPath)
	if err != nil {
		return err
	}
	_, newRest := splitConnPath(newPath)
	return target.Rename(ctx, connOldRest, newRest)
}

// Highlight, ProcessKey, OnHotkey and OnProgressTask complete the
// sdk/f4plugin.Plugin interface. CloudFox does not offer editor syntax
// highlighting, drive-scoped hotkeys or a progress-task callback over this
// transport in part 1; long-running transfers already report progress
// through f4's normal copy/move UI, which does not go through this plugin
// interface at all.
func (p *RPCPlugin) Highlight(line string, prev any, base uint64) ([]uint64, any, error) {
	return nil, nil, nil
}

func (p *RPCPlugin) ProcessKey(drive string, event vtinput.InputEvent) (bool, error) {
	return false, nil
}

func (p *RPCPlugin) OnHotkey(vk uint16, mods uint32) error { return nil }

func (p *RPCPlugin) OnProgressTask() error { return nil }

var _ f4plugin.Plugin = (*RPCPlugin)(nil)
