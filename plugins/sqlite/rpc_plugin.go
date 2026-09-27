package sqlite

import (
	"context"
	"fmt"
	"sync"

	"github.com/unxed/f4/sdk/f4plugin"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/f4/vfs/hostpath"
	"github.com/unxed/vtinput"
)

// RPCPlugin adapts the in-process SQLite VFS mount (databaseProvider,
// databaseVFS, databaseSession -- none of it changed by this file) to the
// subprocess RPC transport described in docs/PLUGINS.md and implemented by
// sdk/f4plugin. It is plugins/sqlite's f4#1178 part 1 of 4, mirroring
// plugins/cloudfox/rpc_plugin.go's own shape and its doc comment's
// reasoning.
//
// Unlike cloudfox's ~30 MB of cloud SDKs, this extraction sheds plugins/
// sqlite's own code, not its third-party SQL engine dependency itself:
// internal/sheet/store.go (the native ".f4s.sqlite" spreadsheet format, an
// unrelated feature) imports the very same github.com/ncruces/go-sqlite3
// driver directly and unconditionally, in both builds, so that dependency
// stays linked into f4 regardless of this file existing. The win here is
// architectural consistency (the same download-on-demand catalog as
// cloudfox/android/iOS) plus shedding this package's own, unique code --
// not a binary-size reduction the way cloudfox's own part 1 was.
//
// databaseVFS already speaks the host-agnostic, path-addressed vfs.VFS
// interface (ReadDir/Stat/Open/... taking a plain path string); this file
// only translates the RPC edge (sdk/f4plugin.Plugin's flat "drive + path"
// namespace) into it. That translation is narrower than cloudfox's own
// RPCPlugin needs, because databaseVFS's own model is already flat: a path
// is either the database file itself (dbPathFor recognizes it by its
// header, exactly as databaseProvider.CanOpen does) or that file joined
// with the escaped name of one of its tables (databaseVFS.tableOf, which
// this file's Stat/ReadDir/Open call into unmodified by handing it the raw
// RPC path -- databaseVFS.Abs already accepts an absolute path directly).
// dbPathFor resolves which of the two a path is, without assuming it is
// already mounted; mount then gets or creates the cached *databaseVFS.
//
// Known gap (f4#1178, same class of limitation as cloudfox's connection
// dialogs): the interactive "SQLite client" panel command -- browsing table
// rows, editing a cell, running arbitrary SQL (plugin.go's openCurrent,
// ui.go's browser) -- pushes a vtui.FrameManager frame directly in the host
// process. That has no equivalent on the RPC Host.* surface (Host.InputBox
// and Host.Menu are too thin for a live, pageable, editable grid, the same
// reason cloudfox's own rpc_plugin.go gives for not porting its
// add/edit-connection dialogs), and RunPluginCommand/Host has no callback to
// ask the host which file the panel cursor is even on. So this RPCPlugin
// does not implement f4plugin.CommandProvider at all: installing
// sqlite-plugin through PlugRing gets a read-only "SQLite" drive that
// mounts a database given its path (typed, pasted, or reached from a saved
// panel path/bookmark) exactly like the in-process panel does -- lists its
// tables, and reads one out as CSV (F3/F5, the same as the in-process
// databaseVFS.Open) -- but not the interactive client. Porting the client to
// the RPC surface (most likely via the PanelProvider/.vui mechanism
// docs/PLUGINS.md describes for panel-owning plugins) is follow-up work, not
// part 1 of the plan.
type RPCPlugin struct {
	mu      sync.Mutex
	dbs     map[string]*databaseVFS
	readers map[uint32]vfs.ReadAtCloser
	nextID  uint32
}

// NewRPCPlugin builds the RPC-facing adapter around a fresh, empty mount
// cache.
func NewRPCPlugin() *RPCPlugin {
	return &RPCPlugin{
		dbs:     make(map[string]*databaseVFS),
		readers: make(map[uint32]vfs.ReadAtCloser),
	}
}

func (p *RPCPlugin) Init(*f4plugin.Host) ([]string, error) {
	return []string{databaseHandlerName}, nil
}

// dbPathFor decides which database file, if any, governs a raw absolute RPC
// path: the path itself, when it has the SQLite header (databaseProvider's
// own recognition rule, by content rather than by name); otherwise its
// immediate parent, when the path names one of that database's tables the
// way tableFileName/databaseVFS.tableOf expect. It does not require the
// database to already be mounted, unlike databaseVFS.existingTable.
func dbPathFor(raw string) (string, bool) {
	clean := hostpath.Clean(raw)
	if clean == "" || clean == "." {
		return "", false
	}
	if hasSQLiteHeader(clean) {
		return clean, true
	}
	parent := hostpath.Dir(clean)
	if parent == "" || parent == clean {
		return "", false
	}
	return parent, hasSQLiteHeader(parent)
}

// mount returns the cached databaseVFS for dbPath, creating it the first
// time this path is seen. parent is nil: nothing this file calls
// (ReadDir/Stat/Open/MkDir/Remove/Rename) ever reaches databaseVFS.ParentVFS
// or Clone, both of which are the only readers of that field, and both of
// which are host-panel-navigation concepts this RPC transport has no use
// for.
func (p *RPCPlugin) mount(dbPath string) *databaseVFS {
	p.mu.Lock()
	defer p.mu.Unlock()
	if existing, ok := p.dbs[dbPath]; ok {
		return existing
	}
	mounted := newDatabaseVFS(nil, dbPath)
	p.dbs[dbPath] = mounted
	return mounted
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

// ReadDir lists a mounted database's tables. The drive's own root (an empty
// path, before any database has been named) has nothing to list: unlike
// cloudfox's manager VFS, there is no saved catalog of "known databases" to
// show there, only whatever absolute path the panel is given -- typed,
// pasted, or restored from a saved path/bookmark, all of which arrive as a
// non-empty path here.
func (p *RPCPlugin) ReadDir(drive, path string) ([]f4plugin.VFSItem, error) {
	ctx := context.Background()
	if hostpath.Clean(path) == "" {
		return nil, nil
	}
	dbPath, ok := dbPathFor(path)
	if !ok {
		return nil, fmt.Errorf("%s: %w", path, errNotADatabase)
	}
	mounted := p.mount(dbPath)
	var items []f4plugin.VFSItem
	err := mounted.ReadDir(ctx, path, func(chunk []vfs.VFSItem) {
		for _, entry := range chunk {
			items = append(items, convertItem(entry))
		}
	})
	return items, err
}

func (p *RPCPlugin) Stat(drive, path string) (f4plugin.VFSItem, error) {
	ctx := context.Background()
	if hostpath.Clean(path) == "" {
		return f4plugin.VFSItem{KnownMetadata: uint32(vfs.MetadataExplicit), Name: databaseHandlerName, IsDir: true}, nil
	}
	dbPath, ok := dbPathFor(path)
	if !ok {
		return f4plugin.VFSItem{}, fmt.Errorf("%s: %w", path, errNotADatabase)
	}
	item, err := p.mount(dbPath).Stat(ctx, path)
	if err != nil {
		return f4plugin.VFSItem{}, err
	}
	return convertItem(item), nil
}

// Open is what F3 and F5 read over this transport, exactly as it is for the
// in-process panel: the whole table as CSV (databaseVFS.Open ->
// exportTableToTemp). Opening the database file itself, rather than one of
// its tables, is not a file at all in this model and fails the same way it
// does in-process (databaseVFS.existingTable -> os.ErrNotExist).
func (p *RPCPlugin) Open(drive, path string) (uint32, int64, error) {
	ctx := context.Background()
	dbPath, ok := dbPathFor(path)
	if !ok {
		return 0, 0, fmt.Errorf("%s: %w", path, errNotADatabase)
	}
	reader, err := p.mount(dbPath).Open(ctx, path)
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
		return nil, fmt.Errorf("sqlite: unknown file handle %d", fileID)
	}
	buf := make([]byte, length)
	n, err := reader.ReadAt(context.Background(), buf, offset)
	return buf[:n], err
}

func (p *RPCPlugin) CloseFile(fileID uint32) error {
	p.mu.Lock()
	reader, ok := p.readers[fileID]
	delete(p.readers, fileID)
	p.mu.Unlock()
	if !ok {
		return fmt.Errorf("sqlite: unknown file handle %d", fileID)
	}
	return reader.Close()
}

// MkDir, Remove, Rename, Create and Write complete the sdk/f4plugin.Plugin
// interface with the same answer databaseVFS itself gives every mutation
// attempted through the panel: readOnlyError, unconditionally. Nothing
// through this transport can write to the database or its tables, exactly
// as nothing through the in-process panel can (see vfs.go's own comment on
// readOnlyError -- the client, not the panel, is where data changes, and
// this RPC transport does not offer the client at all, see the type doc).
func (p *RPCPlugin) MkDir(drive, path string) error              { return readOnlyError{} }
func (p *RPCPlugin) Remove(drive, path string) error             { return readOnlyError{} }
func (p *RPCPlugin) Rename(drive, oldPath, newPath string) error { return readOnlyError{} }
func (p *RPCPlugin) Create(drive, path string) (uint32, error)   { return 0, readOnlyError{} }
func (p *RPCPlugin) Write(fileID uint32, data []byte) error      { return readOnlyError{} }

// Highlight, ProcessKey, OnHotkey and OnProgressTask complete the
// sdk/f4plugin.Plugin interface. The SQLite drive offers no editor syntax
// highlighting, drive-scoped hotkeys or progress-task callback over this
// transport.
func (p *RPCPlugin) Highlight(line string, prev any, base uint64) ([]uint64, any, error) {
	return nil, nil, nil
}

func (p *RPCPlugin) ProcessKey(drive string, event vtinput.InputEvent) (bool, error) {
	return false, nil
}

func (p *RPCPlugin) OnHotkey(vk uint16, mods uint32) error { return nil }

func (p *RPCPlugin) OnProgressTask() error { return nil }

var _ f4plugin.Plugin = (*RPCPlugin)(nil)
