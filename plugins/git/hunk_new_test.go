package git

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/unxed/vtinput"
)

// TestBuildPatchUnstagesPartOfANewFile: unstaging one line of a staged new
// file writes the reverse patch as a modification of it; staging part of
// the same patch is still refused.
func TestBuildPatchUnstagesPartOfANewFile(t *testing.T) {
	mk := func(mode hunkMode) *filePatch {
		fp := &filePatch{
			mode:   mode,
			header: []string{"diff --git a/n b/n", "new file mode 100644", "index 0000000..1", "--- /dev/null", "+++ b/n"},
			hunks:  []*diffHunk{testHunk(0, 0, 1, 3, "+1", "+2", "+3")},
		}
		pickLines(fp.hunks[0], 1)
		return fp
	}
	want := "diff --git a/n b/n\nindex 0000000..1\n--- a/n\n+++ b/n\n@@ -1,2 +1,3 @@\n 1\n+2\n 3\n"
	if got := mustBuildPatch(t, mk(modeUnstage)); got != want {
		t.Errorf("unstaging part of a new file:\n%s\nwant\n%s", got, want)
	}
	if _, err := buildPatch(mk(modeStage)); !errors.Is(err, errWholeFileOnly) {
		t.Errorf("staging part of a new file: error = %v, want errWholeFileOnly", err)
	}
}

// TestHunkViewUnstagesPartOfANewFile: Shift+F4 on a staged new file with
// "2" and "3" picked leaves the index with the other lines; the working
// file is untouched.
func TestHunkViewUnstagesPartOfANewFile(t *testing.T) {
	repo := realGitRepo(t)
	writeRepoFile(t, filepath.Join(repo, "keep.txt"), "keep\n")
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, filepath.Join(repo, "n.txt"), numbered(1, 5))
	runRealGit(t, repo, "add", "n.txt")

	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, modeUnstage)
	pickRows(t, v, 2, 3)
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after unstaging")
	}
	if got, want := runRealGit(t, repo, "show", ":n.txt"), "1\n4\n5\n"; got != want {
		t.Errorf("index n.txt = %q, want %q", got, want)
	}
	if got, want := readRepoFile(t, filepath.Join(repo, "n.txt")), numbered(1, 5); got != want {
		t.Errorf("working file = %q, want it untouched", got)
	}
}
