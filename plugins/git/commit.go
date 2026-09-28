package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/unxed/f4/internal/dialog"
	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/toast"
	"github.com/unxed/vtui"
)

// hasStagedChanges reports whether any entry currently listed in the panel
// already has something staged for it -- the same "index column is not
// blank/untracked" test stageActionFor (stage.go) uses to decide whether its
// own toggle key should stage or unstage an entry, reused here to decide
// whether there is anything for a commit to record.
//
// A conflicted entry (unmergedXY, stage.go) is never counted as staged here,
// even though its index slot is not blank: stageActionFor always resolves it
// to "add" (resolve), so this panel treats an unresolved conflict the same
// way Insert already does -- something to mark resolved first, not something
// ready to commit as-is.
func (p *statusPanel) hasStagedChanges() bool {
	for _, r := range p.table.Rows {
		row, ok := r.(statusRow)
		if !ok {
			continue
		}
		if stageActionFor(row.entry) == stageActionRestore {
			return true
		}
	}
	return false
}

// showCommitDialog is Ctrl+K on the status panel (panel.go's ProcessKey): a
// one-line commit message prompt over whatever Insert (stage.go) has already
// staged. It reuses internal/dialog.FileInputBox, the same single-line input
// dialog internal/app/actions.go's actionRename already builds its Rename
// prompt from, rather than composing a new vtui.Window/vtui.Edit pair from
// scratch for what is the same shape of dialog.
//
// Nothing staged means nothing for `git commit` to record: opening a dialog
// only for the user to cancel it is worse than not opening one at all, so
// this shows a toast instead, the same guard actionMkDir and friends skip
// only because they have no equivalent "there is nothing to do" case.
func (p *statusPanel) showCommitDialog() {
	if !p.hasStagedChanges() {
		toast.Show(i18n.Msg("GitStatus.NothingToCommit"), 3e9)
		return
	}

	dialog.FileInputBox(i18n.Msg("GitStatus.CommitTitle"), i18n.Msg("GitStatus.CommitPrompt"), "", p.onCommitMessageEntered)
}

// onCommitMessageEntered is the commit dialog's OnOk callback (showCommitDialog
// above), split out on its own so a test can drive the "empty/whitespace-only
// message" and "run git commit" paths directly, without going through
// internal/dialog.FileInputBox's own UI plumbing to reach them -- the same
// split diff.go keeps between showDiff (the trigger) and presentDiff (the
// pure decision it makes once the data is in hand).
func (p *statusPanel) onCommitMessageEntered(message string) {
	message = strings.TrimSpace(message)
	if message == "" {
		toast.Show(i18n.Msg("GitStatus.CommitMessageEmpty"), 3e9)
		return
	}
	p.runCommit(message)
}

// runCommit runs `git commit -m message` over the currently staged changes
// and reloads the panel, the same "mutate, then reload, toast on failure"
// shape toggleStage (stage.go) already uses for git add/restore. Multi-line
// messages, --amend and a commit signature are deliberately out of scope for
// this first cut -- see the ticket for the full remaining list.
func (p *statusPanel) runCommit(message string) {
	output, err := runGitIn(context.Background(), p.dir, "commit", "-m", message)
	if err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.CommitFailed"), firstLine(string(output), err)), 3e9)
		return
	}

	if err := p.reload(); err != nil {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.RefreshFailed"), err), 3e9)
	} else {
		toast.Show(fmt.Sprintf(i18n.Msg("GitStatus.CommitDone"), firstOutputLine(string(output))), 3e9)
	}
	if vtui.FrameManager != nil {
		vtui.FrameManager.Redraw()
	}
}

// firstOutputLine returns output's first line, or output itself if it has
// none. Unlike firstLine (panel.go), which falls back to a non-nil err's own
// message for a failed command, this has no error to fall back to: it is
// only ever used on `git commit`'s own success output, which is never empty
// (git always prints at least the "[branch sha] message" summary line).
func firstOutputLine(output string) string {
	if idx := strings.IndexByte(output, '\n'); idx >= 0 {
		return output[:idx]
	}
	return output
}
