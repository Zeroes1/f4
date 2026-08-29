# Plan: a source-faithful `conptyreconcile.exe`

This is the plan for the standalone ConPTY probe. It is not a plan to change
f4, and it does not restore the deleted implementation or the old field
results.

## 1. Scope and finish line

The deliverable is one self-contained Windows executable, built from scratch;
no file or code from the deleted `tools/conptyreconcile` is transplanted.
`conptyreconcile.exe` answers one question:

> Can the complete logic needed by the probe be moved into f4 as a literal
> port of Microsoft's pinned OpenConsole code, with no unresolved host
> assumptions?

The work is complete only when all of these are true:

1. The executable contains a complete audit trail for every behavior that
   affects the result.
2. Every behavior present in the selected Microsoft source is ported at
   transpilation level, without an equivalent reimplementation or a guessed
   semantic.
3. A mock made from that port passes the complete test matrix and 300
   independently generated, recorded seeds.
4. The same matrix and the same recorded seed list pass against the pinned
   `OpenConsole.exe`.
5. The executable verifies that it is talking to the pinned binary and fails
   closed if that binary is absent or has the wrong version or hash.
6. The source, documentation, executable, and verification report are pushed
   to GitHub on `main`.

After that publication and report, work stops. There is no f4 integration,
no unrelated cleanup, no additional host investigation, and no further action
until a new instruction.

## 2. Source and host rules

The sole normative host is the pinned OpenConsole described in
`docs/PINNED_CONSOLE.md`:

- x64 `OpenConsole.exe`, version `1.12.220408003-release1.12`;
- source tag `v1.12.10982.0`;
- source commit `e9b4e2e18fb1b9cee6839969d42cd0f95d228926`;
- SHA-256
  `14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e`.

The maintainer's Windows 11 host contributes exactly one retained observation:
some conhost versions emit a logical line longer than the window as one
uninterrupted ConPTY run. No other observation from that host is a rule for
the implementation or a test oracle.

No other conhost version, inbox fallback, Windows Terminal instance, Linux
pty, Wine run, or historical capture is a target, oracle, or acceptance
criterion. Old dumps, old seed results, and conclusions tied to a different
host or source revision are discarded rather than restored or replayed.

The executable must locate the pinned host explicitly, verify its identity,
and refuse to substitute any other executable. The old fallback to the
machine's inbox `conhost.exe` must not exist.

## 3. The port audit

Before writing the new mock, create a source ledger for the pinned commit.
For every copied function, data structure, constant, table, state transition,
and externally visible byte, record the exact Microsoft file and symbol. The
ledger must distinguish only these two categories:

- **MS port:** syntax may change to Go, but control flow, state, ordering,
  defaults, and edge cases remain the same as in the pinned source.
- **Documented reconstruction:** allowed only when the pinned source is truly
  unavailable for that exact behavior. It must cite a documented protocol
  fact or an explicitly recorded pinned-host observation, be isolated from the
  MS port, and be marked as a remaining boundary.

There is no third category. In particular, “equivalent”, “reasonable”,
“usual conhost behavior”, and “the tests need it” are not permissions to add
logic. An unresolved source or documentation gap blocks the finish line; it is
not silently filled in and it is not deleted.

The audit covers, as applicable to the probe's execution path, the pinned
OpenConsole buffer, row/cell flags, text writes, cursor movement, delayed EOL,
wrap and reflow, scrolling and row eviction, VT output, parser state, Unicode
width/grapheme handling, and the Windows ConPTY launch/resize path. Every
source file used by the mock carries the pinned commit and the ledger entry.

## 4. What the new standalone probe must contain

The deleted version is used only as a checklist of capabilities that must not
be lost. Its old code, old mocks, old expected results, and old host findings
are not reused.

### 4.1 Pinned-host session and capture

- Start only the verified adjacent pinned `OpenConsole.exe` through the
  Windows ConPTY path.
- Capture the live stream and resize repaint without assuming read boundaries.
- Record timestamps, sizes, resize events, byte counts, markers, host identity,
  and the seed in a replayable log and raw dump.
- Wait for semantic child markers and bounded lifecycle completion; fixed sleeps
  are not completion evidence.
- Keep live output and repaint/frame bytes distinguishable even when they are
  interleaved in one read.
- Make one default invocation run the complete gate automatically. Diagnostic
  flags may select one seed or stage, but a user must not have to orchestrate
  several programs or remember hidden parameters.

### 4.2 Port-backed mock

The mock is a thin harness around the pinned MS port. It must not hand-compose
the byte grammar from captures. It must exercise the ported behavior for:

- screen/buffer writes, cursor and autowrap state, delayed EOL, CR/LF and
  absolute cursor movement;
- wrapped logical rows, width changes during output, scrolling, circular
  buffer eviction, and a frame beginning partway through a logical line;
- wide cells, double-byte padding, combining and variation sequences,
  surrogate pairs, grapheme clusters, and the pinned Unicode width table;
- repaint/frame generation after a resize, including same-size, height-only,
  and rapid resize calls wherever the pinned source path makes them observable;
- alternate-screen transitions and screen-buffer switching that belong to the
  pinned execution path;
- incremental parsing of arbitrary byte chunks, including chunks ending in
  UTF-8, CSI, OSC, BEL/ST, or other supported escape sequences;
- frame/live reconciliation by stream order and cursor state, never by
  content-only matching;
- conservative behavior: no invented text, no lost text, no accidental merge
  of distinct equal lines, and no split of a genuine wrapped line;
- idempotent reprocessing and stable replay of a failed seed.

The probe may use a second independent session of the same verified pinned
host to read back a visible slice, but it must never use a different host as a
reference.

The standalone terminal pipeline that existed in the deleted version is also
retained inside the probe where it is needed to validate the result: logical
line mirror, re-wrap at a new width, visible slice, row accounting, coordinate
mapping, and scroll bounds. These are probe checks only; they are not f4
integration and must not modify f4 code.

Features whose only purpose was to integrate with f4's UI, f4's process
watcher, or f4's geometry decision are explicitly outside this task.

## 5. Test plan

Every test must state whether its expected behavior comes from the pinned MS
port, from a permitted documented reconstruction, or from an independent
input invariant. A green test that merely compares two copies of a guessed
model is not evidence.

### 5.1 Port and model tests

Cover the complete audited source path, including empty and one-column
buffers, exact-width and exact-multiple lines, delayed EOL, cursor movement,
scrolling, row eviction, resize reflow, wide-cell padding, Unicode width and
grapheme transitions, parser state persistence, and all supported VT controls
used by the probe.

### 5.2 Adversarial generated cases

The generator must include, and the assertions must preserve in order and
count:

- CJK and other wide characters;
- ambiguous-width characters under each pinned setting;
- combining marks, variation selectors, surrogate pairs, and composed
  grapheme sequences;
- bidirectional text patterns;
- repeated identical characters and many identical complete lines;
- blank lines next to exact-width lines;
- widths `N-1`, `N`, and `N+1`, exact multiples, width one, and large widths;
- genuine long wraps, lines that exactly fill one or several rows, and lines
  that narrowly miss those boundaries;
- random byte/text patterns, control sequences, cursor moves, erases, tabs,
  title changes, and end markers;
- output that continues while a frame is being emitted;
- a frame whose top begins in the middle of a wrapped line;
- every chunking of the same stream, including one-byte chunks and boundaries
  inside multibyte characters and escape sequences.

Assertions must prove that equal text does not cause distinct lines to be
merged or dropped, that no text is invented, that only source-justified
padding is consumed, and that the result is invariant under read chunking.

### 5.3 Fuzzing

Fuzz the incremental parser, cursor/grid feed, width and row operations,
frame/live splitter, reconciliation, report generation, and every boundary
between them. Required properties include no panic or hang, bounded resource
use, no lost printable input, no invented output, safe handling of malformed
controls, and idempotence where applicable.

Fuzz failures must print a reproducible input and seed. A failure is fixed in
the source-faithful port or documented reconstruction; it is never hidden by
weakening the assertion.

Every failed scenario must identify the seed, stage, first mismatch, relevant
dimensions and byte offset, and the paths of its log and dump. Garbage input
and a missing frame must produce a controlled failure report, not a panic or a
false pass.

### 5.4 Real commands on the pinned host

Run actual Windows commands through the pinned session, including a recursive
`dir` workload and commands that emit explicit progress/end markers. Validate
arrival, ordering, line accounting, parser behavior, and the final report;
the command output is not replaced by a fixture.

During the real output, issue rapid width changes at synchronized points and
also while bytes are actively arriving. Exercise repeated, identical,
narrowing, widening, and `N-1/N/N+1` sizes. Log the complete resize timeline
and make incomplete output or a missed frame a hard failure.

### 5.5 Three-hundred-seed gate

Run exactly 300 independent randomized scenarios from a recorded seed list.
Each seed must cover generated content, randomized chunk boundaries, output
and frame interleaving, and resize timing/widths. Print every seed, preserve a
per-seed log/dump, and return a non-zero exit status on the first failing
scenario while still reporting the complete summary.

The same recorded seed list and the same assertions are then run against the
verified pinned `OpenConsole.exe`. A seed not run on the pinned host is not
counted as passed.

## 6. Approaches retained for the decision record

The earlier approach analysis is retained as analysis, not as a source of
semantics and not as permission to implement every option.

1. **Read the pinned ConPTY stream and repaint.** This is the execution model
   tested by the probe. The cursor/grid model and the source port determine
   what bytes mean; line-boundary rules are not inferred from CRLF alone.
2. **Maintain a logical mirror and re-wrap locally.** This is the standalone
   probe's reconciliation check. It is valid only to the extent its behavior
   is sourced from the pinned port or an explicitly documented reconstruction.
3. **Use resize-triggered frames as an oracle.** The probe may use a frame only
   when the pinned source/session proves which resize produced it and what
   state it represents. A wide resize is not an oracle by assumption.
4. **Bundle the pinned OpenConsole.** This is the reproducibility mechanism
   for the executable, with identity verification and no inbox fallback.
5. **Fish+ and an API proxy.** Bundling OpenConsole does not automatically turn
   Fish+ into a transparent OpenConsole-to-Windows ABI. Such a design would
   have to proxy the exact native process, console, input, output, resize, and
   synchronization interfaces. Compiling OpenConsole to WASM would not supply
   that Windows ABI; it would still require a native adapter. These are
   documented options for a later decision, not hidden assumptions in this
   probe and not additional implementation scope here.

No behavior is selected from another conhost to fill a gap. No other conhost
is tested or investigated.

## 7. Publication gate and handoff

Before publication, perform a final checklist review against this file and
the source ledger:

- no deleted historical code or stale field result was restored;
- no source revision other than the pinned commit appears in the port;
- no host other than the pinned OpenConsole appears in the acceptance run;
- every mock behavior is an MS port or a cited documented reconstruction;
- all listed old-probe capabilities still have a test or an explicit
  out-of-scope reason;
- all 300 mock seeds pass;
- the same 300 seeds and the real-command/resize matrix pass on pinned
  OpenConsole;
- the executable fails closed on a missing or mismatched host;
- the GitHub commit contains the plan, source, test data/report, and the
  downloadable executable.

Only after every item is true: commit to `main`, push to GitHub, provide the
exe link and the verification report, and stop.
