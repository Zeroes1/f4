package plughost

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// useCatalogURL points the catalog fetch at url for one test.
func useCatalogURL(t *testing.T, url string) {
	t.Helper()
	old := PlugRingCatalogURL
	PlugRingCatalogURL = url
	t.Cleanup(func() { PlugRingCatalogURL = old })
}

func serveCatalog(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write catalog response: %v", err)
		}
	}))
	t.Cleanup(ts.Close)
	return ts
}

const remoteCatalogYAML = `
- id: remote-lua
  name: Remote Lua
  version: "2.0"
  entrypoint: main.lua
  url: https://example.invalid/main.lua
`

func TestFetchCatalogPrefersTheLocalIndexForTheDefaultURL(t *testing.T) {
	dir := t.TempDir()
	// #nosec G703 -- dir is the test's temporary directory.
	if err := os.Mkdir(filepath.Join(dir, "plugring"), 0o750); err != nil {
		t.Fatal(err)
	}
	index := "- id: local-lua\n  name: Local Lua\n  version: \"1.0\"\n  entrypoint: main.lua\n"
	// #nosec G703 -- dir is the test's temporary directory.
	if err := os.WriteFile(filepath.Join(dir, "plugring", "index.yaml"), []byte(index), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	useCatalogURL(t, defaultPlugRingCatalogURL)

	items, err := FetchCatalog(context.Background())
	if err != nil {
		t.Fatalf("FetchCatalog: %v", err)
	}
	if len(items) != 1 || items[0].ID != "local-lua" {
		t.Fatalf("items = %+v, want the single local entry", items)
	}
	// The catalog is normalised on the way out, as for a downloaded one.
	if items[0].Category != PlugRingCategoryOther || len(items[0].Runtimes) != 1 || items[0].Runtimes[0] != PlugRingRuntimeEmbedded {
		t.Errorf("local entry was not normalised: %+v", items[0])
	}
}

func TestFetchCatalogReportsRequestAndNetworkFailures(t *testing.T) {
	useCatalogURL(t, "http://[::1")
	if items, err := FetchCatalog(context.Background()); err == nil || !strings.Contains(err.Error(), "failed to create request") || items != nil {
		t.Fatalf("an unparsable URL: items=%v err=%v", items, err)
	}

	ts := serveCatalog(t, http.StatusOK, remoteCatalogYAML)
	useCatalogURL(t, ts.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if items, err := FetchCatalog(ctx); err == nil || !strings.Contains(err.Error(), "network error while fetching catalog") || items != nil {
		t.Fatalf("a cancelled request: items=%v err=%v", items, err)
	}
}

func TestFetchPlugRingCatalogMergesTheFirstPartyPlugins(t *testing.T) {
	ts := serveCatalog(t, http.StatusOK, remoteCatalogYAML)
	useCatalogURL(t, ts.URL)

	items, err := FetchPlugRingCatalog(context.Background())
	if err != nil {
		t.Fatalf("FetchPlugRingCatalog: %v", err)
	}
	ids := make(map[string]bool, len(items))
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids["remote-lua"] {
		t.Errorf("the community entry is missing from %v", ids)
	}
	for _, own := range FirstPartyPlugRingItems() {
		if !ids[own.ID] {
			t.Errorf("first-party plugin %q is missing from the merged catalog %v", own.ID, ids)
		}
	}
}

func TestFetchPlugRingCatalogPassesTheFetchErrorOn(t *testing.T) {
	ts := serveCatalog(t, http.StatusServiceUnavailable, "")
	useCatalogURL(t, ts.URL)

	items, err := FetchPlugRingCatalog(context.Background())
	if err == nil || err.Error() != "server returned status 503" {
		t.Fatalf("error = %v, want the server status", err)
	}
	if items != nil {
		t.Fatalf("a failed fetch still returned %d items; first-party entries must not mask the outage", len(items))
	}
}

func TestResolveAssetURLFillsPlatformPlaceholders(t *testing.T) {
	got := ResolveAssetURL("https://host/{os}-{arch}/plugin_{os}_{arch}.zip")
	if strings.Contains(got, "{") || strings.Contains(got, "}") {
		t.Fatalf("ResolveAssetURL left a placeholder behind: %q", got)
	}
	if plain := "https://host/plugin.zip"; ResolveAssetURL(plain) != plain {
		t.Errorf("a URL without placeholders was changed")
	}
}
