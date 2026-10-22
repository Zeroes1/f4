package cicheck

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// cacheWriteRule is the one condition under which a step may save a cache.
// build.yml defines it once, as CACHE_WRITE, and every saving step defers to
// that variable instead of repeating the condition.
const cacheWriteRule = "${{ github.event_name == 'push' && github.ref == 'refs/heads/main' }}"

type ciStep struct {
	Name string         `yaml:"name"`
	If   string         `yaml:"if"`
	Uses string         `yaml:"uses"`
	With map[string]any `yaml:"with"`
}

// ciFile is a workflow or a composite action: a workflow keeps its steps
// under jobs, a composite action under runs.
type ciFile struct {
	Env  map[string]string `yaml:"env"`
	Jobs map[string]struct {
		Steps []ciStep `yaml:"steps"`
	} `yaml:"jobs"`
	Runs struct {
		Steps []ciStep `yaml:"steps"`
	} `yaml:"runs"`
}

// TestOnlyAPushToMainSavesACache keeps every workflow from saving a cache
// anywhere but on a push to main. Each run saves into the scope of its own
// ref, so a pull request's entries serve only that pull request's reruns, yet
// they count against the repository's 10 GB just as main's do. Copies from
// pull requests, lunobot/staging and tags once pushed the repository over the
// limit, and the overflow evicted main's entries several times a day, every
// eviction a cold compile for the next run. A job added with a plain setup-go
// step -- its cache is on by default and saves on every miss -- would bring
// that back without a word, so each kind of step that can save is held to the
// rule here.
func TestOnlyAPushToMainSavesACache(t *testing.T) {
	root := filepath.Join("..", "..")
	workflows, err := filepath.Glob(filepath.Join(root, ".github", "workflows", "*.yml"))
	if err != nil {
		t.Fatal(err)
	}
	actions, err := filepath.Glob(filepath.Join(root, ".github", "actions", "*", "action.yml"))
	if err != nil {
		t.Fatal(err)
	}

	setupGoSteps := 0
	mainSaves := false
	for _, path := range append(workflows, actions...) {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.ToSlash(rel)

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var file ciFile
		if err := yaml.Unmarshal(data, &file); err != nil {
			t.Fatalf("%s: %v", name, err)
		}

		rule, defined := file.Env["CACHE_WRITE"]
		if defined && rule != cacheWriteRule {
			t.Errorf("%s: CACHE_WRITE is %q, want %q", name, rule, cacheWriteRule)
		}
		writes := defined && rule == cacheWriteRule
		if name == ".github/workflows/build.yml" {
			mainSaves = writes
		}

		for _, step := range fileSteps(name, file) {
			if strings.HasPrefix(step.Uses, "actions/setup-go@") {
				setupGoSteps++
			}
			checkCacheStep(t, step, writes)
		}
	}

	// A parse that silently stopped seeing steps would pass everything above.
	if setupGoSteps == 0 {
		t.Fatal("no setup-go step found under .github; the workflows were not read")
	}
	// The other half of the rule: main must still save, or every run is cold.
	if !mainSaves {
		t.Errorf(".github/workflows/build.yml does not define CACHE_WRITE as %q, so main saves no cache", cacheWriteRule)
	}
}

// placedStep is a step together with where it sits, for the error messages.
type placedStep struct {
	ciStep
	where string
}

// fileSteps lists a file's steps, jobs in name order so that failures read
// the same on every run.
func fileSteps(name string, file ciFile) []placedStep {
	jobs := make([]string, 0, len(file.Jobs))
	for job := range file.Jobs {
		jobs = append(jobs, job)
	}
	sort.Strings(jobs)

	var steps []placedStep
	for _, step := range file.Runs.Steps {
		steps = append(steps, placedStep{step, name + ": " + stepLabel(step)})
	}
	for _, job := range jobs {
		for _, step := range file.Jobs[job].Steps {
			steps = append(steps, placedStep{step, name + ": job " + job + ": " + stepLabel(step)})
		}
	}
	return steps
}

func stepLabel(step ciStep) string {
	if step.Name != "" {
		return step.Name
	}
	return step.Uses
}

func checkCacheStep(t *testing.T, step placedStep, writes bool) {
	t.Helper()
	action, _, _ := strings.Cut(step.Uses, "@")

	switch action {
	case "actions/setup-go":
		switch cache := withValue(step.ciStep, "cache"); {
		case cache == "false":
		case cache == "${{ env.CACHE_WRITE }}" && writes:
		default:
			t.Errorf("%s: setup-go's cache is %q; want false, or ${{ env.CACHE_WRITE }} in a workflow that defines CACHE_WRITE -- unset, it saves on every miss", step.where, cache)
		}
	case "actions/cache":
		t.Errorf("%s: actions/cache saves wherever it runs; restore with actions/cache/restore and save with actions/cache/save behind env.CACHE_WRITE", step.where)
	case "actions/cache/save":
		// The flag has to gate the whole condition: no `||` that could
		// save without it.
		gated := strings.HasPrefix(step.If, "env.CACHE_WRITE == 'true'") && !strings.Contains(step.If, "||")
		if !writes || !gated {
			t.Errorf("%s: a save must run only when env.CACHE_WRITE == 'true', in a workflow that defines it; its condition is %q", step.where, step.If)
		}
	case "golangci/golangci-lint-action":
		switch skip := withValue(step.ciStep, "skip-save-cache"); {
		case skip == "true":
		case skip == "${{ env.CACHE_WRITE != 'true' }}" && writes:
		default:
			t.Errorf("%s: skip-save-cache is %q; want true, or ${{ env.CACHE_WRITE != 'true' }} in a workflow that defines CACHE_WRITE", step.where, skip)
		}
	}
}

func withValue(step ciStep, key string) string {
	value, ok := step.With[key]
	if !ok {
		return ""
	}
	return fmt.Sprint(value)
}
