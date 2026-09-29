package dockerfs

import (
	"archive/tar"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

// fakeNode is one path of the fake container file system.
type fakeNode struct {
	mode os.FileMode
	data string
	link string
}

// fakeDocker is the state behind fakeDaemon.
type fakeDocker struct {
	mu    sync.Mutex
	fsys  map[string]fakeNode
	execs [][]string
	// failExec makes every exec exit with status 1.
	failExec bool
}

func (f *fakeDocker) node(p string) (fakeNode, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	n, ok := f.fsys[p]
	return n, ok
}

func (f *fakeDocker) setFail(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failExec = v
}

func (f *fakeDocker) ran() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][]string(nil), f.execs...)
}

// fakeDaemon answers the Engine API calls the panel makes, over TCP.
func fakeDaemon(t *testing.T, fsys map[string]fakeNode) (*client, *fakeDocker) {
	t.Helper()
	state := &fakeDocker{fsys: fsys}
	mux := http.NewServeMux()
	mux.HandleFunc("/containers/aaaaaaaaaaaaaaaa/exec", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Cmd []string }
		_ = json.NewDecoder(r.Body).Decode(&req)
		state.mu.Lock()
		state.execs = append(state.execs, req.Cmd)
		state.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]string{"Id": "exec1"})
	})
	mux.HandleFunc("/exec/exec1/start", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/exec/exec1/json", func(w http.ResponseWriter, r *http.Request) {
		code := 0
		state.mu.Lock()
		if state.failExec {
			code = 1
		}
		state.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"Running": false, "ExitCode": code})
	})
	mux.HandleFunc("/containers/json", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]containerInfo{
			{ID: "aaaaaaaaaaaaaaaa", Names: []string{"/web"}, Created: 1700000000},
			{ID: "bbbbbbbbbbbbbbbb", Created: 1700000001},
		})
	})
	mux.HandleFunc("/containers/aaaaaaaaaaaaaaaa/archive", func(w http.ResponseWriter, r *http.Request) {
		p := path.Clean(r.URL.Query().Get("path"))
		if r.Method == http.MethodPut {
			tr := tar.NewReader(r.Body)
			for {
				hdr, err := tr.Next()
				if err != nil {
					break
				}
				data, _ := io.ReadAll(tr)
				node := fakeNode{mode: 0o644, data: string(data)}
				if hdr.Typeflag == tar.TypeDir {
					node = fakeNode{mode: os.ModeDir | 0o755}
				}
				state.mu.Lock()
				state.fsys[path.Join(p, strings.TrimSuffix(hdr.Name, "/"))] = node
				state.mu.Unlock()
			}
			return
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		node, ok := fsys[p]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "no such file: " + p})
			return
		}
		mode := node.mode
		st := pathStat{Name: path.Base(p), Size: int64(len(node.data)), Mode: uint32(mode), LinkTarget: node.link, MTime: time.Unix(1700000000, 0).UTC()}
		raw, _ := json.Marshal(st)
		w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
		if r.Method == http.MethodHead {
			return
		}
		tw := tar.NewWriter(w)
		defer func() { _ = tw.Close() }()
		base := path.Base(p)
		if p == "/" {
			base = ""
		}
		write := func(name string, n fakeNode) {
			hdr := &tar.Header{Name: name, Mode: int64(n.mode.Perm()), ModTime: time.Unix(1700000000, 0)}
			switch {
			case n.mode.IsDir():
				hdr.Typeflag = tar.TypeDir
				hdr.Name += "/"
			case n.mode&os.ModeSymlink != 0:
				hdr.Typeflag = tar.TypeSymlink
				hdr.Linkname = n.link
			default:
				hdr.Typeflag = tar.TypeReg
				hdr.Size = int64(len(n.data))
			}
			_ = tw.WriteHeader(hdr)
			if hdr.Typeflag == tar.TypeReg {
				_, _ = tw.Write([]byte(n.data))
			}
		}
		write(base, node)
		if !node.mode.IsDir() {
			return
		}
		var names []string
		for name := range fsys {
			if name != p && strings.HasPrefix(name, strings.TrimSuffix(p, "/")+"/") {
				names = append(names, name)
			}
		}
		sort.Strings(names)
		for _, name := range names {
			rel := strings.TrimPrefix(name, strings.TrimSuffix(p, "/")+"/")
			write(path.Join(base, rel), fsys[name])
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	cli, err := newClient("tcp://" + strings.TrimPrefix(srv.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	return cli, state
}

func testFS() map[string]fakeNode {
	return map[string]fakeNode{
		"/":             {mode: os.ModeDir | 0o755},
		"/etc":          {mode: os.ModeDir | 0o755},
		"/etc/hostname": {mode: 0o644, data: "web\n"},
		"/usr":          {mode: os.ModeDir | 0o755},
		"/usr/bin":      {mode: os.ModeDir | 0o755},
		"/usr/bin/ls":   {mode: 0o755, data: "ELF"},
		"/bin":          {mode: os.ModeSymlink | 0o777, link: "usr/bin"},
	}
}

func listNames(t *testing.T, v *dockerVFS, p string) map[string]vfs.VFSItem {
	t.Helper()
	got := map[string]vfs.VFSItem{}
	err := v.ReadDir(context.Background(), p, func(items []vfs.VFSItem) {
		for _, it := range items {
			got[it.Name] = it
		}
	})
	if err != nil {
		t.Fatalf("ReadDir(%q): %v", p, err)
	}
	return got
}

func TestDockerVFSBrowsesContainers(t *testing.T) {
	cli, _ := fakeDaemon(t, testFS())
	v := newDockerVFS(func() (*client, error) { return cli, nil })
	defer func() { _ = v.Close() }()

	root := listNames(t, v, "/")
	if len(root) != 2 || !root["web"].IsDir || !root["bbbbbbbbbbbb"].IsDir {
		t.Fatalf("containers listed as %v", root)
	}

	top := listNames(t, v, "/web")
	if !top["etc"].IsDir || !top["usr"].IsDir {
		t.Fatalf("container root listed as %v", top)
	}
	if !top["bin"].IsSymlink || !top["bin"].IsDir {
		t.Fatalf("a symlink to a folder should be a folder link, got %+v", top["bin"])
	}

	etc := listNames(t, v, "/web/etc")
	if it := etc["hostname"]; it.IsDir || it.Size != 4 || !it.SizeKnown {
		t.Fatalf("hostname listed as %+v", it)
	}
	if len(etc) != 1 {
		t.Fatalf("/web/etc has %d entries, want 1: %v", len(etc), etc)
	}

	viaLink := listNames(t, v, "/web/bin")
	if !viaLink["ls"].IsExecutable {
		t.Fatalf("listing through a symlink: %v", viaLink)
	}
}

func TestDockerVFSStatSetPathAndOpen(t *testing.T) {
	cli, _ := fakeDaemon(t, testFS())
	v := newDockerVFS(func() (*client, error) { return cli, nil })
	defer func() { _ = v.Close() }()
	ctx := context.Background()

	if err := v.SetPath("/web/etc"); err != nil {
		t.Fatal(err)
	}
	if v.GetPath() != "/web/etc" || v.IsAtRoot() {
		t.Fatalf("path is %q", v.GetPath())
	}
	if it, err := v.Stat(ctx, "hostname"); err != nil || it.IsDir || it.Size != 4 {
		t.Fatalf("Stat(hostname) = %+v, %v", it, err)
	}
	if it, err := v.Stat(ctx, "/web"); err != nil || !it.IsDir {
		t.Fatalf("Stat(/web) = %+v, %v", it, err)
	}
	if it, err := v.Stat(ctx, "/"); err != nil || !it.IsDir {
		t.Fatalf("Stat(/) = %+v, %v", it, err)
	}
	if err := v.SetPath("/web/etc/hostname"); !errors.Is(err, errNotADirectory) {
		t.Fatalf("SetPath on a file: %v", err)
	}
	if err := v.SetPath("/nope"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("SetPath on a missing container: %v", err)
	}
	if _, err := v.Stat(ctx, "/web/missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Stat on a missing path: %v", err)
	}

	f, err := v.Open(ctx, "/web/etc/hostname")
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(io.NewSectionReader(readerAt{f}, 0, f.Size()))
	_ = f.Close()
	if string(data) != "web\n" {
		t.Fatalf("Open read %q", data)
	}
	if _, err := v.Open(ctx, "/web/etc"); !errors.Is(err, errIsADirectory) {
		t.Fatalf("Open on a folder: %v", err)
	}
	if _, err := v.Open(ctx, "/web"); !errors.Is(err, errIsADirectory) {
		t.Fatalf("Open on a container: %v", err)
	}
	if got := v.PanelTitle("/web/etc"); got != "Docker:web/etc" {
		t.Fatalf("PanelTitle = %q", got)
	}
	if got := v.PanelTitle("/"); got != "Docker" {
		t.Fatalf("PanelTitle(/) = %q", got)
	}
	if clone := v.Clone().(*dockerVFS); clone.GetPath() != "/web/etc" {
		t.Fatalf("clone path %q", clone.GetPath())
	}
}

type readerAt struct{ f vfs.ReadAtCloser }

func (r readerAt) ReadAt(p []byte, off int64) (int, error) {
	return r.f.ReadAt(context.Background(), p, off)
}

func TestDockerVFSWrites(t *testing.T) {
	cli, state := fakeDaemon(t, testFS())
	v := newDockerVFS(func() (*client, error) { return cli, nil })
	defer func() { _ = v.Close() }()
	ctx := context.Background()

	w, err := v.Create(ctx, "/web/etc/motd")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("a second Close: %v", err)
	}
	if n, ok := state.node("/etc/motd"); !ok || n.data != "hello" {
		t.Fatalf("uploaded file: %+v %v", n, ok)
	}

	// Through a symlinked folder.
	w, err = v.Create(ctx, "/web/bin/tool")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("x"))
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok := state.node("/usr/bin/tool"); !ok {
		t.Fatal("a file created through /bin should land in /usr/bin")
	}

	if err := v.MkDir(ctx, "/web/etc/conf.d"); err != nil {
		t.Fatal(err)
	}
	if n, ok := state.node("/etc/conf.d"); !ok || !n.mode.IsDir() {
		t.Fatalf("mkdir: %+v %v", n, ok)
	}

	if err := v.Remove(ctx, "/web/etc/motd"); err != nil {
		t.Fatal(err)
	}
	if err := v.Rename(ctx, "/web/etc/hostname", "/web/etc/host"); err != nil {
		t.Fatal(err)
	}
	ran := state.ran()
	if len(ran) != 2 || strings.Join(ran[0], " ") != "rm -rf -- /etc/motd" || strings.Join(ran[1], " ") != "mv -- /etc/hostname /etc/host" {
		t.Fatalf("commands run in the container: %v", ran)
	}

	state.setFail(true)
	if err := v.Remove(ctx, "/web/etc/hostname"); !errors.Is(err, errExecFailed) {
		t.Fatalf("a failing rm: %v", err)
	}
	state.setFail(false)

	if _, err := v.Create(ctx, "/web/etc/hostname/x"); !errors.Is(err, errNotADirectory) {
		t.Fatalf("Create under a file: %v", err)
	}
}

func TestDockerVFSRefusesWhatItCannotDo(t *testing.T) {
	cli, _ := fakeDaemon(t, testFS())
	v := newDockerVFS(func() (*client, error) { return cli, nil })
	defer func() { _ = v.Close() }()
	ctx := context.Background()
	_, createErr := v.Create(ctx, "/web")
	errs := []error{
		v.MkDir(ctx, "/"), v.MkDir(ctx, "/web"), v.Remove(ctx, "/web"), v.Remove(ctx, "/"),
		v.Rename(ctx, "/web/etc", "/other/etc"), v.Rename(ctx, "/web/etc", "/web"),
		v.SetAttributes(ctx, "/web/etc", vfs.VFSItem{}), createErr,
	}
	for i, err := range errs {
		if !errors.Is(err, os.ErrPermission) || err.Error() == "" {
			t.Errorf("operation %d: %v", i, err)
		}
	}
}

func TestDockerVFSReportsAnUnreachableDaemon(t *testing.T) {
	cli, err := newClient("tcp://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	v := newDockerVFS(func() (*client, error) { return cli, nil })
	err = v.ReadDir(context.Background(), "/", func([]vfs.VFSItem) {})
	if err == nil || !strings.Contains(err.Error(), "Docker") {
		t.Fatalf("an unreachable daemon should give a Docker-specific error, got %v", err)
	}
	failing := newDockerVFS(func() (*client, error) { return nil, errUnsupportedHost })
	if err := failing.ReadDir(context.Background(), "/", nil); !errors.Is(err, errUnsupportedHost) {
		t.Fatalf("a client that cannot be made: %v", err)
	}
	_ = v.Close()
	if _, err := v.clientFor(); err == nil {
		t.Fatal("a closed panel should not reconnect")
	}
}

func TestNewClientHosts(t *testing.T) {
	for _, host := range []string{"unix:///var/run/docker.sock", "tcp://127.0.0.1:2375", "http://127.0.0.1:2375"} {
		if cli, err := newClient(host); err != nil {
			t.Errorf("newClient(%q): %v", host, err)
		} else {
			cli.close()
		}
	}
	for _, host := range []string{"npipe:////./pipe/docker_engine", "ssh://user@host", "unix://", "tcp://", "://bad"} {
		if _, err := newClient(host); !errors.Is(err, errUnsupportedHost) {
			t.Errorf("newClient(%q) = %v, want errUnsupportedHost", host, err)
		}
	}
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:2375")
	if _, err := clientFromEnv(); err != nil {
		t.Fatal(err)
	}
}

func TestPluginRegistersADrive(t *testing.T) {
	if err := NewPlugin().Init(nil); err == nil {
		t.Fatal("Init(nil) should fail")
	}
	p := NewPlugin()
	if p.GetName() != "Docker" || p.Close() != nil {
		t.Fatal("plugin identity")
	}
}

func TestPathHelpers(t *testing.T) {
	if name, inner := split("/web/etc/passwd"); name != "web" || inner != "/etc/passwd" {
		t.Errorf("split = %q %q", name, inner)
	}
	if name, inner := split("/web"); name != "web" || inner != "/" {
		t.Errorf("split(/web) = %q %q", name, inner)
	}
	if name, inner := split("/"); name != "" || inner != "/" {
		t.Errorf("split(/) = %q %q", name, inner)
	}
	for in, want := range map[string]string{"": "", "/": "", "./": "", "dir/": "dir", "./dir/x": "dir/x", "/dir": "dir"} {
		if got := strings.Join(tarSegments(in), "/"); got != want {
			t.Errorf("tarSegments(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := parsePathStat(""); err == nil {
		t.Error("an empty stat header should be an error")
	}
	if _, err := parsePathStat("!!"); err == nil {
		t.Error("a bad stat header should be an error")
	}
	if _, err := parsePathStat(base64.StdEncoding.EncodeToString([]byte("nope"))); err == nil {
		t.Error("a non-JSON stat header should be an error")
	}
}
