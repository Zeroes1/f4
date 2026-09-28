package git

import (
	"context"
	"errors"
	"fmt"

	"github.com/unxed/f4/internal/diffview"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/textdiff"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtui"
)

// commitChangedFiles lists the paths <hash> touches, via `git show
// --name-status` (git-diff(1)'s --name-status format, parsed by
// parseNameStatus in log.go). A merge commit -- or, for that matter, one
// that happens to revert itself to a no-op -- comes back with zero entries
// rather than an error: `git show` with no -m/-c flag prints no diff at all
// for a commit with more than one parent (git-show(1)), and showDiff below
// already treats "not exactly one changed file" as "nothing to show"
// rather than mistaking either shape for a failure.
func commitChangedFiles(ctx context.Context, dir, hash string) ([]logDiffEntry, error) {
	output, err := runGitIn(ctx, dir, "show", "--no-color", "--pretty=format:", "--name-status", hash)
	if err != nil {
		return nil, errors.New(firstLine(string(output), err))
	}
	return parseNameStatus(output), nil
}

// revisionFileContent reads path's content as of rev (for example "<hash>"
// or "<hash>^") in dir's repository, split into diff lines -- the same
// diffSideFromBytes-wrapped `git show <rev>:./<path>` headFileContent
// (diff.go) runs for "HEAD", generalized to any revision so it can also read
// a commit's parent. A failed lookup (the path did not exist at rev -- a
// root commit's missing parent, or the path added/deleted by this very
// commit) is, like headFileContent's own HEAD miss, deliberately not an
// error: (nil, nil) gives an empty side, so the whole file shows as inserted
// or deleted, the same shape headFileContent/worktreeFileContent give the
// status panel's own Enter.
func revisionFileContent(ctx context.Context, dir, rev, path string) ([]string, error) {
	out, err := runGitIn(ctx, dir, "show", rev+":./"+path)
	if err != nil {
		return nil, nil
	}
	return diffSideFromBytes(out)
}

// showDiff is Enter on the log view (logview.go's ProcessKey): a
// side-by-side diff of the commit under the cursor, reusing
// internal/diffview exactly as the status panel's own showDiff (diff.go)
// does for a working-tree change -- just against the commit's parent and
// the commit itself (<hash>^ and <hash>) instead of HEAD and the worktree.
//
// This first version only handles a commit that changes exactly one path:
// diffview.DiffView takes two whole files, not a multi-file patch, and a
// commit touching several files would need either picking one of them (a
// changed-files sub-list, its own atomic follow-up of f4#659) or a
// different, patch-shaped widget entirely -- both out of scope for this
// part. Zero changed files (a merge commit, or a no-op one --
// commitChangedFiles's own doc comment) and more than one both show a toast
// instead of guessing which file the user meant.
//
// Loading the changed-file list and both revisions' content can mean three
// subprocesses, so this runs off the UI goroutine the same way the status
// panel's own showDiff (diff.go) does, posting the result back with
// vtui.TaskContext.RunOnUI.
func (lv *LogView) showDiff() {
	entry, ok := lv.selectedEntry()
	if !ok {
		return
	}
	hash, shortHash, dir := entry.Hash, entry.ShortHash, lv.dir
	vtui.RunAsync(func(ctx *vtui.TaskContext) {
		files, err := commitChangedFiles(ctx, dir, hash)
		if err != nil {
			ctx.RunOnUI(func() {
				toast.Show(fmt.Sprintf(i18n.Msg("GitLog.DiffFailed"), err), 3e9)
			})
			return
		}
		if len(files) != 1 {
			ctx.RunOnUI(func() {
				toast.Show(fmt.Sprintf(i18n.Msg("GitLog.DiffUnsupported"), len(files)), 3e9)
			})
			return
		}
		f := files[0]
		left, leftErr := revisionFileContent(ctx, dir, hash+"^", f.oldPath())
		right, rightErr := revisionFileContent(ctx, dir, hash, f.Path)
		ctx.RunOnUI(func() {
			presentLogDiff(shortHash, f, left, right, leftErr, rightErr)
		})
	})
}

// presentLogDiff runs on the UI goroutine only: it turns showDiff's loaded
// content into either an error dialog, a toast, or an open DiffView, the
// same background/UI split presentDiff (diff.go) keeps for the status
// panel's own Enter.
func presentLogDiff(shortHash string, f logDiffEntry, left, right []string, leftErr, rightErr error) {
	if leftErr != nil {
		vtui.ShowMessage(i18n.Msg("Error.Title"), fmt.Sprintf(i18n.Msg("GitDiff.ReadFailed"), f.oldPath(), leftErr), []string{i18n.Msg("vtui.Ok")})
		return
	}
	if rightErr != nil {
		vtui.ShowMessage(i18n.Msg("Error.Title"), fmt.Sprintf(i18n.Msg("GitDiff.ReadFailed"), f.Path, rightErr), []string{i18n.Msg("vtui.Ok")})
		return
	}

	dv, err := diffview.NewDiffView(shortHash+"^:"+f.oldPath(), shortHash+":"+f.Path, left, right)
	if err != nil {
		if err == textdiff.ErrTooLarge {
			vtui.ShowMessage(i18n.Msg("Error.Title"), i18n.Msg("GitDiff.TooLarge"), []string{i18n.Msg("vtui.Ok")})
			return
		}
		vtui.ShowMessage(i18n.Msg("Error.Title"), fmt.Sprintf(i18n.Msg("GitDiff.CompareFailed"), err), []string{i18n.Msg("vtui.Ok")})
		return
	}

	if vtui.FrameManager != nil {
		dv.ResizeConsole(vtui.FrameManager.GetScreenSize(), vtui.FrameManager.GetScreenHeight())
		vtui.FrameManager.AddScreen(dv)
	}
}
