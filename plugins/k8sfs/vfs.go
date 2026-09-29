package k8sfs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/unxed/f4/vfs"
)

const (
	// statBatch is how many paths one stat command gets, to keep the command
	// line short.
	statBatch = 100
	// statTimeout bounds the quick calls made from SetPath.
	statTimeout = 30 * time.Second
)

var (
	errNotADirectory = errors.New("not a directory")
	errIsADirectory  = errors.New("is a directory")
)

// unsupportedError is what every change gets; it is os.ErrPermission to the
// file operations that ask.
type unsupportedError struct{}

func (unsupportedError) Error() string {
	return k8sText("K8s.ReadOnly",
		"The Kubernetes panel is read-only: copy files out with F5",
		"Панель Kubernetes только для чтения: файлы можно скопировать наружу клавишей F5")
}

func (unsupportedError) Is(target error) bool { return target == os.ErrPermission }

// k8sVFS is the Kubernetes panel. Paths are POSIX: "/" lists namespaces,
// "/<ns>" its pods, "/<ns>/<pod>" the pod's containers and
// "/<ns>/<pod>/<container>/..." the container's file system.
type k8sVFS struct {
	open func() (*restClient, error)

	mu     sync.Mutex
	cli    *restClient
	cwd    string
	closed bool
}

func newK8sVFS(open func() (*restClient, error)) *k8sVFS {
	return &k8sVFS{open: open, cwd: "/"}
}

func (v *k8sVFS) clientFor() (*restClient, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.closed {
		return nil, errors.New("Kubernetes: the panel is closed")
	}
	if v.cli != nil {
		return v.cli, nil
	}
	cli, err := v.open()
	if err != nil {
		return nil, err
	}
	v.cli = cli
	return cli, nil
}

func (v *k8sVFS) IsAtRoot() bool      { return v.GetPath() == "/" }
func (v *k8sVFS) IsAbs(p string) bool { return strings.HasPrefix(p, "/") }

func (v *k8sVFS) GetPath() string {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.cwd
}

func (v *k8sVFS) Join(elem ...string) string { return path.Join(elem...) }
func (v *k8sVFS) Base(p string) string       { return path.Base(path.Clean(p)) }
func (v *k8sVFS) Dir(p string) string        { return path.Dir(path.Clean(p)) }

func (v *k8sVFS) Abs(p string) (string, error) {
	if p == "" {
		return v.GetPath(), nil
	}
	if v.IsAbs(p) {
		return path.Clean(p), nil
	}
	return path.Join(v.GetPath(), p), nil
}

// location is a panel path taken apart. depth counts how many of namespace,
// pod and container it names (0 to 3); inner is the path inside the container
// ("/" at its root).
type location struct {
	ns, pod, container, inner string
	depth                     int
}

func parseLocation(abs string) location {
	rest := strings.Trim(path.Clean(abs), "/")
	if rest == "" {
		return location{inner: "/"}
	}
	parts := strings.SplitN(rest, "/", 4)
	loc := location{ns: parts[0], depth: 1, inner: "/"}
	if len(parts) > 1 {
		loc.pod, loc.depth = parts[1], 2
	}
	if len(parts) > 2 {
		loc.container, loc.depth = parts[2], 3
	}
	if len(parts) > 3 {
		loc.inner = "/" + parts[3]
	}
	return loc
}

func (v *k8sVFS) SetPath(p string) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), statTimeout)
	defer cancel()
	item, err := v.Stat(ctx, abs)
	if err != nil {
		return err
	}
	if !item.IsDir {
		return fmt.Errorf("%s: %w", abs, errNotADirectory)
	}
	v.mu.Lock()
	v.cwd = abs
	v.mu.Unlock()
	return nil
}

func dirItem(name string, mtime time.Time) vfs.VFSItem {
	return vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit | vfs.MetadataMTime,
		Name:          name,
		IsDir:         true,
		NoExtension:   true, // a pod called web.1 is not a file of type "1"
		MTime:         mtime,
	}
}

func (v *k8sVFS) ReadDir(ctx context.Context, p string, onChunk func([]vfs.VFSItem)) error {
	abs, err := v.Abs(p)
	if err != nil {
		return err
	}
	cli, err := v.clientFor()
	if err != nil {
		return err
	}
	loc := parseLocation(abs)
	send := func(items []vfs.VFSItem) {
		if len(items) > 0 && onChunk != nil {
			onChunk(items)
		}
	}
	switch loc.depth {
	case 0:
		names, err := cli.listNamespaces(ctx)
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(names))
		for _, n := range names {
			items = append(items, dirItem(n, time.Time{}))
		}
		send(items)
		return nil
	case 1:
		pods, err := cli.listPods(ctx, loc.ns)
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(pods))
		for _, pod := range pods {
			items = append(items, dirItem(pod.name, pod.created))
		}
		send(items)
		return nil
	case 2:
		pod, err := findPod(ctx, cli, loc.ns, loc.pod)
		if err != nil {
			return err
		}
		items := make([]vfs.VFSItem, 0, len(pod.containers))
		for _, c := range pod.containers {
			items = append(items, dirItem(c, pod.created))
		}
		send(items)
		return nil
	}
	return v.listInside(ctx, cli, loc, send)
}

func findPod(ctx context.Context, cli *restClient, ns, name string) (podInfo, error) {
	pods, err := cli.listPods(ctx, ns)
	if err != nil {
		return podInfo{}, err
	}
	for _, p := range pods {
		if p.name == name {
			return p, nil
		}
	}
	return podInfo{}, fmt.Errorf("%s/%s: %w", ns, name, os.ErrNotExist)
}

func run(ctx context.Context, cli *restClient, loc location, cmd ...string) (string, error) {
	var out bytes.Buffer
	err := cli.exec(ctx, loc.ns, loc.pod, loc.container, cmd, &out)
	return out.String(), err
}

// listInside lists a folder of a container: ls gives the names and which are
// folders (links to folders included), stat then adds sizes, times and modes
// where the container has stat.
func (v *k8sVFS) listInside(ctx context.Context, cli *restClient, loc location, send func([]vfs.VFSItem)) error {
	out, err := run(ctx, cli, loc, "ls", "-1ApL", "--", loc.inner)
	if err != nil && (out == "" || !errors.Is(err, errExecFailed)) {
		return err
	}
	var names []string
	isDir := map[string]bool{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		name := strings.TrimSuffix(line, "/")
		if name == "" {
			continue
		}
		names = append(names, name)
		isDir[name] = strings.HasSuffix(line, "/")
	}
	details := map[string]vfs.VFSItem{}
	for start := 0; start < len(names); start += statBatch {
		end := min(start+statBatch, len(names))
		args := []string{"stat", "-c", "%F|%s|%Y|%a|%n", "--"}
		for _, n := range names[start:end] {
			args = append(args, path.Join(loc.inner, n))
		}
		text, statErr := run(ctx, cli, loc, args...)
		if statErr != nil && text == "" {
			break // no stat here: names and folder flags are all there is
		}
		for _, line := range strings.Split(text, "\n") {
			if item, ok := parseStatLine(line); ok {
				details[item.Name] = item
			}
		}
	}
	items := make([]vfs.VFSItem, 0, len(names))
	for _, n := range names {
		item, ok := details[n]
		if !ok {
			item = vfs.VFSItem{KnownMetadata: vfs.MetadataExplicit | vfs.MetadataHidden, Name: n, IsHidden: strings.HasPrefix(n, ".")}
		}
		item.IsDir = isDir[n]
		items = append(items, item)
	}
	send(items)
	return ctx.Err()
}

// parseStatLine reads one line of `stat -c '%F|%s|%Y|%a|%n'`. Name is the base
// name of the path stat was given.
func parseStatLine(line string) (vfs.VFSItem, bool) {
	parts := strings.SplitN(strings.TrimRight(line, "\r"), "|", 5)
	if len(parts) != 5 || parts[4] == "" {
		return vfs.VFSItem{}, false
	}
	size, _ := strconv.ParseInt(parts[1], 10, 64)
	unix, _ := strconv.ParseInt(parts[2], 10, 64)
	perm, _ := strconv.ParseUint(parts[3], 8, 32)
	name := path.Base(parts[4])
	kind := parts[0]
	item := vfs.VFSItem{
		KnownMetadata: vfs.MetadataExplicit | vfs.MetadataPermissions | vfs.MetadataExecutable | vfs.MetadataHidden | vfs.MetadataMTime,
		Name:          name,
		IsDir:         kind == "directory",
		IsSymlink:     kind == "symbolic link",
		MTime:         time.Unix(unix, 0),
		UnixMode:      uint32(perm),
		IsExecutable:  perm&0o111 != 0,
		IsHidden:      strings.HasPrefix(name, "."),
	}
	if strings.HasPrefix(kind, "regular") {
		item.Size, item.SizeKnown = size, true
	}
	return item, true
}

func (v *k8sVFS) Stat(ctx context.Context, p string) (vfs.VFSItem, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return vfs.VFSItem{}, err
	}
	loc := parseLocation(abs)
	if loc.depth == 0 {
		return dirItem("Kubernetes", time.Time{}), nil
	}
	cli, err := v.clientFor()
	if err != nil {
		return vfs.VFSItem{}, err
	}
	switch loc.depth {
	case 1:
		names, err := cli.listNamespaces(ctx)
		if err != nil {
			return vfs.VFSItem{}, err
		}
		for _, n := range names {
			if n == loc.ns {
				return dirItem(n, time.Time{}), nil
			}
		}
		return vfs.VFSItem{}, fmt.Errorf("%s: %w", loc.ns, os.ErrNotExist)
	case 2:
		pod, err := findPod(ctx, cli, loc.ns, loc.pod)
		if err != nil {
			return vfs.VFSItem{}, err
		}
		return dirItem(pod.name, pod.created), nil
	}
	if loc.inner == "/" {
		pod, err := findPod(ctx, cli, loc.ns, loc.pod)
		if err != nil {
			return vfs.VFSItem{}, err
		}
		for _, c := range pod.containers {
			if c == loc.container {
				return dirItem(c, pod.created), nil
			}
		}
		return vfs.VFSItem{}, fmt.Errorf("%s: %w", abs, os.ErrNotExist)
	}
	out, err := run(ctx, cli, loc, "stat", "-c", "%F|%s|%Y|%a|%n", "--", loc.inner)
	if err != nil {
		if errors.Is(err, errExecFailed) {
			return vfs.VFSItem{}, fmt.Errorf("%s: %w", abs, os.ErrNotExist)
		}
		return vfs.VFSItem{}, err
	}
	item, ok := parseStatLine(strings.TrimSpace(out))
	if !ok {
		return vfs.VFSItem{}, fmt.Errorf("%s: unreadable stat output", abs)
	}
	if item.IsSymlink {
		if _, err := run(ctx, cli, loc, "test", "-d", loc.inner); err == nil {
			item.IsDir = true
		}
	}
	return item, nil
}

// Open copies a file out of the container with cat into a temporary file,
// which is what gives F3 and F5 random access.
func (v *k8sVFS) Open(ctx context.Context, p string) (vfs.ReadAtCloser, error) {
	abs, err := v.Abs(p)
	if err != nil {
		return nil, err
	}
	loc := parseLocation(abs)
	if loc.depth < 3 || loc.inner == "/" {
		return nil, fmt.Errorf("%s: %w", abs, errIsADirectory)
	}
	cli, err := v.clientFor()
	if err != nil {
		return nil, err
	}
	if item, err := v.Stat(ctx, abs); err != nil {
		return nil, err
	} else if item.IsDir {
		return nil, fmt.Errorf("%s: %w", abs, errIsADirectory)
	}
	file, err := os.CreateTemp("", "f4-k8s-*")
	if err != nil {
		return nil, err
	}
	tempPath := file.Name()
	fail := func(err error) (vfs.ReadAtCloser, error) {
		_ = file.Close()
		_ = os.Remove(tempPath)
		return nil, err
	}
	if err := cli.exec(ctx, loc.ns, loc.pod, loc.container, []string{"cat", "--", loc.inner}, file); err != nil {
		return fail(err)
	}
	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		return fail(err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fail(err)
	}
	return &vfs.TempFileWrapper{File: file, SizeVal: size, TempPath: tempPath}, nil
}

func (v *k8sVFS) MkDir(context.Context, string) error          { return unsupportedError{} }
func (v *k8sVFS) Remove(context.Context, string) error         { return unsupportedError{} }
func (v *k8sVFS) Rename(context.Context, string, string) error { return unsupportedError{} }
func (v *k8sVFS) SetAttributes(context.Context, string, vfs.VFSItem) error {
	return unsupportedError{}
}
func (v *k8sVFS) Create(context.Context, string) (io.WriteCloser, error) {
	return nil, unsupportedError{}
}

func (v *k8sVFS) GetCapabilities() vfs.VFSCapabilities {
	return vfs.VFSCapabilities{HasRandomAccess: true, HasUnixPermissions: true}
}

func (v *k8sVFS) Search(context.Context, string, string) (chan int64, error) { return nil, nil }

func (v *k8sVFS) ParentVFS() vfs.VFS { return nil }

// PanelTitle names the panel by where it is, "Kubernetes:default/web/app/etc".
func (v *k8sVFS) PanelTitle(p string) string {
	abs, err := v.Abs(p)
	if err != nil || abs == "/" {
		return "Kubernetes"
	}
	return "Kubernetes:" + strings.TrimPrefix(abs, "/")
}

// Clone opens its own connection when first used.
func (v *k8sVFS) Clone() vfs.VFS {
	clone := newK8sVFS(v.open)
	clone.cwd = v.GetPath()
	return clone
}

func (v *k8sVFS) Close() error {
	v.mu.Lock()
	cli := v.cli
	v.cli = nil
	v.closed = true
	v.mu.Unlock()
	cli.close()
	return nil
}

var (
	_ vfs.VFS                = (*k8sVFS)(nil)
	_ vfs.PanelTitleProvider = (*k8sVFS)(nil)
)
