package cicheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type actionMetadata struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Inputs      map[string]struct {
		Description string `yaml:"description"`
	} `yaml:"inputs"`
	Outputs map[string]struct {
		Description string `yaml:"description"`
	} `yaml:"outputs"`
}

// TestActionMetadataHoldsNoExpressions keeps ${{ }} out of the prose of a
// composite action's metadata. The runner evaluates expressions there too,
// where no context exists, so the action fails to load in every job that
// uses it -- and actionlint passes it. An input's description of cached-deb
// once advised passing ${{ env.CACHE_WRITE }} in those very characters, and
// all six jobs that install qemu through it failed with "Unrecognized
// named-value: 'env'".
func TestActionMetadataHoldsNoExpressions(t *testing.T) {
	root := filepath.Join("..", "..")
	paths, err := filepath.Glob(filepath.Join(root, ".github", "actions", "*", "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no composite action found under .github/actions")
	}

	for _, path := range paths {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.ToSlash(rel)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var action actionMetadata
		if err := yaml.Unmarshal(data, &action); err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		prose := map[string]string{"name": action.Name, "description": action.Description}
		for input, spec := range action.Inputs {
			prose["inputs."+input+".description"] = spec.Description
		}
		for output, spec := range action.Outputs {
			prose["outputs."+output+".description"] = spec.Description
		}
		for field, text := range prose {
			if strings.Contains(text, "${{") {
				t.Errorf("%s: %s holds an expression, which the runner evaluates with no context and so fails to load the action: %q", name, field, text)
			}
		}
	}
}
