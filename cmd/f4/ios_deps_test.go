package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
)

// TestIOSPluginBuildProfiles keeps the plugin built in to full builds
// while preserving the downloadable plugin boundary in lite builds.
func TestIOSPluginBuildProfiles(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/danielpaulus/go-ios",
		"github.com/Masterminds/semver",
		"gvisor.dev/gvisor",
		"github.com/quic-go/quic-go",
		"github.com/vishvananda/netlink",
		"github.com/vishvananda/netns",
		"github.com/songgao/water",
		"github.com/miekg/dns",
		"github.com/grandcat/zeroconf",
		"howett.net/plist",
		"go.mozilla.org/pkcs7",
		"software.sslmate.com/src/go-pkcs12",
		"golang.zx2c4.com/wintun",
		"github.com/unxed/f4/plugins/ios",
	}

	for _, profile := range []struct {
		name     string
		tags     string
		included bool
	}{
		{name: "full", included: true},
		{name: "lite", tags: "lite,vtui_noebiten,vtui_nogogpu"},
	} {
		t.Run(profile.name, func(t *testing.T) {
			command := exec.Command("go", "list", "-deps", "./cmd/f4")
			if profile.tags != "" {
				command = exec.Command("go", "list", "-deps", "-tags", "lite,vtui_noebiten,vtui_nogogpu", "./cmd/f4")
			}
			command.Dir = testutil.ModuleRootDir(t)
			out, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("go list -deps (%s): %v\n%s", profile.name, err, out)
			}
			deps := strings.Fields(string(out))
			pluginFound := false
			for _, imported := range deps {
				if imported == "github.com/unxed/f4/plugins/ios" {
					pluginFound = true
				}
				if profile.included {
					continue
				}
				for _, bad := range forbidden {
					if imported == bad || strings.HasPrefix(imported, bad+"/") {
						t.Errorf("lite build unexpectedly imports %s", imported)
					}
				}
			}
			if pluginFound != profile.included {
				t.Errorf("%s plugin included = %v, want %v", profile.name, pluginFound, profile.included)
			}
		})
	}
}
