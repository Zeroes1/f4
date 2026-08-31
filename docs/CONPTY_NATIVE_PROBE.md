# Native OpenConsole probe

`tools/conptyreconcile` contains a standalone Windows probe for the pinned
`OpenConsole.exe` described in [CONPTY_NATIVE_TEST.md](CONPTY_NATIVE_TEST.md).
It is deliberately below the f4 integration layer: it does not parse VT,
render a screen, or compare against a second terminal model. Its output is the
observed contract that the future f4 adapter must consume.

## Run

From a Windows checkout with Go installed:

```text
go run ./tools/conptyreconcile -probe -probe-report native-openconsole-probe.json
```

The probe downloads the pinned Windows Terminal MSIX bundle into
`%LOCALAPPDATA%\f4\native-conpty\1.12.10983.0\`, verifies the nested x64
package and `OpenConsole.exe` version/SHA-256, and reuses the verified cache on
later runs. `-probe-host C:\path\OpenConsole.exe` is available for an already
extracted package, but the same identity checks still apply.

The command starts the verified host through its native ConDrv server/client
path and `--headless` arguments. It attaches the probe executable as the child
through `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE`, so this is not the operating
system's default `NewPTY` host. Three sessions (`80x25`, `1x1`, and `121x40`)
run a deterministic workload containing markers, Unicode, combining marks,
tabs, SGR/erase/cursor movement, OSC title, alternate-screen transitions, and
a 257-character line. Resizes are sent while the child writes short chunks,
including a `1x1` minimum and a wide `121x40` window.

## Artifacts

The JSON report records:

- pinned host path, version, SHA-256, package URL, process architecture and
  working directory;
- the exact child payload (`expected_input`), command line and dimensions;
- exit code, resize timestamps, marker presence, raw-output SHA-256 and every
  observed output event with timestamp, dimensions and read chunk bytes.

Each session's unmodified ConPTY stream is also written beside the report in
`native-openconsole-probe.json.sessions\<width>x<height>.raw` (the directory
name follows the report path). No output is dropped or normalized.

The report is intentionally suitable as an implementation handoff: f4 should
feed the recorded raw stream through its real `PanelsFrame`/terminal-session
path and compare logical marker/payload order and screen/history snapshots.
The probe itself only establishes host identity, native plumbing, timing,
resize interleaving, lifecycle and the bytes that actually came from this
specific OpenConsole build.

Linux and other non-Windows builds fail explicitly; they cannot close the
native gate.
