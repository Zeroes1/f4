package update

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/unxed/f4/internal/netproxy"
)

// pressCtrlC stands in for the console's interrupt: RunCLI's interrupt
// context is canceled when the returned function is called.
func pressCtrlC(t *testing.T) context.CancelFunc {
	t.Helper()
	ctx, press := context.WithCancel(context.Background())
	old := interruptContext
	interruptContext = func() (context.Context, context.CancelFunc) { return ctx, press }
	t.Cleanup(func() {
		interruptContext = old
		press()
	})
	return press
}

// serveStalledNightly serves a nightly whose check or download (stallOn: the
// API path or "/archive") stops halfway and waits for the client to go away,
// signalling started once the client is waiting. The archive sends its first
// bytes, the API answer only its headers: a part of the JSON would fail the
// check before Ctrl+C is pressed.
func serveStalledNightly(t *testing.T, stallOn string, started chan<- struct{}) {
	t.Helper()
	oldAPI, oldOS, oldArch, oldProxy := APIURL, CurrentOS, CurrentArch, netproxy.Global()
	t.Cleanup(func() {
		APIURL, CurrentOS, CurrentArch = oldAPI, oldOS, oldArch
		netproxy.SetGlobal(oldProxy)
	})
	CurrentOS, CurrentArch = "linux", "amd64"
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeDirect})

	archiveURL := ""
	// Lets a stalled answer go when the test ends, for a client that never
	// went away.
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == stallOn {
			w.Header().Set("Content-Length", "1048576")
			w.WriteHeader(http.StatusOK)
			if stallOn == "/archive" {
				_, _ = w.Write(make([]byte, 4096))
			}
			w.(http.Flusher).Flush()
			close(started)
			select {
			case <-r.Context().Done():
			case <-release:
			}
			return
		}
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "nightly",
			Assets:  []Asset{{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: archiveURL, UpdatedAt: "2026-10-23T04:57:40Z"}},
		})
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(release) }) // before server.Close, which waits for the handler
	archiveURL = server.URL + "/archive"
	APIURL = server.URL
}

// #2172: Ctrl+C and Ctrl+Break did not stop `f4 --update` on Windows, which
// went on downloading. Interrupted while it checks or downloads, the update
// stops with nothing installed and nothing recorded.
func TestRunCLICtrlCStopsCheckAndDownload(t *testing.T) {
	for _, stallOn := range []string{"/tags/nightly", "/archive"} {
		t.Run(stallOn, func(t *testing.T) {
			exe := installFixture(t)
			press := pressCtrlC(t)
			started := make(chan struct{})
			serveStalledNightly(t, stallOn, started)
			go func() {
				<-started
				press()
			}()

			saves := 0
			result := make(chan int, 1)
			go func() { result <- RunCLI("", Settings{Channel: ChannelNightly}, Build{}, func(Settings) { saves++ }) }()
			select {
			case got := <-result:
				if got != exitInterrupted {
					t.Fatalf("RunCLI() = %d, want %d", got, exitInterrupted)
				}
			case <-time.After(20 * time.Second):
				t.Fatal("RunCLI went on after Ctrl+C")
			}
			if saves != 0 {
				t.Errorf("settings saved %d times, want none", saves)
			}
			if got, _ := os.ReadFile(exe); string(got) != "old f4" {
				t.Errorf("binary = %q, want it untouched", got)
			}
			if entries, _ := os.ReadDir(filepath.Dir(exe)); len(entries) != 1 {
				t.Errorf("install directory holds %d entries, want only f4: %v", len(entries), entries)
			}
		})
	}
}

// Once the archive is in, Ctrl+C no longer stops the update: an install
// killed halfway would leave f4 half replaced, with nobody to put the backup
// back.
func TestRunCLICtrlCLetsTheInstallFinish(t *testing.T) {
	exe := cliUpdateFixture(t, targz(t, map[string]string{"f4": "new f4"}))
	press := pressCtrlC(t)
	CheckInstalled = func() error {
		press()
		return nil
	}

	var saved Settings
	if got := RunCLI("", Settings{Channel: ChannelNightly}, Build{}, func(s Settings) { saved = s }); got != 0 {
		t.Fatalf("RunCLI() = %d, want 0", got)
	}
	if got, _ := os.ReadFile(exe); string(got) != "new f4" {
		t.Errorf("binary = %q, want the new one", got)
	}
	if saved.LastVersion != "2026-09-29T00:23:13Z" {
		t.Errorf("LastVersion = %q, want the installed asset's time", saved.LastVersion)
	}
}
