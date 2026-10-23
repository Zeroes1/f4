package update

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/unxed/f4/internal/netproxy"
)

// #2172: the nightly of 8c18f25, which `f4 -v` printed as
// "8c18f25 [2026-10-23 07:45]" in UTC+3. The asset times are stand-ins: the
// nightly has been republished since.
const (
	issue2172NightlyBody  = "Automated pre-release build of the latest main branch. Commit: 8c18f25. Built on: 2026-10-23 04:45:11 UTC, the time of that commit. Published: 2026-10-23 04:57 UTC."
	issue2172NightlyAsset = "2026-10-23T04:57:40Z"
	// The nightly the updater installed before the user unpacked 8c18f25.
	issue2172EarlierAsset = "2026-10-22T07:05:12Z"
)

var issue2172Build = Build{Version: "8c18f25", TimeText: "2026-10-23T04:45:11Z"}

func TestNightlyNeedsUpdateAsksTheRunningBuild(t *testing.T) {
	nightly := Release{TagName: "nightly", Body: issue2172NightlyBody}
	cases := []struct {
		name        string
		release     Release
		build       Build
		lastVersion string
		want        bool
	}{
		{
			// The user unpacked the nightly by hand; LastVersion still names
			// the one the updater installed before it.
			name:        "the nightly itself, unpacked by hand",
			release:     nightly,
			build:       issue2172Build,
			lastVersion: issue2172EarlierAsset,
		},
		{
			name:        "the nightly itself, installed by the updater",
			release:     nightly,
			build:       issue2172Build,
			lastVersion: issue2172NightlyAsset,
		},
		{
			name:        "notes with a longer hash of the same commit",
			release:     Release{TagName: "nightly", Body: "Commit: 8c18f25b4f. Built on: 2026-10-23 04:45:11 UTC"},
			build:       issue2172Build,
			lastVersion: issue2172EarlierAsset,
		},
		{
			// #1218: the install finished, the older binary still runs.
			name:        "an older build, the nightly recorded as installed",
			release:     nightly,
			build:       Build{Version: "f72e747", TimeText: "2026-10-22T06:43:20Z"},
			lastVersion: issue2172NightlyAsset,
			want:        true,
		},
		{
			name:        "a newer local build is not sent back",
			release:     nightly,
			build:       Build{Version: "430c0d1", TimeText: "2026-10-23T15:21:33Z"},
			lastVersion: issue2172EarlierAsset,
		},
		{
			name:        "a modified build of the same commit is not sent back",
			release:     nightly,
			build:       Build{Version: "8c18f25-dirty", TimeText: "2026-10-23T04:45:11Z"},
			lastVersion: issue2172EarlierAsset,
		},
		{
			// A release build reports its tag, not a commit.
			name:        "a release build, nightly not installed",
			release:     nightly,
			build:       Build{Version: "v0.3.0-beta", IsRelease: true, TimeText: "2026-09-26T22:41:49Z"},
			lastVersion: "v0.3.0-beta",
			want:        true,
		},
		{
			name:        "a release build, nightly recorded as installed",
			release:     nightly,
			build:       Build{Version: "v0.3.0-beta", IsRelease: true, TimeText: "2026-09-26T22:41:49Z"},
			lastVersion: issue2172NightlyAsset,
		},
		{
			name:        "notes from before #343",
			release:     Release{TagName: "nightly", Body: "Automated pre-release build of the latest main branch."},
			build:       issue2172Build,
			lastVersion: issue2172EarlierAsset,
			want:        true,
		},
		{
			name:        "a build without VCS data",
			release:     nightly,
			build:       Build{Version: "(devel)"},
			lastVersion: issue2172NightlyAsset,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nightlyNeedsUpdate(tc.release, tc.build, tc.lastVersion, issue2172NightlyAsset); got != tc.want {
				t.Fatalf("nightlyNeedsUpdate() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSameCommit(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"8c18f25", "8c18f25", true},
		{"8c18f25", "8c18f25b4feba5c2", true},
		{"8c18f25b4f", "8C18F25", true},
		{"8c18f25", "8c18f26", false},
		{"8c18f2", "8c18f2", false},
		{"8c18f25-dirty", "8c18f25", false},
		{"v0.3.0-beta", "8c18f25", false},
		{"", "", false},
	}
	for _, tc := range cases {
		if got := sameCommit(tc.a, tc.b); got != tc.want {
			t.Errorf("sameCommit(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

// #2172, end to end: `f4 --update` on the nightly it already runs answers
// "Already up to date" and downloads nothing.
func TestRunCLIDoesNotDownloadTheRunningNightly(t *testing.T) {
	oldAPI, oldOS, oldArch, oldProxy := APIURL, CurrentOS, CurrentArch, netproxy.Global()
	t.Cleanup(func() {
		APIURL, CurrentOS, CurrentArch = oldAPI, oldOS, oldArch
		netproxy.SetGlobal(oldProxy)
	})
	CurrentOS, CurrentArch = "linux", "amd64"
	netproxy.SetGlobal(netproxy.Settings{Mode: netproxy.ModeDirect})
	archiveURL := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/archive" {
			t.Error("the nightly that runs was downloaded again")
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(Release{
			TagName: "nightly",
			Body:    issue2172NightlyBody,
			Assets:  []Asset{{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: archiveURL, UpdatedAt: issue2172NightlyAsset}},
		})
	}))
	defer server.Close()
	archiveURL = server.URL + "/archive"
	APIURL = server.URL

	saves := 0
	cfg := Settings{Channel: ChannelNightly, LastVersion: issue2172EarlierAsset}
	if got := RunCLI("", cfg, issue2172Build, func(Settings) { saves++ }); got != 0 {
		t.Fatalf("RunCLI() = %d, want 0", got)
	}
	if saves != 0 {
		t.Errorf("settings saved %d times, want none", saves)
	}
}
