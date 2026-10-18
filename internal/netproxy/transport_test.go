package netproxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Clients built from the same Settings share one Transport, so their
// connections are reused; different Settings keep their own.
func TestHTTPClientsShareTheTransportOfTheirSettings(t *testing.T) {
	direct := Settings{Mode: ModeDirect}
	first := direct.HTTPClient(time.Second).Transport
	second := direct.HTTPClient(5 * time.Second).Transport
	if first != second {
		t.Error("two clients of the same Settings got different Transports")
	}
	other := (Settings{Mode: ModeHTTP, Host: "proxy.invalid", Port: "3128"}).HTTPClient(time.Second).Transport
	if other == first {
		t.Error("a client of different Settings shares the Transport of another")
	}
}

// readLoopGoroutines counts the goroutines reading a kept-alive connection.
func readLoopGoroutines() int {
	buf := make([]byte, 1<<20)
	n := runtime.Stack(buf, true)
	return strings.Count(string(buf[:n]), "net/http.(*persistConn).readLoop")
}

// A request leaves its connection open for reuse, with a goroutine reading
// it; CloseIdleConnections ends that, which is what a test package does
// before it checks for leaked goroutines.
func TestCloseIdleConnectionsEndsTheReadLoops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	before := readLoopGoroutines()
	resp, err := (Settings{Mode: ModeDirect}).HTTPClient(5 * time.Second).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if readLoopGoroutines() <= before {
		t.Fatal("the request left no kept-alive connection to close")
	}

	CloseIdleConnections()

	deadline := time.Now().Add(2 * time.Second)
	for readLoopGoroutines() > before {
		if !time.Now().Before(deadline) {
			t.Fatal("the kept-alive connection's goroutine outlived CloseIdleConnections")
		}
		time.Sleep(time.Millisecond)
	}
}
