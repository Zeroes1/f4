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

// errWholeFileOnly is what buildPatch reports for a hunk picked only in
// part when the patch adds or deletes the whole file ("--- /dev/null" or
// "+++ /dev/null"): a partial selection there would have to turn the
// creation or deletion into an ordinary modification, which is a different
// header, not just different hunk lines. Such a hunk is picked whole.
var errWholeFileOnly = errors.New("a new or deleted file can only be picked whole")

// errNoNewlineInside is what buildPatch reports when the picked lines
// would leave a line marked "\ No newline at end of file" in the middle
// of one side of the rebuilt hunk -- the last line of a file without a
// trailing newline kept as it is, with picked lines after it. No file has
// such content; git apply would reject the patch as corrupt.
var errNoNewlineInside = errors.New("the picked lines would put a line without a trailing newline in the middle of the file")

// diffHunk is one "@@ -a,b +c,d @@" section of a unified diff, together
// with which of its changed lines the user picked. Its body lines are kept
// byte-for-byte as git printed them (including a "\ No newline at end of
// file" marker and any trailing '\r'), so a patch rebuilt from them applies
// to exactly the same content it was cut from.
//
// picked runs parallel to lines and only means something for a "+" or "-"
// line; picking a whole hunk picks every one of those.
type diffHunk struct {
	oldStart, oldCount int
	newStart, newCount int
	section            string // whatever git printed after the closing "@@" (usually the enclosing function)
	lines              []string
	picked             []bool
}

// isChangeLine tells a "+" or "-" line of a hunk body from a context line
// and a "\ No newline at end of file" marker.
func isChangeLine(line string) bool {
	return line != "" && (line[0] == '+' || line[0] == '-')
}

// changeCounts returns how many "+"/"-" lines the hunk has and how many of
// them are picked.
func (h *diffHunk) changeCounts() (picked, total int) {
	for i, line := range h.lines {
		if isChangeLine(line) {
			total++
			if h.picked[i] {
				picked++
			}
		}
	}
	return picked, total
}

// anyPicked reports whether at least one changed line of the hunk is picked.
func (h *diffHunk) anyPicked() bool {
	picked, _ := h.changeCounts()
	return picked > 0
}

// allPicked reports whether the hunk is picked whole.
func (h *diffHunk) allPicked() bool {
	picked, total := h.changeCounts()
	return total > 0 && picked == total
}

// pickAll picks (or drops) every changed line of the hunk.
func (h *diffHunk) pickAll(on bool) {
	for i, line := range h.lines {
		h.picked[i] = on && isChangeLine(line)
	}
}

// headerLine renders the hunk's "@@" line as it appears in the view, with
// "[picked/changed]" after it while only some of its lines are picked.
func (h *diffHunk) headerLine() string {
	s := fmt.Sprintf("@@ -%d,%d +%d,%d @@%s", h.oldStart, h.oldCount, h.newStart, h.newCount, h.section)
	if picked, total := h.changeCounts(); picked > 0 && picked < total {
		s += fmt.Sprintf(" [%d/%d]", picked, total)
	}
	return s
}

// pickedBody rebuilds the hunk's body for the picked lines only and counts
// the lines of its two sides. The side that describes the index as it is
// now -- "-" when staging, "+" when unstaging (the patch is then applied
// in reverse) -- keeps all its lines; the other side takes only the picked
// ones:
//
//   - a picked "+" or "-" line stays as it is;
//   - an unpicked line of the index side is not part of the change, so it
//     becomes a context line (" " and the same text);
//   - an unpicked line of the other side is left out;
//   - a "\ No newline at end of file" marker goes with the line before it.
//
// This is what `git add -p`'s "e" (edit) asks the user to do by hand.
func (h *diffHunk) pickedBody(staged bool) (lines []string, oldN, newN int, err error) {
	indexSide := byte('-')
	if staged {
		indexSide = '+'
	}
	dropped := false // whether the line before a marker was left out
	for i, line := range h.lines {
		switch {
		case strings.HasPrefix(line, "\\"):
			if !dropped {
				lines = append(lines, line)
			}
			continue
		case isChangeLine(line) && h.picked[i]:
			lines = append(lines, line)
			if line[0] == '-' {
				oldN++
			} else {
				newN++
			}
		case isChangeLine(line) && line[0] == indexSide:
			lines = append(lines, " "+line[1:])
			oldN++
			newN++
		case isChangeLine(line):
			dropped = true
			continue
		default:
			lines = append(lines, line)
			oldN++
			newN++
		}
		dropped = false
	}
	if err = checkNoNewlineMarkers(lines); err != nil {
		return nil, 0, 0, err
	}
	return lines, oldN, newN, nil
}

// checkNoNewlineMarkers makes sure a "\ No newline at end of file" marker
// in a rebuilt hunk body still marks the last line of its side: after the
// line it follows, no other line of that side (context lines belong to
// both) may come.
func checkNoNewlineMarkers(lines []string) error {
	for i, line := range lines {
		if !strings.HasPrefix(line, "\\") || i == 0 {
			continue
		}
		side := lineSide(lines[i-1])
		for _, next := range lines[i+1:] {
			if strings.HasPrefix(next, "\\") {
				continue
			}
			if n := lineSide(next); side == ' ' || n == ' ' || n == side {
				return errNoNewlineInside
			}
		}
	}
	return nil
}

// filePatch is one file's diff, split into the file header (the
// "diff --git", "index", "---" and "+++" lines) and its hunks.
//
// staged tells the two directions apart. False: `git diff`, index vs.
// worktree, and applying the picked lines adds them to the index
// (`git add -p`). True: `git diff --cached`, HEAD vs. index, and applying
// the picked lines takes them back out of it (`git reset -p`) -- the same
// patch applied with `git apply --cached -R`.
type filePatch struct {
	header []string
	hunks  []*diffHunk
	staged bool
}

// lineSide is '-' or '+' for a changed line and ' ' for a context line
// (which is on both sides of the hunk).
func lineSide(line string) byte {
	if isChangeLine(line) {
		return line[0]
	}
	return ' '
}

// selectedCount reports how many hunks have at least one line picked.
func (fp *filePatch) selectedCount() int {
	n := 0
	for _, h := range fp.hunks {
		if h.anyPicked() {
			n++
		}
	}
	return n
}

// wholeFile reports whether the patch creates or deletes the file.
func (fp *filePatch) wholeFile() bool {
	for _, line := range fp.header {
		if line == "--- /dev/null" || line == "+++ /dev/null" {
			return true
		}
	}
	return false
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
	for _, h := range fp.hunks {
		h.picked = make([]bool, len(h.lines))
	}
	return fp, nil
}

// buildPatch renders a patch with only the picked lines, ready for
// `git apply --cached` (or `git apply --cached -R` for a staged patch). It
// returns "" when nothing is picked. A hunk with no picked line is left
// out; every other one is rebuilt by pickedBody, so a hunk picked whole
// comes out exactly as git printed it.
//
// One side of every kept hunk describes the index as it is now, and its
// start stays as git printed it; the start of the other side is
// recomputed, the way `git add -p` and `git reset -p` do it, from the
// line-count change (new lines minus old lines) of the rebuilt hunks
// written before it -- hunks left out, and lines left out of the hunks
// that were kept, are not part of the change:
//
//   - staging (fp.staged false): the patch goes from the index ("-a") to
//     the new index ("+c" = "-a" moved by that change);
//   - unstaging (fp.staged true): the patch is applied in reverse, from
//     the index ("+c") back to the new index ("-a" = "+c" moved back).
//
// An "old mode"/"new mode" pair is dropped from the header: a mode change
// is not a line the user picked, so staging a few lines must not stage it
// along with them (Insert still stages the whole file, mode included).
//
// A patch that creates or deletes the file only takes its hunk whole
// (errWholeFileOnly), and picked lines that would strand a
// "\ No newline at end of file" line mid-file are refused
// (errNoNewlineInside); nothing is applied in either case.
func buildPatch(fp *filePatch) (string, error) {
	if fp.selectedCount() == 0 {
		return "", nil
	}
	var b strings.Builder
	for _, line := range fp.header {
		if strings.HasPrefix(line, "old mode ") || strings.HasPrefix(line, "new mode ") {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	delta := 0
	for _, h := range fp.hunks {
		if !h.anyPicked() {
			continue
		}
		if !h.allPicked() && fp.wholeFile() {
			return "", errWholeFileOnly
		}
		lines, oldN, newN, err := h.pickedBody(fp.staged)
		if err != nil {
			return "", err
		}
		oldStart, newStart := h.oldStart, startAfter(linesBefore(h.oldStart, h.oldCount)+delta, newN)
		if fp.staged {
			oldStart, newStart = startAfter(linesBefore(h.newStart, h.newCount)-delta, oldN), h.newStart
		}
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@%s\n", oldStart, oldN, newStart, newN, h.section)
		for _, line := range lines {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		delta += newN - oldN
	}
	return b.String(), nil
}

// linesBefore turns one side of a hunk header into the number of lines of
// that side before the hunk: the start is the hunk's first line, or -- for
// a side with no lines at all ("-0,0", say) -- the line it comes after.
func linesBefore(start, count int) int {
	if count == 0 {
		return start
	}
	return start - 1
}

// startAfter is linesBefore the other way round: the start to print for a
// side with count lines that comes after the given number of lines.
func startAfter(before, count int) int {
	if count == 0 {
		return before
	}
	return before + 1
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

// applyFilePatch stages the picked lines of fp -- or, for a staged
// patch, takes them out of the index: the rebuilt patch goes to a
// temporary file and `git apply --cached` (with -R for a staged patch)
// reads it from there.
//
// It runs at the repository root rather than in dir: patch paths are
// root-relative, and `git apply` started from a subdirectory silently
// skips every path outside that subdirectory ("Skipped patch ..."),
// reporting success without having staged anything.
func applyFilePatch(ctx context.Context, dir string, fp *filePatch) error {
	patch, err := buildPatch(fp)
	if err != nil {
		return err
	}
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
