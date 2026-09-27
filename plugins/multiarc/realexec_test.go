package multiarc

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
)

// The *_realexec_test.go files in this package run the actual archivers
// against real archives, with runTool/lookupTool left as production has
// them. Each one skips when the tool it needs is not on PATH, which is what
// lets the same files run on every CI cell: a Linux runner has GNU tar, zip
// and unzip, a macOS runner has bsdtar, and a Windows runner has bsdtar as
// tar.exe and usually nothing else. The helpers below are shared by them.

// requireRealTool skips the test unless every one of names is on PATH.
func requireRealTool(t *testing.T, names ...string) {
	t.Helper()
	for _, name := range names {
		if _, err := exec.LookPath(name); err != nil {
			t.Skipf("%s is not on PATH", name)
		}
	}
}

// writeTree creates files under root. A key ending in "/" makes an empty
// directory; any other key is a file holding its value.
func writeTree(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		full := filepath.Join(root, filepath.FromSlash(strings.TrimSuffix(name, "/")))
		if strings.HasSuffix(name, "/") {
			if err := os.MkdirAll(full, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", full, err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

// runReal runs a tool in dir to build a test fixture, failing the test if
// the tool fails. It execs directly rather than through runTool, so a
// fixture never depends on the code it is there to test.
func runReal(t *testing.T, dir, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...) // #nosec G204 -- test fixture: a known archiver and paths under t.TempDir.
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v (%s)", name, args, err, out)
	}
}

// openReal opens the archive at localPath the way the provider would: by
// its name, through whichever backend detectFormat picks from the real PATH.
func openReal(t *testing.T, localPath string) *MultiArcVFS {
	t.Helper()
	b, id, ok := detectFormat(filepath.Base(localPath))
	if !ok {
		t.Fatalf("detectFormat(%s): no backend", localPath)
	}
	return NewMultiArcVFS(vfs.NewOSVFS(filepath.Dir(localPath)), localPath, filepath.Base(localPath), b, id)
}

// readMember reads one member back through MultiArcVFS.Open.
func readMember(t *testing.T, v *MultiArcVFS, p string) string {
	t.Helper()
	f, err := v.Open(context.Background(), p)
	if err != nil {
		t.Fatalf("Open %s: %v", p, err)
	}
	defer func() { _ = f.Close() }()
	buf := make([]byte, f.Size())
	n, err := f.ReadAt(context.Background(), buf, 0)
	if err != nil && n != len(buf) {
		t.Fatalf("ReadAt %s: %v", p, err)
	}
	return string(buf[:n])
}
