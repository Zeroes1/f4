# ProcList

A live list of running processes, shown as a panel. f4#312: part 1 shipped
Linux, part 2 (this update) added Windows and macOS. View-only throughout.

## What it does

- Registers a `vfs.PanelProvider` (`internal/plughost/panel_providers.go`) --
  f4's first in-tree consumer of that API. Opening it replaces the active
  panel slot with a `vtui.Table` listing every process the platform's
  collector could read: PID, name, resident memory (`Mem`) and `CPU%`.
- Refreshes itself roughly twice a second from a background goroutine, in
  the same "ticker + `RunOnUI(vtui.FrameManager.Redraw)`" pattern
  `internal/app/arkanoid.go` uses for its game loop. All table mutation
  still happens on the UI goroutine; the background goroutine only collects
  and hands the result to `RunOnUI`.
- CPU% is computed the way `top` does on every platform: the delta of
  cumulative kernel+user CPU time between two samples, divided by the
  elapsed wall time -- not divided by the number of CPUs, so a busy
  multi-threaded process can read above 100%. The unit that cumulative time
  arrives in differs per platform (Linux: `/proc/[pid]/stat`'s `utime`+
  `stime` in clock ticks; Windows: `GetProcessTimes`' kernel+user
  `FILETIME`, 100ns units; macOS: libproc's `pti_total_user`+
  `pti_total_system`, nanoseconds) but the delta-over-wall-time math is the
  same collector-side logic on all three (`compareSamples`/`sample` in
  `panel.go`, format helpers alongside it).
- Sortable by any column (click a header; default sort is CPU%
  descending) and has type-to-filter (`Table.QuickSearch`) across all
  columns.
- Reachable from the plugin menu/command palette ("Open ProcList", added
  automatically by `RegisterPanelProvider`) and from **Commands -> Process
  list** / **Ctrl+Alt+R**, on every platform `Supported()` reports `true`
  for.

## Platform support

- **Linux** (`collector_linux.go`): every readable `/proc/[pid]` entry --
  `/proc/[pid]/stat` for name and CPU ticks, `/proc/[pid]/status` for
  `VmRSS`.
- **Windows** (`collector_windows.go`): a Toolhelp32 snapshot
  (`CreateToolhelp32Snapshot`/`Process32First`/`Next`) for PID and image
  name, `GetProcessTimes` for CPU and `psapi.dll`'s `GetProcessMemoryInfo`
  (loaded the same hand-written-struct way `internal/sysinfo/mem_windows.go`
  loads `GlobalMemoryStatusEx`) for working-set size. No WMI, matching the
  owner's decision (f4#312) not to port FAR3 ProcList's WMI-backed metrics.
- **macOS** (`collector_darwin.go`): `sysctl kern.proc.all`
  (`golang.org/x/sys/unix.SysctlKinfoProcSlice`) for PID and name --
  `kinfo_proc`'s own memory/CPU fields are widely known to be stale
  BSD-compatibility leftovers on modern XNU (`x/sys/unix`'s own `KinfoProc`
  even names the relevant embedded fields `Dummy`) -- so CPU% and memory
  instead come from libproc's `proc_pidinfo(PROC_PIDTASKINFO)`, loaded via
  `github.com/ebitengine/purego` the same way `cpu_windows.go` loads
  `pdh.dll`'s counters: no cgo, with `readDarwinTaskInfo` refusing to trust
  the result unless `proc_pidinfo` reports back exactly `sizeof(proc_taskinfo)`
  bytes filled.
- **FreeBSD/NetBSD/OpenBSD**: still the `collector_other.go` stub
  (`Supported() == false`) after part 2. Each exposes its own, differently
  laid out `kinfo_proc`/`kinfo_proc2`, `golang.org/x/sys/unix` defines none
  of them, and -- unlike Windows/macOS -- none of the three has a
  GitHub-hosted runner at all, not even for a one-off `sandbox.yml` manual
  check (its OS choices are limited to ubuntu/windows/macos). A hand-rolled
  struct layout for any of them would only ever be cross-compile-checked by
  CI, never executed, which is a real risk for code that reads raw kernel
  memory by hand: see `collector_other.go`'s own comment.
- Everywhere else (illumos, solaris, dragonfly, js/wasm, ...): the same
  `collector_other.go` stub.

## What it deliberately does not do

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
- `collector_linux.go`, `collector_windows.go`, `collector_darwin.go` -- one
  real collector per supported platform, each defining the same
  package-private shape: `Supported`, `sample`, `collector`/`newCollector`,
  and `(*collector).collect`.
- `collector_other.go` -- the fallback stub for everything else:
  `Supported() == false`, and a `newProcListPanel` that only exists so this
  file has the same shape as the real collectors' (it is never actually
  called; `Plugin.Init` checks `Supported()` first).
- `panel.go` -- the panel itself: table columns, formatting, numeric sort
  comparator, and the refresh ticker. Entirely platform-agnostic (it only
  names the collector's shape above), so it builds and runs on every
  platform that has a real collector (`//go:build linux || windows ||
  darwin`) without change from part 1.
