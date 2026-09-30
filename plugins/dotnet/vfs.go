package dotnet

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"sort"
	"strings"

	"github.com/unxed/f4/internal/dotnet"
	"github.com/unxed/f4/vfs"
)

var errReadOnly = errors.New(".NET assembly panel is read-only")

// node is one folder or file of the assembly's tree.
type node struct {
	dir   bool
	data  []byte
	names []string
	kids  map[string]*node
}

func newDir() *node { return &node{dir: true, kids: make(map[string]*node)} }

// add puts a child under n, keeping the first of two equal names apart from
// the second by a numeric suffix.
func (n *node) add(name string, child *node) {
	name = strings.NewReplacer("/", "_", "\\", "_", "\x00", "_").Replace(name)
	if name == "" {
		name = "_"
	}
	unique := name
	for i := 2; n.kids[unique] != nil && i < 1000; i++ {
		unique = fmt.Sprintf("%s (%d)", name, i)
	}
	if n.kids[unique] != nil {
		return
	}
	n.kids[unique] = child
	n.names = append(n.names, unique)
}

func file(text string) *node { return &node{data: []byte(text)} }

// buildTree lays an assembly out as folders: assembly.md (the report),
// References, Namespaces/<namespace>/<type> and Resources.
func buildTree(info *dotnet.Info, name string) *node {
	root := newDir()
	root.add("assembly.md", file(dotnet.Report(info, name)))
	refs := newDir()
	for _, ref := range info.References {
		refs.add(ref.Name+" "+ref.Version.String(), file(ref.Name+", Version="+ref.Version.String()+"\n"))
	}
	root.add("References", refs)
	spaces := newDir()
	for _, ns := range info.Namespaces() {
		dir := newDir()
		label := ns
		if label == "" {
			label = "(global)"
		}
		types := append([]string(nil), info.Types[ns]...)
		sort.Strings(types)
		for _, t := range types {
			key := t
			if ns != "" {
				key = ns + "." + t
			}
			body := "namespace " + ns + "\ntype " + t + "\n"
			for _, m := range info.Members[key] {
				body += m.Kind + " " + m.Name + "\n"
			}
			dir.add(t, file(body))
		}
		spaces.add(label, dir)
	}
	root.add("Namespaces", spaces)
	if len(info.Resources) > 0 {
		res := newDir()
		bytesOf := make(map[string][]byte, len(info.Blobs))
		for _, blob := range info.Blobs {
			if _, dup := bytesOf[blob.Name]; !dup {
				bytesOf[blob.Name] = blob.Data
			}
		}
		for _, r := range info.Resources {
			if data, ok := bytesOf[r]; ok {
				res.add(r, &node{data: data})
				continue
			}
			res.add(r, file("resource "+r+" is not stored in this file\n"))
		}
		root.add("Resources", res)
	}
	return root
}

// assemblyVFS is a read-only vfs.VFS over the tree of one assembly.
type assemblyVFS struct {
	parent vfs.VFS
	name   string
	root   *node
	path   string
}

func newAssemblyVFS(parent vfs.VFS, name string, info *dotnet.Info) *assemblyVFS {
	return &assemblyVFS{parent: parent, name: name, root: buildTree(info, name), path: "/"}
}

func (v *assemblyVFS) GetTitle() string { return v.name }

func (v *assemblyVFS) PanelTitle(p string) string {
	key := v.key(p)
	if key == "" {
		return ".NET:" + v.name
	}
	return ".NET:" + v.name + "/" + key
}

func (v *assemblyVFS) IsAtRoot() bool             { return v.path == "" || v.path == "/" }
func (v *assemblyVFS) GetPath() string            { return v.path }
func (v *assemblyVFS) IsAbs(p string) bool        { return path.IsAbs(p) }
func (v *assemblyVFS) Join(elem ...string) string { return path.Join(elem...) }
func (v *assemblyVFS) Base(p string) string       { return path.Base(p) }
func (v *assemblyVFS) Dir(p string) string        { return path.Dir(p) }

func (v *assemblyVFS) abs(p string) string {
	if p == "" {
		return v.path
	}
	if path.IsAbs(p) {
		return path.Clean(p)
	}
	return path.Join(v.path, p)
}

func (v *assemblyVFS) key(p string) string { return strings.Trim(v.abs(p), "/") }

func (v *assemblyVFS) Abs(p string) (string, error) { return v.abs(p), nil }

// lookup finds the node at a slash-separated key ("" is the root).
func (v *assemblyVFS) lookup(key string) *node {
	n := v.root
	if key == "" {
		return n
	}
	for _, part := range strings.Split(key, "/") {
		if n == nil || !n.dir {
			return nil
		}
		n = n.kids[part]
	}
	return n
}

func (v *assemblyVFS) SetPath(p string) error {
	key := v.key(p)
	if n := v.lookup(key); n == nil || !n.dir {
		return os.ErrInvalid
	}
	v.path = "/" + key
	return nil
}

func itemOf(name string, n *node) vfs.VFSItem {
	item := vfs.VFSItem{Name: name, IsDir: n.dir}
	if !n.dir {
		item.Size = int64(len(n.data))
		item.SizeKnown = true
	}
	return item
}

func (v *assemblyVFS) ReadDir(_ context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	n := v.lookup(v.key(p))
	if n == nil || !n.dir {
		return os.ErrNotExist
	}
	items := make([]vfs.VFSItem, 0, len(n.names))
	for _, name := range n.names {
		items = append(items, itemOf(name, n.kids[name]))
	}
	if onChunk != nil {
		onChunk(items)
	}
	return nil
}

func (v *assemblyVFS) Stat(_ context.Context, p string) (vfs.VFSItem, error) {
	key := v.key(p)
	n := v.lookup(key)
	if n == nil {
		return vfs.VFSItem{}, os.ErrNotExist
	}
	name := path.Base("/" + key)
	if key == "" {
		name = v.name
	}
	return itemOf(name, n), nil
}

func (v *assemblyVFS) MkDir(context.Context, string) error          { return errReadOnly }
func (v *assemblyVFS) Remove(context.Context, string) error         { return errReadOnly }
func (v *assemblyVFS) Rename(context.Context, string, string) error { return errReadOnly }
func (v *assemblyVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return errReadOnly
}
func (v *assemblyVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, errReadOnly
}

func (v *assemblyVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true}
}

func (v *assemblyVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }

// memFile serves the bytes of one node as a vfs.ReadAtCloser.
type memFile struct {
	data []byte
	pos  int64
}

func (m *memFile) Size() int64  { return int64(len(m.data)) }
func (m *memFile) Close() error { return nil }

func (m *memFile) ReadAt(ctx context.Context, p []byte, off int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if off < 0 || off >= int64(len(m.data)) {
		return 0, io.EOF
	}
	n := copy(p, m.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

func (m *memFile) Read(ctx context.Context, p []byte) (int, error) {
	n, err := m.ReadAt(ctx, p, m.pos)
	m.pos += int64(n)
	return n, err
}

func (v *assemblyVFS) Open(_ context.Context, p string) (vfs.ReadAtCloser, error) {
	n := v.lookup(v.key(p))
	if n == nil || n.dir {
		return nil, os.ErrNotExist
	}
	return &memFile{data: n.data}, nil
}

func (v *assemblyVFS) ParentVFS() vfs.VFS { return v.parent }

func (v *assemblyVFS) Clone() vfs.VFS {
	clone := *v
	return &clone
}

func (v *assemblyVFS) Close() error { return nil }

var (
	_ vfs.VFS                = (*assemblyVFS)(nil)
	_ vfs.TitleProvider      = (*assemblyVFS)(nil)
	_ vfs.PanelTitleProvider = (*assemblyVFS)(nil)
)
