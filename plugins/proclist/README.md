# ProcList

A live list of running processes, shown as a panel. This is f4#312 part 1 of
4: Linux only, view-only.

## What v1 does

- Registers a `vfs.PanelProvider` (`internal/plughost/panel_providers.go`) --
  f4's first in-tree consumer of that API. Opening it replaces the active
  panel slot with a `vtui.Table` listing every readable `/proc/[pid]` entry:
  PID, name, resident memory (`Mem`) and `CPU%`.
- Refreshes itself roughly twice a second from a background goroutine, in
  the same "ticker + `RunOnUI(vtui.FrameManager.Redraw)`" pattern
  `internal/app/arkanoid.go` uses for its game loop. All table mutation
  still happens on the UI goroutine; the background goroutine only reads
  `/proc` and hands the result to `RunOnUI`.
- CPU% is computed the way `top` does: the delta of `utime+stime` (from
  `/proc/[pid]/stat`) between two samples, divided by the elapsed wall time
  -- not divided by the number of CPUs, so a busy multi-threaded process can
  read above 100%.
- Sortable by any column (click a header, or Ctrl+Alt+R -> click; default
  sort is CPU% descending) and has type-to-filter (`Table.QuickSearch`)
  across all columns.
- Reachable from the plugin menu/command palette ("Open ProcList", added
  automatically by `RegisterPanelProvider`) and from **Commands -> Process
  list** / **Ctrl+Alt+R**.

## What it deliberately does not do yet

- **Any other platform.** `Supported()` reports `false` outside Linux, and
  neither the panel provider nor the Commands menu row/hotkey are
  registered there. Part 2 of the plan adds Windows
  (Toolhelp32/`NtQuerySystemInformation`) and macOS/\*BSD (`sysctl`), all
  without WMI -- see `plugin.go`'s package comment for why WMI itself does
  not port.
- **Process management.** No kill (FAR3's F8), no priority/nice (FAR3's
  Shift-F1/F2). That's part 3.
- **Anything else FAR3's ProcList shows**: PPID, thread count, command line,
  start time, environment, open file handles, WMI performance counters,
  remote/network process lists. Some of those are candidates for part 4
  (the FAR3 F3 details view); WMI-perf-counters and the handle viewer are
  Windows/NT-specific and not planned to be ported at all.

## Layout

- `plugin.go` -- `Plugin` (`Init`/`Close`/`GetName`), registers the panel
  provider. No build tag: it defers to `Supported()`.
- `collector_linux.go` -- `/proc` reading and parsing (`Supported() == true`
  here): PID enumeration, `/proc/[pid]/stat` (name, utime, stime) and
  `/proc/[pid]/status` (VmRSS), and the two-sample CPU% delta.
- `collector_other.go` -- the `!linux` stub: `Supported() == false`, and a
  `newProcListPanel` that only exists so this file has the same shape as
  `collector_linux.go`'s (it is never actually called; `Plugin.Init` checks
  `Supported()` first).
- `panel_linux.go` -- the panel itself: table columns, formatting, numeric
  sort comparator, and the refresh ticker.
