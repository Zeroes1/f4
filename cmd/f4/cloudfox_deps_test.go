package main

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/testutil"
)

// TestCloudPluginBuildProfiles keeps the plugin built in to full builds
// while preserving the downloadable plugin boundary in lite builds.
func TestCloudPluginBuildProfiles(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not available")
	}

	forbidden := []string{
		"github.com/aws/aws-sdk-go-v2",
		"github.com/aws/smithy-go",
		"github.com/zalando/go-keyring",
		"golang.org/x/oauth2",
		"google.golang.org/api",
		"github.com/unxed/f4/plugins/cloudfox",
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
				if imported == "github.com/unxed/f4/plugins/cloudfox" {
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
