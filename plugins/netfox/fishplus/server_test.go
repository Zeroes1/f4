package fishplus

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// serverSession connects a client Session to a Server through two pipes, the
// way a client reaches an f4 started as the remote command.
func serverSession(t *testing.T, srv *Server) (*Session, <-chan error) {
	t.Helper()
	cr, sw := io.Pipe() // server -> client
	sr, cw := io.Pipe() // client -> server
	done := make(chan error, 1)
	go func() {
		err := srv.Serve(sr, sw)
		_ = sw.Close()
		done <- err
	}()
	sess := NewSession(cw, cr, closerFunc(func() error { _ = cw.Close(); return cr.Close() }))
	t.Cleanup(func() { _ = sess.Close() })
	return sess, done
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func TestServerNativeSession(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	sess, done := serverSession(t, &Server{Dir: dir})

	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapNative}); err != nil {
		t.Fatalf("handshake: %v", err)
	}
	if !sess.Features().Has("native") || sess.Features().Proto != ProtocolVersion {
		t.Fatalf("features = %#v, want the native marker at protocol %d", sess.Features(), ProtocolVersion)
	}
	if err := sess.Noop(ctx); err != nil {
		t.Fatalf("noop: %v", err)
	}
	resp, err := sess.Exec(ctx, "pwd")
	if err != nil || !resp.OK() || strings.Join(resp.Lines, "\n") != dir {
		t.Fatalf("pwd = %#v, %v, want %q", resp, err, dir)
	}
	// A payload the line protocol has to escape survives the round trip.
	for _, payload := range []string{"plain", "with space", "~tilde", "two\nlines"} {
		got, err := sess.Ping(ctx, payload)
		if err != nil || got != payload {
			t.Fatalf("ping(%q) = %q, %v", payload, got, err)
		}
	}
	// A command the server does not implement is refused after its path lines
	// were read, and the session stays usable.
	resp, err = sess.ExecPath(ctx, "mkdir", "/nowhere")
	if err != nil || resp.OK() || !strings.Contains(resp.Msg, "unknown command") {
		t.Fatalf("mkdir = %#v, %v, want an unknown command error", resp, err)
	}
	if err := sess.Noop(ctx); err != nil {
		t.Fatalf("noop after a refused command: %v", err)
	}
	resp, err = sess.Exec(ctx, "exit")
	if err != nil || !resp.OK() {
		t.Fatalf("exit = %#v, %v", resp, err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Serve: %v", err)
	}
}

func TestServerDefaultsToTheWorkingDirectory(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sess, _ := serverSession(t, &Server{})
	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapNative}); err != nil {
		t.Fatal(err)
	}
	want, _ := os.Getwd()
	resp, err := sess.Exec(ctx, "pwd")
	if err != nil || strings.Join(resp.Lines, "\n") != want {
		t.Fatalf("pwd = %#v, %v, want %q", resp, err, want)
	}
}

func TestServerRejectsABadHelloAndAnUnfollowableRequest(t *testing.T) {
	var out strings.Builder
	if err := (&Server{}).Serve(strings.NewReader("hello\n"), &out); err == nil {
		t.Fatal("a hello without the native prefix must be refused")
	}
	out.Reset()
	in := NativeHelloLine("tok") + "1 write 0 4 raw\n/x\nDATA"
	if err := (&Server{}).Serve(strings.NewReader(in), &out); err == nil {
		t.Fatal("a command whose payload cannot be skipped must end the session")
	}
	if !strings.Contains(out.String(), ".tok 1 err unknown command") {
		t.Fatalf("the refusal was not sent: %q", out.String())
	}
}

func TestServerFileSystemCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a file.txt"), []byte("hello"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hidden"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	haveLinks := os.Symlink("sub", filepath.Join(root, "link")) == nil

	sess, _ := serverSession(t, &Server{Dir: root})
	if err := sess.HandshakeWithOptions(ctx, HandshakeOptions{Bootstrap: BootstrapNative}); err != nil {
		t.Fatal(err)
	}
	if sess.Features().ListingMode() != "find" {
		t.Fatalf("listing mode = %q, want find", sess.Features().ListingMode())
	}
	c := NewClient(sess)

	entries, err := c.Enum(ctx, root)
	if err != nil {
		t.Fatalf("enum: %v", err)
	}
	byName := map[string]Entry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	if f, ok := byName["a file.txt"]; !ok || !f.IsRegular() || f.Size != 5 || (runtime.GOOS != "windows" && f.Perm() != 0o640) {
		t.Errorf("a file.txt = %#v", f)
	}
	if d, ok := byName["sub"]; !ok || !d.IsDir() {
		t.Errorf("sub = %#v", d)
	}
	if _, ok := byName[".hidden"]; !ok {
		t.Error("hidden entries must be listed")
	}
	if haveLinks {
		if l, ok := byName["link"]; !ok || !l.IsSymlink() || !l.TargetIsDir {
			t.Errorf("link = %#v, want a symlink to a directory", l)
		}
		if target, err := c.ReadLink(ctx, filepath.Join(root, "link")); err != nil || target != "sub" {
			t.Errorf("ReadLink = %q, %v", target, err)
		}
		if e, err := c.Stat(ctx, filepath.Join(root, "link")); err != nil || !e.IsDir() {
			t.Errorf("Stat through a link = %#v, %v, want a directory", e, err)
		}
		if e, err := c.Lstat(ctx, filepath.Join(root, "link")); err != nil || !e.IsSymlink() {
			t.Errorf("Lstat of a link = %#v, %v, want the link", e, err)
		}
	}
	if e, err := c.Stat(ctx, filepath.Join(root, "a file.txt")); err != nil || e.Size != 5 || e.Name != "a file.txt" {
		t.Errorf("Stat = %#v, %v", e, err)
	}
	if _, err := c.Stat(ctx, filepath.Join(root, "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Stat of a missing path = %v, want os.ErrNotExist", err)
	}
	if _, err := c.Stat(ctx, "relative"); err == nil {
		t.Error("a relative path must be refused")
	}
	big := make([]byte, 700*1024+13) // more than two chunks
	for i := range big {
		big[i] = byte(i * 7)
	}
	bigPath := filepath.Join(root, "big.bin")
	if err := os.WriteFile(bigPath, big, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := c.ReadFile(ctx, bigPath); err != nil || !bytes.Equal(got, big) {
		t.Errorf("ReadFile = %d bytes, %v, want the %d bytes of the file", len(got), err, len(big))
	}
	if part, size, err := c.Read(ctx, bigPath, 100, 50); err != nil || size != int64(len(big)) || !bytes.Equal(part, big[100:150]) {
		t.Errorf("Read(100, 50) = %d bytes, size %d, %v", len(part), size, err)
	}
	if part, _, err := c.Read(ctx, bigPath, int64(len(big))+10, 5); err != nil || len(part) != 0 {
		t.Errorf("Read past the end = %d bytes, %v, want none", len(part), err)
	}
	if _, _, err := c.Read(ctx, root, 0, 10); err == nil {
		t.Error("reading a directory must fail")
	}
	dirs, err := c.TargetDirs(ctx, []string{root, filepath.Join(root, "a file.txt"), filepath.Join(root, "missing")})
	if err != nil || len(dirs) != 3 || !dirs[0] || dirs[1] || dirs[2] {
		t.Errorf("TargetDirs = %v, %v, want [true false false]", dirs, err)
	}
}
