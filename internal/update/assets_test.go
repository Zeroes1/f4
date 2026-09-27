package update

import (
	"strings"
	"testing"
)

// The release publishes two Linux flavors under one GOOS/GOARCH pair:
// f4-linux-<arch>.tar.gz (static, no FFI) and f4-linux-musl-<arch>.tar.gz
// (musl, full FFI). Asset selection is the only thing keeping a musl install
// from quietly updating itself into the flavor it did not ask for, so the
// preference order and -- more importantly -- the directions the fallback
// must NOT go are pinned here.

func linuxAssets() []Asset {
	return []Asset{
		{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "https://example/generic-amd64"},
		{Name: "f4-linux-musl-amd64.tar.gz", BrowserDownloadURL: "https://example/musl-amd64"},
		{Name: "f4-linux-arm64.tar.gz", BrowserDownloadURL: "https://example/generic-arm64"},
		{Name: "f4-linux-musl-arm64.tar.gz", BrowserDownloadURL: "https://example/musl-arm64"},
		{Name: "f4-darwin-arm64.tar.gz", BrowserDownloadURL: "https://example/darwin"},
	}
}

func TestUpdateAssetSuffixes_PicksMatchingFlavor(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		goarch  string
		libc    string
		assets  []Asset
		wantURL string
	}{
		{
			name:    "musl build prefers the musl asset",
			goos:    "linux",
			goarch:  "amd64",
			libc:    "musl",
			assets:  linuxAssets(),
			wantURL: "https://example/musl-amd64",
		},
		{
			name:    "musl build on arm64 stays on arm64",
			goos:    "linux",
			goarch:  "arm64",
			libc:    "musl",
			assets:  linuxAssets(),
			wantURL: "https://example/musl-arm64",
		},
		{
			// The generic Linux artifacts are static and do start on a musl
			// system, so an older release without musl assets should still
			// update rather than refuse. FFI is lost; f4 keeps running.
			name:   "musl build falls back to the static asset",
			goos:   "linux",
			goarch: "amd64",
			libc:   "musl",
			assets: []Asset{
				{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "https://example/generic-amd64"},
			},
			wantURL: "https://example/generic-amd64",
		},
		{
			// The dangerous direction: a glibc build must never take the
			// musl artifact, which would not start at all on its system.
			name:    "glibc build ignores the musl asset",
			goos:    "linux",
			goarch:  "amd64",
			libc:    "",
			assets:  linuxAssets(),
			wantURL: "https://example/generic-amd64",
		},
		{
			name:   "glibc build refuses rather than take a musl-only release",
			goos:   "linux",
			goarch: "amd64",
			libc:   "",
			assets: []Asset{
				{Name: "f4-linux-musl-amd64.tar.gz", BrowserDownloadURL: "https://example/musl-amd64"},
			},
			wantURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, _, kind := pickAsset(tt.assets, assetSuffixes(tt.goos, tt.goarch, tt.libc))
			if url != tt.wantURL {
				t.Errorf("picked %q, want %q", url, tt.wantURL)
			}
			if tt.wantURL != "" && kind != "targz" {
				t.Errorf("archive kind = %q, want targz", kind)
			}
		})
	}
}

// A musl asset name ends with "-musl-<arch>.tar.gz", so it cannot match the
// generic "-linux-<arch>.tar.gz" suffix. That is what makes the glibc case
// above safe, and it would break silently if either name scheme changed, so
// assert the property directly rather than only through the table.
func TestUpdateAssetSuffixes_MuslNameDoesNotMatchGenericSuffix(t *testing.T) {
	for _, arch := range []string{"amd64", "arm64"} {
		muslName := "f4-linux-musl-" + arch + ".tar.gz"
		for _, suffix := range assetSuffixes("linux", arch, "") {
			if strings.HasSuffix(muslName, suffix) {
				t.Errorf("glibc suffix %q matches musl asset %q", suffix, muslName)
			}
		}
	}
}

func TestUpdateAssetSuffixes_NonLinuxUnchanged(t *testing.T) {
	// A libc value must not leak into platforms that have no such split.
	if got := assetSuffixes("darwin", "arm64", "musl"); len(got) != 1 || got[0] != "-darwin-arm64.tar.gz" {
		t.Errorf("darwin suffixes = %v, want [-darwin-arm64.tar.gz]", got)
	}
	got := assetSuffixes("windows", "amd64", "")
	want := []string{"-windows-amd64.7z", "-windows-amd64.zip"}
	if len(got) != len(want) {
		t.Fatalf("windows suffixes = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("windows suffixes = %v, want %v", got, want)
		}
	}
}

// The lite edition is published under the same GOOS/GOARCH as the regular
// one (f4-lite-linux-amd64.tar.gz next to f4-linux-amd64.tar.gz), and both
// names end with "-linux-amd64.tar.gz". Whichever the release listed first
// used to win, so either edition could update itself into the other.
func TestUpdateAssetSuffixes_EditionsDoNotCross(t *testing.T) {
	assets := []Asset{
		{Name: "f4-lite-linux-amd64.tar.gz", BrowserDownloadURL: "https://example/lite-amd64"},
		{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "https://example/generic-amd64"},
		{Name: "f4-lite-linux-arm.tar.gz", BrowserDownloadURL: "https://example/lite-arm"},
		{Name: "f4-linux-arm.tar.gz", BrowserDownloadURL: "https://example/generic-arm"},
	}
	tests := []struct {
		name    string
		lite    bool
		goarch  string
		assets  []Asset
		wantURL string
	}{
		{name: "regular build skips the lite asset listed first", goarch: "amd64", assets: assets, wantURL: "https://example/generic-amd64"},
		{name: "regular build on arm skips the lite asset", goarch: "arm", assets: assets, wantURL: "https://example/generic-arm"},
		{name: "lite build takes the lite asset", lite: true, goarch: "amd64", assets: assets, wantURL: "https://example/lite-amd64"},
		{name: "lite build on arm takes the lite asset", lite: true, goarch: "arm", assets: assets, wantURL: "https://example/lite-arm"},
		{
			name:   "regular build refuses a lite-only release",
			goarch: "amd64",
			assets: []Asset{{Name: "f4-lite-linux-amd64.tar.gz", BrowserDownloadURL: "https://example/lite-amd64"}},
		},
		{
			name:   "lite build refuses a release without a lite asset",
			lite:   true,
			goarch: "amd64",
			assets: []Asset{{Name: "f4-linux-amd64.tar.gz", BrowserDownloadURL: "https://example/generic-amd64"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url, _, kind := pickAsset(tt.assets, editionAssetSuffixes(tt.lite, "linux", tt.goarch, ""))
			if url != tt.wantURL {
				t.Errorf("picked %q, want %q", url, tt.wantURL)
			}
			if tt.wantURL != "" && kind != "targz" {
				t.Errorf("archive kind = %q, want targz", kind)
			}
		})
	}
}

// A lite build cannot unpack .7z (internal/unpack's formats_lite.go), so on
// Windows it looks for the .zip asset only.
func TestUpdateAssetSuffixes_LiteWindowsIsZipOnly(t *testing.T) {
	got := editionAssetSuffixes(true, "windows", "amd64", "")
	if len(got) != 1 || got[0] != "-lite-windows-amd64.zip" {
		t.Errorf("lite windows suffixes = %v, want [-lite-windows-amd64.zip]", got)
	}
}
