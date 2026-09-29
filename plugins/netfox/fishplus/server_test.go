package fishplus

import (
	"context"
	"io"
	"os"
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
	resp, err = sess.ExecPath(ctx, "info", "/nowhere")
	if err != nil || resp.OK() || !strings.Contains(resp.Msg, "unknown command") {
		t.Fatalf("info = %#v, %v, want an unknown command error", resp, err)
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
