# Git client

f4's built-in git client (f4#659): status, diffs, staging, log and branches,
wrapping the host's own `git` binary the way `plugins/multiarc` wraps
archive tools (f4#609) rather than linking a Go git implementation.

## What this first part does (f4#659 part 1 of N)

- Registers a `vfs.PanelProvider` (`f4.gitstatus`) -- like `plugins/proclist`
  (f4#312), this plugin's first consumer of that API. Opening it (menu,
  command palette, or **Commands -> Git status** / **Ctrl+Alt+G**) replaces
  the active panel slot with a read-only list of `git status`'s changed,
  unmerged and untracked paths for that panel's current directory.
- The **Status** column is the same two-letter code `git status --short`
  prints (`M `, ` M`, `A `, `??`, `R `, `UU`, ...) rather than a f4-invented
  vocabulary, so anyone who already knows `git status -s` recognizes it
  immediately. A rename or copy shows as `old -> new` in the **Path** column.
- **F5** re-runs `git status` and replaces the list. There is no background
  refresh: unlike ProcList's live `/proc` view, a git status is a
  point-in-time snapshot the user asks for, not something that needs a
  ticker.
- Sortable by either column (click a header) and has type-to-filter
  (`vtui.Table.QuickSearch`).
- If the active panel's directory is not inside a git repository (or `git`
  is not on `PATH`), opening the panel fails and the host shows git's own
  error as a toast (`internal/panel/plugins.go`'s existing
  `PanelProvider.Open` error handling) -- nothing extra to build here.

## Part 2: diffing a changed file (`diff.go`)

- **Enter** on a listed entry opens a side-by-side diff (`internal/diffview`,
  f4#613) of that one file: its `HEAD` content on the left, its current
  working-tree content on the right -- the exact same widget
  `internal/app/compare_content_ui.go` already uses for f4's plain "Compare
  files by content", per the scope split agreed in f4#613.
- A file with no `HEAD` version yet (new/untracked, or an unborn branch) just
  shows an empty left side, so the whole file appears inserted -- the same
  shape `git diff --no-index /dev/null <file>` would produce. A file deleted
  from the worktree since the panel last loaded shows the mirror image: an
  empty right side. A rename or copy diffs the old name's `HEAD` content
  against the new name's worktree content.
- Same guards as "Compare files by content": a side over 8 MiB, or one that
  looks binary (a NUL byte), is refused with a message instead of diffed.

## Part 3: staging and unstaging a whole file (`stage.go`)

- **Insert** on a listed entry stages it (`git add`) if nothing about it is
  staged yet -- an unstaged modification, a deletion, or an untracked file
  (`??`) -- or unstages it (`git restore --staged`) if the index already has
  something staged for it. This is a toggle per file, not per hunk: staging
  part of a file's changes is a follow-up part of f4#659 (it needs the diff
  widget from f4#613 to show and select hunks, not just the two-letter
  status this panel already displays).
- A merge conflict (`DD`, `AU`, `UD`, `UA`, `DU`, `AA` or `UU`) always runs
  `git add`, regardless of which of those seven codes it is: the only
  sensible action from this panel is marking it resolved, and there is
  nothing meaningful to "unstage" from a conflict.
- A rename or copy runs the command against *both* the old and new names,
  not just the one this panel displays: git records a rename as two separate
  index changes (an add and a delete) that `git status` only *displays*
  combined into one line, so touching only the new name would stage or
  unstage half of it and leave the other half behind.
- Insert was chosen over the Space key some other git clients (`lazygit`,
  `tig`) use for the same gesture: this table's `QuickSearch` (like every
  other `vtui.Table` in f4) claims every printable character while focused,
  Space included, so Space here would type into the filter instead of
  staging anything. Insert is f4's own existing "mark an item" key in a
  Far/Norton-Commander-style file panel (see `isAddItemKey` in
  `internal/panel/menukeys.go`), so it also fits this panel's own
  vocabulary better than a foreign tool's convention would.
- After a successful stage/unstage, the panel reloads (the same `git status`
  call F5 runs) and moves the cursor back to the same path, so toggling
  several files in a row by stepping down the list does not keep resetting
  the cursor to the top of a re-sorted table. A failed git command leaves
  the panel untouched and shows the failure as a toast, the same way a
  failed F5 refresh already does.

## Part 4: committing staged changes (`commit.go`)

- **Ctrl+K** opens a one-line commit message prompt over whatever Insert
  (part 3) has already staged, reusing `internal/dialog.FileInputBox` -- the
  same single-line input dialog `internal/app/actions.go`'s Rename command
  already builds its own prompt from -- rather than composing a new
  `vtui.Window`/`vtui.Edit` pair from scratch for the same shape of dialog.
  Confirming it runs `git commit -m "<message>"` over the index and reloads
  the panel; a leading/trailing-whitespace-only message is rejected the same
  way an empty one is, with a toast, before any commit runs.
- Nothing staged means nothing for `git commit` to record: pressing Ctrl+K
  with an empty index shows a toast ("nothing staged to commit") instead of
  opening a dialog the user would only have to cancel.
- Ctrl+K, not a bare letter: this table's `QuickSearch` claims every
  printable character while focused (the same reason Insert, not Space, was
  chosen for staging in part 3), and Ctrl+K is not already one of f4's own
  action hotkeys anywhere else in the application.
- Deliberately minimal for this first cut: a single-line message only (no
  multi-line body, no `--amend`, no commit signature/author override). Those
  are their own follow-up parts if and when they turn out to be needed --
  see the ticket.

## Part 5: a read-only commit log (`log.go`, `logview.go`, `logdiff.go`)

- **Ctrl+E** on the status panel opens a read-only list of the repository's
  last 200 commits (hash, author, date, subject) -- `git log
  --pretty=format:...`, parsed once by `parseLog` (log.go). Not Ctrl+L, the
  more obvious mnemonic ("Log"): this panel is itself one of `PanelsFrame`'s
  `AltPanels`, and Ctrl+L is already the global **Info Panel** toggle, whose
  own key handling deliberately falls through past *any* focused AltPanel so
  it still works no matter what the active side is showing -- claiming it
  here for something unrelated would break that. Ctrl+G ("Git") is likewise
  already the global **Apply command**. Ctrl+E is free (see panel.go's
  `ProcessKey` doc comment for the full reasoning) and, unlike Ctrl+I or
  Ctrl+J, is not a letter whose Ctrl form some terminals conflate with a
  plain control character (Tab, Line Feed).
- It opens as its own full-screen `vtui.Frame`
  (`vtui.FrameManager.AddScreen`), the same way Enter's diff (part 2) already
  does, rather than a second `vfs.PanelProvider` replacing the status panel
  in its slot: that needs no "go back to what was open before" stack of its
  own, and Escape simply pops back to the status panel underneath, exactly
  as closing a diff already does.
- **F5** re-runs `git log` and replaces the list, the same point-in-time
  refresh the status panel's own F5 is. Sorting is not offered -- a commit
  log's only meaningful order is the one `git log` already produced -- but
  QuickSearch is, the same type-to-filter gesture the status panel has,
  useful here for jumping to a commit by author or by a word from its
  subject.
- **Enter** on a commit shows a side-by-side diff, reusing
  `internal/diffview` (f4#613) exactly as the status panel's own Enter
  (part 2) does -- against the commit's parent and the commit itself
  (`<hash>^` and `<hash>`) instead of HEAD and the worktree. This first
  version only handles a commit that changes exactly one file:
  `diffview.DiffView` takes two whole files, not a multi-file patch, and
  picking one file out of several (or rendering a patch-shaped view instead)
  is its own follow-up. A commit with zero changed files (a merge commit,
  which `git show` does not diff without `-m`/`-c`) or more than one shows a
  toast instead of guessing.

## Part 7: switching branches (`branch.go`, `branchview.go`)

- **Ctrl+S** on the status panel opens a read-only list of local branches
  (`git branch --list --no-color`, parsed by `parseBranchList` in
  `branch.go`), the current one marked with `*` in its own column -- the
  same `vtui.BorderedFrame`+`vtui.Table` full-screen-frame shape Ctrl+E's log
  (part 5) already uses, for the same reason: it needs no panel-provider "go
  back to what was open before" stack of its own, so Escape/F10 simply pops
  it and the status panel underneath is exactly as it was. Not Ctrl+B (the
  more obvious mnemonic, "Branch"): plain `B` is claimed by this table's own
  QuickSearch like every letter, and Ctrl+B is already the global
  **Panel.ToggleKeyBar**, which this panel's own key handling has no
  fallthrough exception for (unlike Ctrl+L/Ctrl+G, see panel.go's
  `ProcessKey` doc comment) -- claiming it here would silently break the key
  bar toggle while this view is open. Ctrl+S ("**S**witch branch") is free by
  the same `grep DefaultKeys` check that justified Ctrl+K and Ctrl+E, and is
  already precedent for a view-local Ctrl+S: `internal/media/image_view.go`
  binds it to that view's own slide-show toggle, scoped the same way.
- **Enter** on a branch runs `git switch <branch>` (not `git checkout`: the
  newer, purpose-built command gives a clearer refusal than checkout's own
  more overloaded one when the working tree has changes a switch would
  overwrite) and, on success, reloads both this list (moving the `*` to the
  new current branch) and the status panel underneath (so its title and
  entries reflect the new HEAD). The view itself stays open afterward --
  switching branches does not imply "done looking at branches," the same way
  showing a diff from the log view does not close the log.
- A working tree with local changes that switching would overwrite is not
  detected ahead of time by this plugin: `git switch` itself refuses with
  its own explanatory message in that case, which surfaces here as a toast
  exactly like any other failed git command in this plugin (`toggleStage`,
  `runCommit`) -- there is no attempt here to stash or merge on the user's
  behalf.
- **F5** re-runs `git branch --list` and replaces the list, the same
  point-in-time refresh every other list in this plugin has. Sorting is not
  offered (a short branch list has no order worth resorting away from), but
  QuickSearch is, the same type-to-filter gesture the status and log views
  already have.

## Part 9: a multi-line commit message editor (`commit.go`)

- **Ctrl+K** now opens `showCommitMessageEditor`, a `vtui.MultiLineEdit`
  field, instead of part 4's one-line `internal/dialog.FileInputBox` prompt
  -- the "editor for commit messages" the ticket asked for from the start,
  which a single-line field could never really be: a message with its own
  subject line, a blank line, and a longer body (the shape git itself
  expects from an `$EDITOR`-composed `COMMIT_EDITMSG`) had nowhere to go
  before this. Composed by hand from `vtui.NewCenteredDialog`,
  `vtui.NewButton` and `vtui.NewHBoxLayout`, the same way
  `plugins/envman/dialogs.go`'s own profile dialog builds its own
  `MultiLineEdit` field, rather than through `internal/dialog.FileDialog`:
  that helper's fixed heights are sized for its own family of file dialogs
  (copy/move/rename), not a resizable paragraph of text.
- Confirming (the **Ok** button, or its own mnemonic) runs `git commit -m
  "<message>"` exactly as part 4 did -- `-m`'s argument reaches git through
  `exec.Cmd`'s own argv, never a shell, so an embedded newline needs no
  escaping and git records the message verbatim (after its own
  `commit.cleanup=strip`, the same cleanup a message typed into `$EDITOR`
  would get either way). Only the dialog changed; `onCommitMessageEntered`
  and `runCommit` (commit.go) are otherwise unchanged from part 4.
- Enter inside the field types a newline, the way any multi-line text field
  should -- it does not submit the dialog the way a single-line `vtui.Edit`'s
  Enter would have. `strings.TrimSpace` on the confirmed text still only
  strips a blank line the user left at the very start or end of the field;
  it does not touch a blank line between the subject and the body, which is
  exactly the convention a multi-line message needs to keep. A message that
  is blank throughout is rejected with the same toast an empty single-line
  one already was.
- `--amend` and a commit signature/author override remain out of scope --
  see the ticket for the remaining list.

## Part 10: creating and deleting branches (`branchview.go`)

- **Insert** on the branch list opens a single-line
  `internal/dialog.FileInputBox` prompt for a new branch's name -- the same
  dialog part 4 first built its own one-line commit prompt from, before
  part 9 replaced that one with a multi-line editor; a branch name is
  always one line, so there is no reason to reach for the heavier widget
  here. Confirming it runs plain `git branch <name>`, not `git switch -c
  <name>`: creating a branch and switching to it are two separate gestures
  this panel already keeps apart (switching is Enter, part 7), and leaving
  the checked-out branch untouched is the safer default -- nothing stops a
  user who does want to switch from pressing Enter on the freshly created
  entry right afterward. A blank name (confirming the dialog without typing
  anything) is rejected with a toast before any git command runs, the same
  way part 4's blank commit message was.
- **Delete/F8** on the branch list deletes the branch under the cursor,
  after a Yes/No-style confirmation (`vtui.ShowMessageOn`) -- the same
  "confirm, then act on button 0" shape
  `internal/plughost/permissions_ui.go`'s own Revoke button already uses
  for its own irreversible action. It runs `git branch -d <name>` (the safe
  delete), never `-D`: git itself refuses `-d` when the branch has commits
  not reachable from any other ref ("not fully merged"), and that refusal
  surfaces here as git's own message, the same way every other failed
  command in this plugin already reports its own. The branch checked out
  right now is refused outright, with its own toast, before a confirmation
  dialog even opens -- git would refuse it anyway, and there is nothing
  useful about asking the user to confirm an operation that cannot succeed.
- Both reload the branch list afterward (so a newly created or deleted
  branch shows up or disappears immediately), the same point-in-time
  refresh F5 already gives this list.

## What is deliberately not here yet

Everything else the ticket asks for: staging/unstaging a single hunk within
a file (needs f4#613's diff widget to pick the hunk, and f4#613 is itself
still open), and a per-file diff for a multi-file commit in the log view.
Each is its own atomic follow-up part of f4#659, not this one.

## Design notes

- Every git invocation goes through `execGit` (a package-level var, the
  same substitution seam `plugins/multiarc/tools.go`'s `runToolIn` is), with
  `-c core.quotepath=false` always prepended (`runGitIn`) so a non-ASCII
  path comes back as literal UTF-8 instead of git's octal-escaped quoting.
  A path containing a literal double quote or backslash is a known,
  unhandled edge case for this first version.
- `parseStatus` (`status.go`) is a pure function over `git status
  --porcelain=v2 --branch`'s stable, documented output shape, tested with
  fixed sample text rather than a real repository -- the same split
  `internal/textdiff`'s algorithm keeps from its caller (f4#613).
- Ahead/behind and upstream tracking (`# branch.ab`/`# branch.upstream`) are
  parsed by git but not surfaced by this panel yet; `parseStatus` does not
  keep them either, on the principle that an unused, untested field is the
  wrong thing to carry around. A later part that wants them only has to add
  the two lines back.
- `headFileContent` (`diff.go`) runs `git show HEAD:./<path>`, never bare
  `HEAD:<path>` -- per `gitrevisions(7)`, a path after the colon is resolved
  relative to the repository's top level unless it starts with `./` or
  `../`, in which case it is resolved relative to the current directory
  instead. Since every git invocation here runs with `dir` (a possible
  subdirectory of the repository) as its working directory, and
  `entry.Path`/`entry.OrigPath` come from `git status` reported the same way
  relative to that same `dir`, the `./` prefix is required for the two to
  agree on what the path means.
