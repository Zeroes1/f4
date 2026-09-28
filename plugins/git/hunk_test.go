package git

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// realGitRepo creates an empty repository in a temporary directory, with
// just enough local configuration for commits to work anywhere: an
// identity, no signing, and no CRLF conversion (Git for Windows turns it
// on globally, and its warnings would only add noise to these tests).
func realGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	runRealGit(t, dir, "init", "-q")
	runRealGit(t, dir, "config", "user.name", "f4 test")
	runRealGit(t, dir, "config", "user.email", "f4@example.invalid")
	runRealGit(t, dir, "config", "commit.gpgsign", "false")
	runRealGit(t, dir, "config", "core.autocrlf", "false")
	return dir
}

func runRealGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return string(out)
}

func writeRepoFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// numbered returns lines "1".."n", each followed by a newline.
func numbered(from, to int) string {
	var b strings.Builder
	for i := from; i <= to; i++ {
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	return b.String()
}

// twoHunkRepo commits a 30-line file at rel inside a fresh repository and
// then changes it in two places far enough apart to give two hunks: two
// lines inserted after line 2 (the first hunk grows the file) and line 25
// replaced by "X".
func twoHunkRepo(t *testing.T, rel string) string {
	t.Helper()
	repo := realGitRepo(t)
	path := filepath.Join(repo, filepath.FromSlash(rel))
	writeRepoFile(t, path, numbered(1, 30))
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, path, numbered(1, 2)+"A\nB\n"+numbered(3, 24)+"X\n"+numbered(26, 30))
	return repo
}

func openStatusPanelIn(t *testing.T, dir string) *statusPanel {
	t.Helper()
	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: dir}, Bounds: [4]int{0, 0, 59, 19}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = controller.Close() })
	return controller.(*statusPanel)
}

func key(vk uint16) *vtinput.InputEvent {
	return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vk}
}

func openHunksForOnlyEntry(t *testing.T, p *statusPanel) *HunkView {
	t.Helper()
	return openHunksOf(t, p, false)
}

// openHunksOf opens the HunkView F4 (staged false) or Shift+F4 (staged
// true) would open for the entry under the cursor.
func openHunksOf(t *testing.T, p *statusPanel, staged bool) *HunkView {
	t.Helper()
	entry, ok := p.selectedEntry()
	if !ok {
		t.Fatal("status panel has no entry under the cursor")
	}
	v, err := p.openHunkView(entry, staged)
	if err != nil {
		t.Fatalf("openHunkView: %v", err)
	}
	v.SetPosition(0, 0, 59, 19)
	return v
}

func TestParseFilePatchSkipsLeadingNoiseAndKeepsLinesVerbatim(t *testing.T) {
	diff := "warning: in the working copy of 'f', LF will be replaced by CRLF\n" +
		"diff --git a/f b/f\n" +
		"index 1111111..2222222 100644\n" +
		"--- a/f\n" +
		"+++ b/f\n" +
		"@@ -1 +1,2 @@ func main\n" +
		" one\r\n" +
		"+two\n" +
		"@@ -9,3 +10,2 @@\n" +
		" a\n" +
		"-b\n" +
		" c\n" +
		"\\ No newline at end of file\n"
	fp, err := parseFilePatch(diff)
	if err != nil {
		t.Fatal(err)
	}
	if len(fp.header) != 4 || fp.header[0] != "diff --git a/f b/f" {
		t.Fatalf("header = %q", fp.header)
	}
	if len(fp.hunks) != 2 {
		t.Fatalf("hunks = %d, want 2", len(fp.hunks))
	}
	h := fp.hunks[0]
	if h.oldStart != 1 || h.oldCount != 1 || h.newStart != 1 || h.newCount != 2 || h.section != " func main" {
		t.Errorf("first hunk = %+v", *h)
	}
	if h.lines[0] != " one\r" {
		t.Errorf("CR was not kept: %q", h.lines[0])
	}
	if got := fp.hunks[1].lines; len(got) != 4 || got[3] != "\\ No newline at end of file" {
		t.Errorf("second hunk lines = %q", got)
	}
}

func TestParseFilePatchWithoutHunks(t *testing.T) {
	for _, diff := range []string{
		"",
		"diff --git a/img b/img\nindex 1..2 100644\nBinary files a/img and b/img differ\n",
		"diff --git a/s b/s\nold mode 100644\nnew mode 100755\n",
	} {
		if _, err := parseFilePatch(diff); !errors.Is(err, errNoHunks) {
			t.Errorf("parseFilePatch(%q) error = %v, want errNoHunks", diff, err)
		}
	}
}

func TestBuildPatchShiftsKeptHunksAndDropsModeChange(t *testing.T) {
	fp := &filePatch{
		header: []string{"diff --git a/f b/f", "old mode 100644", "new mode 100755", "index 1..2", "--- a/f", "+++ b/f"},
		hunks: []*diffHunk{
			{oldStart: 1, oldCount: 5, newStart: 1, newCount: 7, lines: []string{"+A", "+B"}},
			{oldStart: 22, oldCount: 7, newStart: 24, newCount: 7, lines: []string{"-25", "+X"}, selected: true},
		},
	}
	got := buildPatch(fp)
	want := "diff --git a/f b/f\nindex 1..2\n--- a/f\n+++ b/f\n@@ -22,7 +22,7 @@\n-25\n+X\n"
	if got != want {
		t.Errorf("buildPatch =\n%s\nwant\n%s", got, want)
	}

	fp.hunks[1].selected = false
	if got := buildPatch(fp); got != "" {
		t.Errorf("buildPatch with nothing selected = %q, want empty", got)
	}
}

// TestHunkViewStagesOnlyThePickedHunk walks the real UI on a real
// repository: F4's view lists both hunks, the cursor goes down to the
// second one, Insert picks it, Enter stages it -- and the index then has
// exactly that change while the first hunk stays unstaged. Staging the
// second hunk alone is the case that needs buildPatch's line shift, since
// the first hunk (left out) inserts two lines above it.
func TestHunkViewStagesOnlyThePickedHunk(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)

	if n := len(v.patch.hunks); n != 2 {
		t.Fatalf("hunks = %d, want 2", n)
	}
	for i := 0; i <= len(v.patch.hunks[0].lines); i++ {
		v.ProcessKey(key(vtinput.VK_DOWN))
	}
	row, ok := v.cursorRow()
	if !ok || !row.header || row.hunk != v.patch.hunks[1] {
		t.Fatalf("cursor is not on the second hunk's @@ line: %+v", row)
	}
	if !v.ProcessKey(key(vtinput.VK_INSERT)) {
		t.Fatal("Insert was not claimed")
	}
	if v.patch.hunks[0].selected || !v.patch.hunks[1].selected {
		t.Fatalf("selection = %v/%v, want false/true", v.patch.hunks[0].selected, v.patch.hunks[1].selected)
	}
	if !v.ProcessKey(key(vtinput.VK_RETURN)) {
		t.Fatal("Enter was not claimed")
	}
	if !v.IsDone() {
		t.Error("the view did not close after staging")
	}

	staged := runRealGit(t, repo, "diff", "--cached")
	if !strings.Contains(staged, "+X") || strings.Contains(staged, "+A") {
		t.Errorf("index diff should hold only the second hunk:\n%s", staged)
	}
	unstaged := runRealGit(t, repo, "diff")
	if !strings.Contains(unstaged, "+A") || strings.Contains(unstaged, "+X") {
		t.Errorf("worktree diff should hold only the first hunk:\n%s", unstaged)
	}
	entry, ok := p.selectedEntry()
	if !ok || entry.Path != "f.txt" || entry.XY != "MM" {
		t.Errorf("status panel after staging: %+v, want f.txt MM", entry)
	}
}

// TestHunkViewInSubdirectory: the panel shows paths relative to its own
// directory, `git diff` prints them relative to the repository root, and
// `git apply` run from a subdirectory would silently skip them -- this is
// the case applyFilePatch runs at the top level for.
func TestHunkViewInSubdirectory(t *testing.T) {
	repo := twoHunkRepo(t, "sub/f.txt")
	p := openStatusPanelIn(t, filepath.Join(repo, "sub"))
	v := openHunksForOnlyEntry(t, p)

	// Insert on the first hunk picks it and jumps to the second.
	v.ProcessKey(key(vtinput.VK_INSERT))
	if row, _ := v.cursorRow(); row.hunk != v.patch.hunks[1] || !row.header {
		t.Errorf("Insert did not move the cursor to the next hunk")
	}
	v.ProcessKey(key(vtinput.VK_F2))

	staged := runRealGit(t, repo, "diff", "--cached")
	if !strings.Contains(staged, "+A") || strings.Contains(staged, "+X") {
		t.Errorf("index diff should hold only the first hunk:\n%s", staged)
	}
}

func TestHunkViewTogglesBackAndRefusesEmptySelection(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)

	v.ProcessKey(key(vtinput.VK_SPACE)) // picks hunk 1, moves to hunk 2
	v.ProcessKey(key(vtinput.VK_UP))
	v.ProcessKey(key(vtinput.VK_SPACE)) // cursor inside hunk 1: drops it again
	if v.patch.selectedCount() != 0 {
		t.Fatalf("selected = %d, want 0", v.patch.selectedCount())
	}
	v.ProcessKey(key(vtinput.VK_RETURN))
	if v.IsDone() {
		t.Error("Enter with nothing picked closed the view")
	}
	if staged := runRealGit(t, repo, "diff", "--cached"); staged != "" {
		t.Errorf("nothing should be staged:\n%s", staged)
	}
	v.ProcessKey(key(vtinput.VK_ESCAPE))
	if !v.IsDone() {
		t.Error("Esc did not close the view")
	}
}

func TestHunksOfUntrackedFileAreNotOffered(t *testing.T) {
	repo := realGitRepo(t)
	writeRepoFile(t, filepath.Join(repo, "new.txt"), "hello\n")
	p := openStatusPanelIn(t, repo)
	entry, ok := p.selectedEntry()
	if !ok {
		t.Fatal("no entry")
	}
	if _, err := p.openHunkView(entry, false); !errors.Is(err, errNoHunks) {
		t.Errorf("openHunkView(untracked) error = %v, want errNoHunks", err)
	}
	// F4 through the panel's own keys: consumed, only a toast.
	if !p.ProcessKey(key(vtinput.VK_F4)) {
		t.Error("F4 was not claimed")
	}
}

// TestHunkViewScreenDump draws the view after picking the first hunk; run
// it with -v to see the screen.
func TestHunkViewScreenDump(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksForOnlyEntry(t, p)
	v.SetPosition(0, 0, 59, 21)
	v.ProcessKey(key(vtinput.VK_INSERT))

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(60, 22)
	v.Show(scr)
	var b strings.Builder
	scr.Dump(&b)
	text := b.String()
	if i := strings.Index(text, "--- CELL METADATA"); i >= 0 {
		text = text[:i]
	}
	t.Log("\n" + text)
	if !strings.Contains(text, "f.txt") || !strings.Contains(text, "@@ -1,5 +1,7 @@") {
		t.Errorf("screen misses the title or the first hunk:\n%s", text)
	}
}

func TestBuildPatchStagedShiftsOldSide(t *testing.T) {
	fp := &filePatch{
		staged: true,
		header: []string{"diff --git a/f b/f", "index 1..2 100644", "--- a/f", "+++ b/f"},
		hunks: []*diffHunk{
			{oldStart: 1, oldCount: 5, newStart: 1, newCount: 7, lines: []string{"+A", "+B"}},
			{oldStart: 22, oldCount: 7, newStart: 24, newCount: 7, lines: []string{"-25", "+X"}, selected: true},
		},
	}
	// The first hunk stays in the index, so the kept one's "-a" moves
	// forward by its two lines; "+c" already counts lines of the index.
	want := "diff --git a/f b/f\nindex 1..2 100644\n--- a/f\n+++ b/f\n@@ -24,7 +24,7 @@\n-25\n+X\n"
	if got := buildPatch(fp); got != want {
		t.Errorf("buildPatch =\n%s\nwant\n%s", got, want)
	}
}

// stagedTwoHunkRepo is twoHunkRepo with both hunks staged.
func stagedTwoHunkRepo(t *testing.T, rel string) string {
	t.Helper()
	repo := twoHunkRepo(t, rel)
	runRealGit(t, repo, "add", "-A")
	return repo
}

// TestHunkViewUnstagesOnlyThePickedHunk is the `git reset -p` side of
// TestHunkViewStagesOnlyThePickedHunk: both hunks staged, Shift+F4's view
// lists them from `git diff --cached`, the second one is picked, Enter
// takes it out of the index -- the first hunk (left in, two lines above)
// is the case needing the shift of the "-a" side.
func TestHunkViewUnstagesOnlyThePickedHunk(t *testing.T) {
	repo := stagedTwoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, true)

	if !v.patch.staged || len(v.patch.hunks) != 2 {
		t.Fatalf("staged = %v, hunks = %d; want true, 2", v.patch.staged, len(v.patch.hunks))
	}
	for i := 0; i <= len(v.patch.hunks[0].lines); i++ {
		v.ProcessKey(key(vtinput.VK_DOWN))
	}
	v.ProcessKey(key(vtinput.VK_INSERT))
	if v.patch.hunks[0].selected || !v.patch.hunks[1].selected {
		t.Fatalf("selection = %v/%v, want false/true", v.patch.hunks[0].selected, v.patch.hunks[1].selected)
	}
	if patch := buildPatch(v.patch); !strings.Contains(patch, "@@ -24,7 +24,7 @@") {
		t.Errorf("unstage patch does not carry the shifted hunk header:\n%s", patch)
	}
	if !v.ProcessKey(key(vtinput.VK_RETURN)) {
		t.Fatal("Enter was not claimed")
	}
	if !v.IsDone() {
		t.Error("the view did not close after unstaging")
	}

	staged := runRealGit(t, repo, "diff", "--cached")
	if !strings.Contains(staged, "+A") || strings.Contains(staged, "+X") {
		t.Errorf("index diff should keep only the first hunk:\n%s", staged)
	}
	unstaged := runRealGit(t, repo, "diff")
	if !strings.Contains(unstaged, "+X") || strings.Contains(unstaged, "+A") {
		t.Errorf("worktree diff should hold only the second hunk:\n%s", unstaged)
	}
	if got := runRealGit(t, repo, "show", ":f.txt"); got != numbered(1, 2)+"A\nB\n"+numbered(3, 30) {
		t.Errorf("index content after unstaging:\n%s", got)
	}
	entry, ok := p.selectedEntry()
	if !ok || entry.Path != "f.txt" || entry.XY != "MM" {
		t.Errorf("status panel after unstaging: %+v, want f.txt MM", entry)
	}
}

// TestHunkViewUnstagesInSubdirectory: `git apply --cached -R` has to run
// at the repository root as well.
func TestHunkViewUnstagesInSubdirectory(t *testing.T) {
	repo := stagedTwoHunkRepo(t, "sub/f.txt")
	p := openStatusPanelIn(t, filepath.Join(repo, "sub"))
	v := openHunksOf(t, p, true)

	v.ProcessKey(key(vtinput.VK_INSERT)) // the first hunk
	v.ProcessKey(key(vtinput.VK_F2))
	if !v.IsDone() {
		t.Fatal("the view did not close after unstaging")
	}

	staged := runRealGit(t, repo, "diff", "--cached")
	if !strings.Contains(staged, "+X") || strings.Contains(staged, "+A") {
		t.Errorf("index diff should keep only the second hunk:\n%s", staged)
	}
	if got := runRealGit(t, repo, "show", ":sub/f.txt"); got != numbered(1, 24)+"X\n"+numbered(26, 30) {
		t.Errorf("index content after unstaging:\n%s", got)
	}
}

// TestUnstagingTheOnlyHunkOfANewFile: a staged new file is one hunk from
// /dev/null; unstaging it takes the file out of the index and leaves it
// untracked, the same end state Insert gives.
func TestUnstagingTheOnlyHunkOfANewFile(t *testing.T) {
	repo := realGitRepo(t)
	writeRepoFile(t, filepath.Join(repo, "keep.txt"), "keep\n")
	runRealGit(t, repo, "add", "-A")
	runRealGit(t, repo, "commit", "-q", "-m", "initial")
	writeRepoFile(t, filepath.Join(repo, "new.txt"), "one\ntwo\n")
	runRealGit(t, repo, "add", "new.txt")

	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, true)
	if len(v.patch.hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(v.patch.hunks))
	}
	v.ProcessKey(key(vtinput.VK_INSERT))
	v.ProcessKey(key(vtinput.VK_RETURN))
	if !v.IsDone() {
		t.Fatal("the view did not close after unstaging")
	}
	if got := runRealGit(t, repo, "status", "--porcelain"); got != "?? new.txt\n" {
		t.Errorf("status after unstaging the new file = %q, want untracked", got)
	}
}

// TestStagedHunksNotOffered: Shift+F4 has nothing to show for a file with
// no staged changes, nor for a staged rename (see openHunkView).
func TestStagedHunksNotOffered(t *testing.T) {
	repo := twoHunkRepo(t, "f.txt") // changed, nothing staged
	p := openStatusPanelIn(t, repo)
	entry, ok := p.selectedEntry()
	if !ok {
		t.Fatal("no entry")
	}
	if _, err := p.openHunkView(entry, true); !errors.Is(err, errNoHunks) {
		t.Errorf("openHunkView(unstaged only, staged) error = %v, want errNoHunks", err)
	}
	shiftF4 := key(vtinput.VK_F4)
	shiftF4.ControlKeyState = vtinput.ShiftPressed
	if !p.ProcessKey(shiftF4) {
		t.Error("Shift+F4 was not claimed")
	}

	runRealGit(t, repo, "checkout", "--", "f.txt")
	runRealGit(t, repo, "mv", "f.txt", "g.txt")
	if err := p.reload(); err != nil {
		t.Fatal(err)
	}
	entry, ok = p.selectedEntry()
	if !ok || entry.OrigPath == "" {
		t.Fatalf("expected a rename entry, got %+v", entry)
	}
	if _, err := p.openHunkView(entry, true); !errors.Is(err, errNoHunks) {
		t.Errorf("openHunkView(rename, staged) error = %v, want errNoHunks", err)
	}
}

// TestUnstageHunkViewScreenDump draws the Shift+F4 view after picking the
// first hunk; run it with -v to see the screen.
func TestUnstageHunkViewScreenDump(t *testing.T) {
	repo := stagedTwoHunkRepo(t, "f.txt")
	p := openStatusPanelIn(t, repo)
	v := openHunksOf(t, p, true)
	v.SetPosition(0, 0, 59, 21)
	v.ProcessKey(key(vtinput.VK_INSERT))

	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(60, 22)
	v.Show(scr)
	var b strings.Builder
	scr.Dump(&b)
	text := b.String()
	if i := strings.Index(text, "--- CELL METADATA"); i >= 0 {
		text = text[:i]
	}
	t.Log("\n" + text)
	if !strings.Contains(text, "Unstage hunks: f.txt") || !strings.Contains(text, "@@ -22,7 +24,7 @@") {
		t.Errorf("screen misses the title or the second hunk:\n%s", text)
	}
}
