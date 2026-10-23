package update

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestNightlyNotesParse ties the release notes the nightly job writes to the
// parser that reads them. Every installed f4 runs commitInfoFromReleaseBody
// over these notes, and a shipped updater cannot be changed: if the job's line
// stops yielding the commit and the commit time, those builds lose the check
// that tells a running nightly from a newer one (#1218). The notes are taken
// from build.yml itself, so an edit there that the parser cannot read fails
// here instead of on users' machines.
func TestNightlyNotesParse(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "build.yml"))
	if err != nil {
		t.Fatal(err)
	}

	notes := regexp.MustCompile(`--notes "([^"]*)"`).FindAllStringSubmatch(string(data), -1)
	if len(notes) != 1 {
		t.Fatalf("build.yml passes --notes %d times, want once (the nightly release)", len(notes))
	}

	// The values the job fills in: a short commit hash, the commit time as
	// `git show -s --format=%ct` gives it through date(1), and the publish
	// time.
	body := strings.NewReplacer(
		"$SHORT_SHA", "5651235",
		"$BUILD_TIME", "2026-10-22 22:33:07",
		"$PUBLISHED", "2026-10-22 23:09",
	).Replace(notes[0][1])
	if strings.Contains(body, "$") {
		t.Fatalf("the nightly notes use a variable this test does not fill in: %q", body)
	}

	commit, builtOn := commitInfoFromReleaseBody(body)
	if commit != "5651235" || builtOn != "2026-10-22 22:33:07" {
		t.Fatalf("commitInfoFromReleaseBody(%q) = %q, %q; want the commit and its time", body, commit, builtOn)
	}
	if got, want := parseBuildTime(builtOn), parseBuildTime("2026-10-22T22:33:07Z"); !got.Equal(want) {
		t.Fatalf("parseBuildTime(%q) = %v, want %v", builtOn, got, want)
	}
}
