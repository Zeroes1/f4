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

## What is deliberately not here yet

Everything else the ticket asks for: diffing a changed file (reusing
`internal/diffview`/`internal/textdiff`, f4#613 -- see that ticket for how
the scope was split between the two), staging/unstaging a hunk or a whole
file, commit (with an editor for the message), log, and branch
switching/creation. Each is its own atomic follow-up part of f4#659, not
this one.

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
