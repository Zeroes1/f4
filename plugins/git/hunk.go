package git

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// errNoHunks is what loadFilePatch reports for a path whose diff has
// nothing to stage (or unstage) piece by piece: no changes on that side at
// all, an untracked file (`git diff` does not show those -- Insert stages
// them whole), a binary file, or a change of the file mode alone.
var errNoHunks = errors.New("no text hunks")

// diffHunk is one "@@ -a,b +c,d @@" section of a unified diff, together
// with whether the user picked it for staging. Its body lines are kept
// byte-for-byte as git printed them (including a "\ No newline at end of
// file" marker and any trailing '\r'), so a patch rebuilt from them applies
// to exactly the same content it was cut from.
type diffHunk struct {
	oldStart, oldCount int
	newStart, newCount int
	section            string // whatever git printed after the closing "@@" (usually the enclosing function)
	lines              []string
	selected           bool
}

// headerLine renders the hunk's "@@" line as it appears in the view.
func (h *diffHunk) headerLine() string {
	return fmt.Sprintf("@@ -%d,%d +%d,%d @@%s", h.oldStart, h.oldCount, h.newStart, h.newCount, h.section)
}

// filePatch is one file's diff, split into the file header (the
// "diff --git", "index", "---" and "+++" lines) and its hunks.
//
// staged tells the two directions apart. False: `git diff`, index vs.
// worktree, and applying the picked hunks adds them to the index
// (`git add -p`). True: `git diff --cached`, HEAD vs. index, and applying
// the picked hunks takes them back out of it (`git reset -p`) -- the same
// patch applied with `git apply --cached -R`.
type filePatch struct {
	header []string
	hunks  []*diffHunk
	staged bool
}

// selectedCount reports how many hunks are picked for staging.
func (fp *filePatch) selectedCount() int {
	n := 0
	for _, h := range fp.hunks {
		if h.selected {
			n++
		}
	}
	return n
}

var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$`)

// parseRange turns the start and optional count of one side of a hunk
// header into numbers; a missing count means 1 (git-diff(1), "combined
// diff format" aside, follows GNU diff's unified format here).
func parseRange(start, count string) (int, int) {
	s, _ := strconv.Atoi(start)
	if count == "" {
		return s, 1
	}
	c, _ := strconv.Atoi(count)
	return s, c
}

// parseFilePatch splits one file's unified diff into its header and hunks.
//
// Anything before the first "diff --git" line is ignored: runGitIn returns
// stdout and stderr combined, and git may put a warning there first (the
// "LF will be replaced by CRLF" notice Git for Windows prints for an
// autocrlf repository is the usual one). A diff with no hunks at all --
// see errNoHunks -- is reported as that error rather than as an empty
// patch the view would have nothing to show for.
func parseFilePatch(diff string) (*filePatch, error) {
	text := strings.TrimSuffix(diff, "\n")
	if text == "" {
		return nil, errNoHunks
	}
	lines := strings.Split(text, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, errNoHunks
	}

	fp := &filePatch{}
	var cur *diffHunk
	for i, line := range lines[start:] {
		if i > 0 && strings.HasPrefix(line, "diff --git ") {
			return nil, errors.New("git diff reported more than one file")
		}
		if m := hunkHeaderRE.FindStringSubmatch(line); m != nil {
			cur = &diffHunk{section: m[5]}
			cur.oldStart, cur.oldCount = parseRange(m[1], m[2])
			cur.newStart, cur.newCount = parseRange(m[3], m[4])
			fp.hunks = append(fp.hunks, cur)
			continue
		}
		if cur == nil {
			fp.header = append(fp.header, line)
			continue
		}
		cur.lines = append(cur.lines, line)
	}
	if len(fp.hunks) == 0 {
		return nil, errNoHunks
	}
	return fp, nil
}

// buildPatch renders a patch with only the selected hunks, ready for
// `git apply --cached` (or `git apply --cached -R` for a staged patch). It
// returns "" when nothing is selected.
//
// One side of every kept hunk describes the index as it is now and stays
// as git printed it; the other side is recomputed, the way `git add -p`
// and `git reset -p` do it, because the hunks left out before it are not
// part of the change:
//
//   - staging (fp.staged false): the patch goes from the index ("-a") to
//     the new index, which will not contain the line-count changes of the
//     hunks left out before the kept one, so each of those shifts its "+c"
//     back by its own (newCount - oldCount);
//   - unstaging (fp.staged true): the patch is applied in reverse, from the
//     index ("+c") back to the new index ("-a"), and the hunks left out
//     before the kept one stay in the index, so each of those shifts its
//     "-a" forward by that same (newCount - oldCount).
//
// An "old mode"/"new mode" pair is dropped from the header: a mode change
// is not a hunk the user picked, so staging a few hunks must not stage it
// along with them (Insert still stages the whole file, mode included).
func buildPatch(fp *filePatch) string {
	if fp.selectedCount() == 0 {
		return ""
	}
	var b strings.Builder
	for _, line := range fp.header {
		if strings.HasPrefix(line, "old mode ") || strings.HasPrefix(line, "new mode ") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	shift := 0
	for _, h := range fp.hunks {
		if !h.selected {
			shift += h.newCount - h.oldCount
			continue
		}
		oldStart, newStart := h.oldStart, h.newStart-shift
		if fp.staged {
			oldStart, newStart = h.oldStart+shift, h.newStart
		}
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@%s\n", oldStart, h.oldCount, newStart, h.newCount, h.section)
		for _, line := range h.lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// loadFilePatch runs `git diff` -- `git diff --cached` when staged is set
// -- for path (relative to dir, the way `git status --porcelain=v2` in dir
// reports it) and parses the result.
//
// The flags pin down every piece of user configuration that would change
// the text in a way `git apply` could no longer read back: no color, no
// external diff driver or textconv filter, and the standard a/ and b/
// prefixes (diff.noprefix and diff.mnemonicPrefix would otherwise change
// them). diff.relative=false keeps the header paths relative to the
// repository root, which is where applyFilePatch runs `git apply`.
func loadFilePatch(ctx context.Context, dir, path string, staged bool) (*filePatch, error) {
	args := []string{"-c", "diff.relative=false", "diff"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--no-color", "--no-ext-diff", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "-U3", "--", path)
	out, err := runGitIn(ctx, dir, args...)
	if err != nil {
		return nil, errors.New(firstLine(string(out), err))
	}
	fp, err := parseFilePatch(string(out))
	if err != nil {
		return nil, err
	}
	fp.staged = staged
	return fp, nil
}

// repoTopLevel returns the root of the working tree dir belongs to.
func repoTopLevel(ctx context.Context, dir string) (string, error) {
	out, err := runGitIn(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", errors.New(firstLine(string(out), err))
	}
	top := strings.TrimRight(string(out), "\r\n")
	if top == "" {
		return "", errors.New("git rev-parse --show-toplevel printed nothing")
	}
	return top, nil
}

// applyFilePatch stages the selected hunks of fp -- or, for a staged
// patch, takes them out of the index: the rebuilt patch goes to a
// temporary file and `git apply --cached` (with -R for a staged patch)
// reads it from there.
//
// It runs at the repository root rather than in dir: patch paths are
// root-relative, and `git apply` started from a subdirectory silently
// skips every path outside that subdirectory ("Skipped patch ..."),
// reporting success without having staged anything.
func applyFilePatch(ctx context.Context, dir string, fp *filePatch) error {
	patch := buildPatch(fp)
	if patch == "" {
		return errors.New("no hunks selected")
	}
	top, err := repoTopLevel(ctx, dir)
	if err != nil {
		return err
	}

	f, err := os.CreateTemp("", "f4-git-hunks-*.patch")
	if err != nil {
		return err
	}
	name := f.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := f.WriteString(patch); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}

	args := []string{"apply", "--cached", "--whitespace=nowarn"}
	if fp.staged {
		args = append(args, "-R")
	}
	out, err := runGitIn(ctx, top, append(args, name)...)
	if err != nil {
		return errors.New(firstLine(string(out), err))
	}
	return nil
}
