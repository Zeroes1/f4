package cicheck

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// stubGH answers `gh api <path> --jq <filter>` from <path>.json in STUB_DIR,
// run through the real jq, and fails like an API error when there is no such
// file, so the step under test meets the same filters it meets on a runner.
const stubGH = `#!/usr/bin/env bash
set -euo pipefail
[ "$1" = api ] || exit 2
path=$2
filter=.
if [ "${3:-}" = --jq ]; then filter=$4; fi
path=${path%%\?*}
file="$STUB_DIR/$(printf '%s' "$path" | tr '/' '_').json"
[ -f "$file" ] || { echo "gh: Not Found (HTTP 404)" >&2; exit 1; }
jq -r "$filter" "$file"
`

// verifiedStep returns the run: script of should-run-full's step that
// decides pr-verified.
func verifiedStep(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "build.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var wf struct {
		Jobs map[string]struct {
			Steps []struct {
				ID  string `yaml:"id"`
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(data, &wf); err != nil {
		t.Fatal(err)
	}
	for _, step := range wf.Jobs["should-run-full"].Steps {
		if step.ID == "verified" {
			return step.Run
		}
	}
	t.Fatal("build.yml: should-run-full has no step with id verified")
	return ""
}

// TestPushSkipsOnlyWhatAPullRequestRunPassed runs the step that lets a push to
// main skip the tests a pull request run already passed on the same tree. A
// wrong "true" would skip tests nobody ran, so every way of not finding such a
// run must come out false: no artifact, an expired one, a run that failed or is
// still going, a push run, another workflow, an API error, an unknown tree.
func TestPushSkipsOnlyWhatAPullRequestRunPassed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the step is bash run on an ubuntu runner")
	}
	for _, tool := range []string{"bash", "jq"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is not installed", tool)
		}
	}
	script := verifiedStep(t)

	const tree = "1111111111111111111111111111111111111111"
	artifacts := func(entries ...string) string {
		return `{"artifacts":[` + strings.Join(entries, ",") + `]}`
	}
	artifact := func(run string, expired bool) string {
		e := "false"
		if expired {
			e = "true"
		}
		return `{"name":"tested-tree-` + tree + `","expired":` + e + `,"workflow_run":{"id":` + run + `}}`
	}
	run := func(event, status, conclusion, path string) string {
		return `{"event":"` + event + `","status":"` + status + `","conclusion":"` + conclusion +
			`","path":"` + path + `","html_url":"https://github.com/o/r/actions/runs/1"}`
	}
	const build = ".github/workflows/build.yml"

	cases := []struct {
		name  string
		tree  string
		files map[string]string // API path -> response; a missing path is an API error
		want  string
	}{
		{"a pull request run passed the tree", tree, map[string]string{
			"repos/o/r/actions/artifacts": artifacts(artifact("7", false)),
			"repos/o/r/actions/runs/7":    run("pull_request", "completed", "success", build),
		}, "true"},
		{"the first run failed, a later one passed", tree, map[string]string{
			"repos/o/r/actions/artifacts": artifacts(artifact("7", false), artifact("8", false)),
			"repos/o/r/actions/runs/7":    run("pull_request", "completed", "failure", build),
			"repos/o/r/actions/runs/8":    run("pull_request", "completed", "success", build),
		}, "true"},
		{"no artifact for the tree", tree, map[string]string{
			"repos/o/r/actions/artifacts": artifacts(),
		}, "false"},
		{"the artifact expired", tree, map[string]string{
			"repos/o/r/actions/artifacts": artifacts(artifact("7", true)),
			"repos/o/r/actions/runs/7":    run("pull_request", "completed", "success", build),
		}, "false"},
		{"the run failed", tree, map[string]string{
			"repos/o/r/actions/artifacts": artifacts(artifact("7", false)),
			"repos/o/r/actions/runs/7":    run("pull_request", "completed", "failure", build),
		}, "false"},
		{"the run is still going", tree, map[string]string{
			"repos/o/r/actions/artifacts": artifacts(artifact("7", false)),
			"repos/o/r/actions/runs/7":    run("pull_request", "in_progress", "", build),
		}, "false"},
		{"the run was not a pull request's", tree, map[string]string{
			"repos/o/r/actions/artifacts": artifacts(artifact("7", false)),
			"repos/o/r/actions/runs/7":    run("push", "completed", "success", build),
		}, "false"},
		{"the run was another workflow's", tree, map[string]string{
			"repos/o/r/actions/artifacts": artifacts(artifact("7", false)),
			"repos/o/r/actions/runs/7":    run("pull_request", "completed", "success", ".github/workflows/quick.yml"),
		}, "false"},
		{"the artifact list fails", tree, map[string]string{}, "false"},
		{"the run cannot be read", tree, map[string]string{
			"repos/o/r/actions/artifacts": artifacts(artifact("7", false)),
		}, "false"},
		{"the push names no tree", "", map[string]string{
			"repos/o/r/actions/artifacts": artifacts(artifact("7", false)),
			"repos/o/r/actions/runs/7":    run("pull_request", "completed", "success", build),
		}, "false"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			bin := filepath.Join(dir, "bin")
			stubs := filepath.Join(dir, "api")
			for _, d := range []string{bin, stubs} {
				if err := os.Mkdir(d, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(stubGH), 0o700); err != nil { //nolint:gosec // G306: the stub is a command PATH must find, so it is executable; t.TempDir() file.
				t.Fatal(err)
			}
			for path, body := range tc.files {
				name := strings.ReplaceAll(path, "/", "_") + ".json"
				if err := os.WriteFile(filepath.Join(stubs, name), []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			stepFile := filepath.Join(dir, "step.sh")
			if err := os.WriteFile(stepFile, []byte(script), 0o600); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, "output")

			cmd := exec.Command("bash", stepFile)
			cmd.Env = append(os.Environ(),
				"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
				"STUB_DIR="+stubs,
				"GH_TOKEN=stub",
				"REPO=o/r",
				"TREE="+tc.tree,
				"GITHUB_OUTPUT="+output,
			)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("the step failed: %v\n%s", err, out)
			}
			got, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			if want := "pr-verified=" + tc.want + "\n"; string(got) != want {
				t.Errorf("GITHUB_OUTPUT = %q, want %q", got, want)
			}
		})
	}
}
