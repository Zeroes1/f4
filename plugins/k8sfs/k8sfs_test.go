package k8sfs

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"golang.org/x/net/websocket"
)

// fakeCluster is an API server with two namespaces, one pod of two containers
// and a tiny file system behind exec.
func fakeCluster(t *testing.T) *restClient {
	t.Helper()
	const token = "s3cret"
	mux := http.NewServeMux()
	authed := func(h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer "+token {
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": "forbidden: no token"})
				return
			}
			h(w, r)
		}
	}
	mux.HandleFunc("/api/v1/namespaces", authed(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[{"metadata":{"name":"default"}},{"metadata":{"name":"kube-system"}}]}`)
	}))
	mux.HandleFunc("/api/v1/namespaces/default/pods", authed(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"items":[{"metadata":{"name":"web","creationTimestamp":"2026-01-02T03:04:05Z"},`+
			`"spec":{"containers":[{"name":"app"},{"name":"sidecar"}]},"status":{"phase":"Running"}}]}`)
	}))
	mux.HandleFunc("/api/v1/namespaces/nowhere/pods", authed(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"message":"namespace not found"}`)
	}))
	commands := map[string]string{
		"ls -1ApL -- /":     "bin/\netc/\nhello.txt\nlnk/\n",
		"ls -1ApL -- /etc":  "hostname\n",
		"cat -- /hello.txt": "hello\n",
		"stat -c %F|%s|%Y|%a|%n -- /bin /etc /hello.txt /lnk": "directory|4096|1700000000|755|/bin\n" +
			"directory|4096|1700000000|755|/etc\nregular file|6|1700000000|644|/hello.txt\nsymbolic link|3|1700000000|777|/lnk\n",
		"stat -c %F|%s|%Y|%a|%n -- /etc/hostname": "regular file|4|1700000001|600|/etc/hostname\n",
		"stat -c %F|%s|%Y|%a|%n -- /hello.txt":    "regular file|6|1700000000|644|/hello.txt\n",
		"stat -c %F|%s|%Y|%a|%n -- /lnk":          "symbolic link|3|1700000000|777|/lnk\n",
		"test -d /lnk":                            "",
	}
	exec := websocket.Server{
		Handshake: func(c *websocket.Config, r *http.Request) error {
			c.Protocol = []string{execSubprotocol}
			return nil
		},
		Handler: func(ws *websocket.Conn) {
			req := ws.Request()
			cmd := strings.Join(req.URL.Query()["command"], " ")
			out, ok := commands[cmd]
			if !ok {
				_ = websocket.Message.Send(ws, append([]byte{2}, "no such file"...))
				_ = websocket.Message.Send(ws, append([]byte{3}, `{"status":"Failure","message":"command terminated with exit code 1"}`...))
				return
			}
			if out != "" {
				_ = websocket.Message.Send(ws, append([]byte{1}, out...))
			}
			_ = websocket.Message.Send(ws, append([]byte{3}, `{"status":"Success"}`...))
		},
	}
	mux.Handle("/api/v1/namespaces/default/pods/web/exec", authed(exec.ServeHTTP))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return newRESTClient(&apiEndpoint{server: srv.URL, token: token})
}

func listAll(t *testing.T, v *k8sVFS, p string) map[string]vfs.VFSItem {
	t.Helper()
	got := map[string]vfs.VFSItem{}
	if err := v.ReadDir(context.Background(), p, func(items []vfs.VFSItem) {
		for _, it := range items {
			got[it.Name] = it
		}
	}); err != nil {
		t.Fatalf("ReadDir(%q): %v", p, err)
	}
	return got
}

func TestK8sVFSBrowsesCluster(t *testing.T) {
	cli := fakeCluster(t)
	v := newK8sVFS(func() (*restClient, error) { return cli, nil })
	defer func() { _ = v.Close() }()

	if ns := listAll(t, v, "/"); len(ns) != 2 || !ns["default"].IsDir || !ns["kube-system"].IsDir {
		t.Fatalf("namespaces: %v", ns)
	}
	pods := listAll(t, v, "/default")
	if len(pods) != 1 || !pods["web"].IsDir || pods["web"].MTime.Year() != 2026 {
		t.Fatalf("pods: %+v", pods)
	}
	if cs := listAll(t, v, "/default/web"); len(cs) != 2 || !cs["app"].IsDir || !cs["sidecar"].IsDir {
		t.Fatalf("containers: %v", cs)
	}

	root := listAll(t, v, "/default/web/app")
	if len(root) != 4 || !root["bin"].IsDir || !root["etc"].IsDir || !root["lnk"].IsDir {
		t.Fatalf("container root: %v", root)
	}
	if hello := root["hello.txt"]; hello.IsDir || hello.Size != 6 || !hello.SizeKnown {
		t.Fatalf("hello.txt: %+v", hello)
	}
	if !root["lnk"].IsSymlink {
		t.Fatalf("lnk should be a symlink to a folder: %+v", root["lnk"])
	}
	etc := listAll(t, v, "/default/web/app/etc")
	if it := etc["hostname"]; it.Size != 4 || it.UnixMode != 0o600 {
		t.Fatalf("hostname: %+v", it)
	}
}

func TestK8sVFSStatOpenAndPaths(t *testing.T) {
	cli := fakeCluster(t)
	v := newK8sVFS(func() (*restClient, error) { return cli, nil })
	defer func() { _ = v.Close() }()
	ctx := context.Background()

	if err := v.SetPath("/default/web/app"); err != nil {
		t.Fatal(err)
	}
	if v.GetPath() != "/default/web/app" || v.IsAtRoot() {
		t.Fatalf("path %q", v.GetPath())
	}
	if it, err := v.Stat(ctx, "hello.txt"); err != nil || it.IsDir || it.Size != 6 {
		t.Fatalf("Stat(hello.txt) = %+v, %v", it, err)
	}
	if it, err := v.Stat(ctx, "lnk"); err != nil || !it.IsDir || !it.IsSymlink {
		t.Fatalf("Stat(lnk) = %+v, %v", it, err)
	}
	for _, p := range []string{"/", "/default", "/default/web", "/default/web/app", "/default/web/sidecar"} {
		if it, err := v.Stat(ctx, p); err != nil || !it.IsDir {
			t.Fatalf("Stat(%q) = %+v, %v", p, it, err)
		}
	}
	for _, p := range []string{"/nope", "/default/ghost", "/default/web/ghost", "/default/web/app/missing"} {
		if _, err := v.Stat(ctx, p); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Stat(%q): %v", p, err)
		}
	}
	if err := v.SetPath("/default/web/app/hello.txt"); !errors.Is(err, errNotADirectory) {
		t.Fatalf("SetPath on a file: %v", err)
	}

	f, err := v.Open(ctx, "/default/web/app/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	data := make([]byte, f.Size())
	if _, err := f.ReadAt(ctx, data, 0); err != nil && !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	_ = f.Close()
	if string(data) != "hello\n" {
		t.Fatalf("Open read %q", data)
	}
	if _, err := v.Open(ctx, "/default/web/app/lnk"); !errors.Is(err, errIsADirectory) {
		t.Fatalf("Open on a folder link: %v", err)
	}
	if _, err := v.Open(ctx, "/default/web"); !errors.Is(err, errIsADirectory) {
		t.Fatalf("Open on a pod: %v", err)
	}
	if _, err := v.Open(ctx, "/default/web/app/missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open on a missing file: %v", err)
	}
	if got := v.PanelTitle("/default/web/app/etc"); got != "Kubernetes:default/web/app/etc" || v.PanelTitle("/") != "Kubernetes" {
		t.Fatalf("PanelTitle = %q", got)
	}
	if v.Clone().(*k8sVFS).GetPath() != "/default/web/app" {
		t.Fatal("clone lost the path")
	}
}

func TestK8sVFSErrorsAndReadOnly(t *testing.T) {
	cli := fakeCluster(t)
	v := newK8sVFS(func() (*restClient, error) { return cli, nil })
	ctx := context.Background()

	if err := v.ReadDir(ctx, "/nowhere", func([]vfs.VFSItem) {}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a missing namespace: %v", err)
	}
	if err := v.ReadDir(ctx, "/default/web/app/gone", func([]vfs.VFSItem) {}); err == nil {
		t.Fatal("listing a missing folder should fail")
	}

	_, createErr := v.Create(ctx, "/x")
	for i, err := range []error{v.MkDir(ctx, "/x"), v.Remove(ctx, "/x"), v.Rename(ctx, "/x", "/y"), v.SetAttributes(ctx, "/x", vfs.VFSItem{}), createErr} {
		if !errors.Is(err, os.ErrPermission) || err.Error() == "" {
			t.Errorf("mutation %d: %v", i, err)
		}
	}

	forbidden := newRESTClient(&apiEndpoint{server: cli.ep.server, token: "wrong"})
	if _, err := forbidden.listNamespaces(ctx); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("a wrong token: %v", err)
	}
	dead := newRESTClient(&apiEndpoint{server: "http://127.0.0.1:1"})
	if _, err := dead.listNamespaces(ctx); err == nil || !strings.Contains(err.Error(), "Kubernetes") {
		t.Fatalf("an unreachable server: %v", err)
	}
	if err := dead.exec(ctx, "a", "b", "c", []string{"ls"}, io.Discard); err == nil {
		t.Fatal("exec on an unreachable server should fail")
	}

	broken := newK8sVFS(func() (*restClient, error) { return nil, errNoKubeconfig })
	if err := broken.ReadDir(ctx, "/", nil); !errors.Is(err, errNoKubeconfig) {
		t.Fatalf("no kubeconfig: %v", err)
	}
	_ = v.Close()
	if _, err := v.clientFor(); err == nil {
		t.Fatal("a closed panel should not reconnect")
	}
}

func TestExecReportsAFailingCommand(t *testing.T) {
	cli := fakeCluster(t)
	var out strings.Builder
	err := cli.exec(context.Background(), "default", "web", "app", []string{"cat", "--", "/nope"}, &out)
	if !errors.Is(err, errExecFailed) || !strings.Contains(err.Error(), "no such file") {
		t.Fatalf("a failing command: %v", err)
	}
}

const sampleKubeconfig = `
apiVersion: v1
current-context: dev
contexts:
- name: dev
  context: {cluster: c1, user: u1}
clusters:
- name: c1
  cluster: {server: "https://k8s.example:6443/", insecure-skip-tls-verify: true}
users:
- name: u1
  user: {token: abc}
`

func TestParseEndpoint(t *testing.T) {
	ep, err := parseEndpoint([]byte(sampleKubeconfig), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if ep.server != "https://k8s.example:6443" || ep.token != "abc" || !ep.tls.InsecureSkipVerify {
		t.Fatalf("endpoint: %+v", ep)
	}

	cases := map[string]string{
		"exec plugin": strings.Replace(sampleKubeconfig, "{token: abc}", "{exec: {command: gke-gcloud-auth-plugin}}", 1),
		"no context":  strings.Replace(sampleKubeconfig, "current-context: dev", "current-context: zz", 1),
		"no cluster":  strings.Replace(sampleKubeconfig, "cluster: c1,", "cluster: zz,", 1),
		"bad server":  strings.Replace(sampleKubeconfig, "https://k8s.example:6443/", "nohost", 1),
		"bad CA data": strings.Replace(sampleKubeconfig, "insecure-skip-tls-verify: true", "certificate-authority-data: '!!'", 1),
		"not yaml":    "{{{",
	}
	for name, cfg := range cases {
		if _, err := parseEndpoint([]byte(cfg), t.TempDir()); !errors.Is(err, errUnsupported) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestLoadEndpointAndPath(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config")
	if _, err := loadEndpoint(cfgPath); !errors.Is(err, errNoKubeconfig) {
		t.Fatalf("a missing file: %v", err)
	}
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("filetoken\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := strings.Replace(sampleKubeconfig, "{token: abc}", "{tokenFile: token}", 1)
	if err := os.WriteFile(cfgPath, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	ep, err := loadEndpoint(cfgPath)
	if err != nil || ep.token != "filetoken" {
		t.Fatalf("loadEndpoint: %+v, %v", ep, err)
	}

	t.Setenv("KUBECONFIG", cfgPath+string(os.PathListSeparator)+filepath.Join(dir, "other"))
	if p, err := kubeconfigPath(); err != nil || p != cfgPath {
		t.Fatalf("kubeconfigPath = %q, %v", p, err)
	}
	t.Setenv("KUBECONFIG", "")
	if p, err := kubeconfigPath(); err != nil || !strings.HasSuffix(p, filepath.Join(".kube", "config")) {
		t.Fatalf("default kubeconfigPath = %q, %v", p, err)
	}
	t.Setenv("KUBECONFIG", cfgPath)
	if cli, err := openFromKubeconfig(); err != nil || cli == nil {
		t.Fatalf("openFromKubeconfig: %v", err)
	}
	t.Setenv("KUBECONFIG", filepath.Join(dir, "missing"))
	if _, err := openFromKubeconfig(); !errors.Is(err, errNoKubeconfig) {
		t.Fatalf("openFromKubeconfig without a file: %v", err)
	}
}

func TestPluginAndHelpers(t *testing.T) {
	p := NewPlugin()
	if p.GetName() != "Kubernetes" || p.Close() != nil || p.Init(nil) == nil {
		t.Fatal("plugin identity")
	}
	loc := parseLocation("/default/web/app/etc/passwd")
	if loc.ns != "default" || loc.pod != "web" || loc.container != "app" || loc.inner != "/etc/passwd" || loc.depth != 3 {
		t.Fatalf("location %+v", loc)
	}
	if loc := parseLocation("/"); loc.depth != 0 || loc.inner != "/" {
		t.Fatalf("root location %+v", loc)
	}
	if _, ok := parseStatLine("garbage"); ok {
		t.Fatal("garbage is not a stat line")
	}
	if err := statusError([]byte(`{"status":"Success"}`), []string{"ls"}, ""); err != nil {
		t.Fatal(err)
	}
	if err := statusError([]byte(`not json`), []string{"ls"}, ""); err != nil {
		t.Fatal(err)
	}
}
