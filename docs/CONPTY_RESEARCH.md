# ConPTY and line structure: the research, the algorithm it justifies, and how it held up

This is the one-document version of a question that took two Windows builds,
three probes, eight field runs and nine documents to answer: **how does a
terminal running inside a ConPTY tell a line that wrapped from a line that
ended, when ConPTY does not say?** Every step below states what was asked,
how it was asked, what the answer was, and how that answer is now pinned in
the test mocks so the code cannot drift from it silently. The last section
is the part that matters for anyone who inherits this: which of these facts
the reflow depends on, how it fails if one of them changes, and the switch
that turns it off.

The detailed ledgers this summarizes: `TERMINAL_LEDGER.md` (every finding,
numbered), `TERMINAL_CONPTY_FINDINGS.md` (the raw evidence and the chase),
`TERMINAL_REFLOW.md` (the algorithm as implemented), `WINCON.md` and
`WINCON_805_HANDOVER.md` (the picture overlay, which turned out to share a
root cause). Finding numbers (P6, A5, W8, ...) refer to the ledger.

## 0. The problem

f4 hosts `cmd.exe` in a pseudoconsole and renders its output itself. When the
window is resized, long lines should re-wrap the way they do in any modern
terminal. On Unix that is bookkeeping: the pty delivers bytes, the terminal
wraps them, and the terminal knows which of its rows are continuations. On
Windows the pseudoconsole (ConPTY) sits between the shell and f4 and *renders
the screen itself* into a VT stream, so what f4 receives is not the shell's
output but a description of a screen -- rows, cursor moves, erases -- and the
one thing that description does not carry is whether a row is the end of a
line or the middle of one.

The user-visible failure was simple: resize the window and history either
stays wrapped at the old width or vanishes.

## 1. What ConPTY does (measured, two builds)

Each step was a scenario in `tools/conptyprobe` (a Windows binary that
creates its own pseudoconsole, drives `cmd.exe` inside it, and records every
byte), run by the maintainer on 10.0.19045 and 10.0.22000.2538. The probe's
log is the evidence; the finding is what the log proved.

| Step | What we checked | How | What we found | Pinned in the mock as |
|---|---|---|---|---|
| 1 | Does ConPTY mark a soft wrap at all? | Print a line longer than the width; record the live stream. | No. On 19045 the wrap point carries a **hard CRLF** (P6). Full rows are padded to width; a short row is followed by `ESC[K`, a full one is not. | `fakeConPTY.print`: full rows padded, CRLF, `ESC[K` only after a short row. `TestFakeConPTYLiveStreamBreaksWithHardCRLF`. |
| 2 | Is that universal? | Same on 22000; read the bytes with a cursor model, not a CRLF split. | No. Within one build the live stream **sometimes has no terminator at all**: the line is written whole across the boundary, the terminal's autowrap breaks it, and ConPTY repositions with an absolute CUP afterwards (P11). A CRLF-only reading saw a 140-column row. | `fakeConPTY.printUnterminated`. `TestFakeConPTYUnterminatedLiveStream`, and `TestHintReadsBothLiveShapesAlike` proves the hint gives the same answer on both shapes. |
| 3 | What does a resize repaint look like? | `ResizePseudoConsole`, drain the frame. | A full-viewport repaint bracketed by `ESC[?25l` ... `ESC[?25h` (P7). On 19045 wrapped lines are written as rows joined by CRLF; on 22000 the logical line is written **whole** and autowrap places it, only the last row ends in CRLF (P12). On 22000 the frame opens with the XTWINOPS size report `ESC[8;rows;cols t` (P14). | `conptyBehaviour` per build: `repaintBreaksWrappedLines`, `sizeReport`. `TestFakeConPTYRepaintShapeMatchesTheBuild`, `TestFakeConPTYRepaintsOnEveryResizeCall`. |
| 4 | Is the `ESC[K` clause reliable? | Print a hard-broken line *exactly* the width. | It arrives as a full row + CRLF with no `ESC[K` -- indistinguishable from a wrap (P13). The hint is wrong once in W lines, and only in that direction (a hard break read as a wrap). | `TestFakeConPTYExactWidthLineIsAmbiguous`. |
| 5 | Does ConPTY keep scrollback we could ask for? | 30 lines into a 12-row console, then widen; look for scrolled-off lines in the repaint. | **No** (P16). The repaint covers the viewport and nothing above it. Whatever scrolled off is gone from ConPTY's side; only f4 has it. | `fakeConPTY.trimToHeightLocked`. `TestFakeConPTYKeepsNoScrollback`. |
| 6 | Do the documented flags change any of this? | `RESIZE_QUIRK` (0x2) and passthrough (0x8) vs 0, byte-for-byte diff. | Accepted, no effect on either build (P8-P10). | Not modelled: nothing in f4 depends on them. |
| 7 | Can we *make* ConPTY tell us the line structure? | Widen the pseudoconsole to a very large width for one frame; read the repaint. | Yes: the wide repaint carries every wrapped line **rejoined** (P15), and the narrow frame after restoring the width shows where each line breaks. Two frames, one answer per row. This is the oracle. | `TestFakeConPTYWideResizeRejoins`; the oracle tests in `reflow_oracle_test.go` run the whole exchange against the fake. |
| 8 | When is it safe to borrow the console for that? | Watch the title ConPTY forwards. | `cmd.exe` sets the title to `... cmd.exe - <command>` while a command runs and drops the suffix when idle (P18, P19). That is the "no child is running" gate the oracle waits for. | `fakeConPTY.title`. `TestFakeConPTYTitleBusySignal`. |
| 9 | Which resizes repaint, and how does a repaint differ from output? | Height-only resize; resize to the size ConPTY already has; compare the bytes with a command batch. | **Every** `ResizePseudoConsole` call repaints (6.15), including a call for an identical size (6.16) -- and that one carries **no** size report. Both were guessed wrong first and cost the field runs of §3. | `fakeConPTY.SetSize` repaints on every call; same size ⇒ no `ESC[8;`; a repaint goes to home after the hide, a command batch (`printBatch`) positions below it. `TestFakeConPTYRepaintsOnEveryResizeCall`, `TestAbsorbNeverTakesCommandOutput`. The probe measures all three (`repaint.*.frame/size_report/starts_at_home`). |

| 10 | Can a repaint be told from a program redrawing its own screen? | Compare a resize repaint with `cls`, and with a full-screen program (f4 inside f4) taking the alternate screen. | **Not by looking at it.** Both open with the hide and go home; the shape is necessary and not sufficient. What separates them is context f4 already has: ConPTY owes exactly one repaint per `ResizePseudoConsole` call, and a full-screen program is on the alternate screen, where f4 does not re-wrap at all (6.19). | `TestNestedFullScreenProgramKeepsItsFrames`, `TestClsStyleRepaintIsNotDroppedWithoutAResize`. |

The line that connects all ten: **ConPTY describes a screen, not a
document**. It will repaint the screen it holds, at whatever size it is asked,
faithfully -- and it holds nothing else. Every hard part of this work followed
from that one sentence: the line structure has to be inferred or provoked,
the history has to be f4's own, and a repaint that arrives has to be judged
by what f4 asked for rather than by what it looks like.

## 2. The algorithm, and which fact each part rests on

Given the above, the design has three parts, in the order they were adopted.

**The hint (rests on steps 1, 2, 4).** While reading the live stream, a row
that is full to the width and is *not* followed by `ESC[K` is marked as a
wrap; the terminal's own autowrap marks the no-terminator shape the same way.
This is cheap, runs on every row, and is wrong once per W lines in one known
direction (P13). It is what carries the history in practice (W8, O12).

**The oracle (rests on steps 3, 5, 7, 8).** At a settled prompt with no
command running, borrow the console: resize it very wide, read the rejoined
repaint, restore, read the narrow one, and stamp wrap flags into
`GridHistory` only for rows that align exactly with both frames and the
local journal. Every stamp is checked before it can change history, and a
pass that cannot prove the two frames describe the same text stamps nothing
("display changed during the pass"). Measured on 22000: 25 boundaries in one
pass, none stale, and **zero disagreements with the hint** wherever it could
check (W8).

**Ownership of the viewport during a resize (rests on steps 3, 5 and 9).**
This is the part that was missing, and the reason the feature looked broken
for seven field runs after it worked. f4 re-wraps its own grid from history
when the width changes; ConPTY then repaints its screen -- which has no
history -- and that repaint used to land on top, replacing recovered rows
with blanks. Two rules fix it: tell ConPTY nothing when the size did not
change (so it sends nothing), and drop ConPTY's resize repaint, recognised by
its shape and not by its timing: the cursor hide, then the size report where
the build sends one, then the move to **home** -- a repaint redraws from the
top, a batch of command output positions below home. Recognising it by
timing, or by the cursor hide alone, took real output during a `dir` and lost
it (6.18): ConPTY hides the cursor around every batch it writes. The shape
rule cannot take output (`TestResizeDuringCommandDoesNotEatOutput`,
`TestLongScrollingDirWithResizesLosesNothing`) and takes a late or split
repaint just the same (`TestLateResizeRepaintIsStillAbsorbed`).

A fourth fact turned out to be load-bearing for the history itself, not for
ConPTY: the history must be bounded in **logical lines**, not rows, because
the same text is more rows at a narrower width, and a row cap evicts on every
narrowing step (6.11, 6.12; `TestGridHistoryBoundIsWidthIndependent`).

## 3. What the field runs taught, in the order they happened

This is recorded because the *method* cost more than the bug, and the next
person to chase something similar should not repeat it.

1. The oracle was reported not to work. It worked; the runner's verdict
   treated a safely-rejected pass as a failure (6.6).
2. The scrollback was reported not to come back. The run was in `probe`
   mode, which re-wraps nothing by design, and the log did not say so. The
   mode line now names every switch it sets (6.7).
3. In the default mode, the log showed 197 repaint frames landing after f4's
   re-wrap. First cause found: the repaint overwrites (6.8). Absorbing it
   changed nothing on screen.
4. Instrumentation inside the re-wrap, one number per run, for four runs.
   Two of the counters could not observe their own subject and returned
   zeros that read as evidence (6.14). The re-wrap was innocent throughout:
   `TestReflowLosesNothingAcrossEveryResizeShape` now says so in one second.
5. The history cap was found and fixed (6.11, 6.12). Still no change on
   screen.
6. Logging *around* the re-wrap, all at once: which view, what ConPTY was
   told, what it sent back with its declared size checked, what was drawn.
   The next log showed characters preserved inside every pass and lost only
   between passes (6.15).
7. Cause two: height-only steps let the frame through (absorber gated on
   width). Fixed. Still no change.
8. Cause three, twelve lines of log: three resize events for a size the view
   already had, each still calling `ResizePseudoConsole`, each answered by a
   repaint **without a size report** that nothing recognised (6.16). Fixed.
   Confirmed working.
9. Careful testing of that build found two more: a late repaint landing
   (only a few rows shown until the next resize) and -- the one that matters
   -- a resize during `dir` eating the listing. Both were the absorber keyed
   on timing and on the cursor hide, which every ConPTY batch carries. Both
   were reproduced on the mocks *before* the fix, and the fix recognises a
   repaint by its shape (6.18).
10. That shape rule was necessary and not sufficient: a program clearing the
   screen and repainting from home matches it, and one such program is f4
   inside f4's own terminal. Two conditions fixed it -- a repaint is dropped
   only when ConPTY owes one, and never on the alternate screen (6.19).
   Written after a reviewer pushed back on the claim that full-screen
   programs were out of scope. They are not, and the claim was wrong.
11. A review before the next field run, reading the code rather than the
   notes about it, found four ways the absorber could still lose bytes --
   a repaint coalesced with output in one read, a frame with no close, a
   debt raised without a call, a clamp too low for a slow ConPTY. All four
   got a failing test first, then a fix (6.21). None had reached the field.

Two things would have shortened this to one run: asking how the symptom was
reproduced (a corner drag interleaves width, height and same-size steps, so
every hypothesis was tested against a log where the innocent path ran
constantly); and the end-to-end test that now exists,
`TestCornerDragKeepsTheScrollback`, which drives the real resize path against
a fake that repaints on every call, on both builds.

## 4. How robust is this, and what is the fallback

**What the reflow assumes about ConPTY**, and what happens if each assumption
breaks on some build:

| Assumption | If it changes | Consequence | Detected by |
|---|---|---|---|
| A resize repaint opens with the cursor hide, [size report], then home (P7, P14, 6.18). | A build repaints from somewhere other than home, or stops hiding the cursor. | The absorber takes nothing; the frame lands after f4's re-wrap and overwrites it -- the 6.8 symptom, visible, not destructive. It can never take output: nothing that is not a repaint matches the shape. | `REFLOW_FRAME ... diverted=false` in the log; `repaint.*.starts_at_home=no` in the probe. |
| A full row without `ESC[K` is a wrap (P6). | A build starts erasing after full rows too. | The hint marks nothing; history re-wraps as if every row ended a line. Wrong shape, no loss. | `REFLOW_ORACLE ... where hint and oracle disagree` becomes nonzero. |
| The wide repaint rejoins lines (P15). | A build clamps the width or keeps wrapping. | The oracle aligns nothing and stamps nothing -- by design it cannot stamp what it cannot prove. The hint carries on alone. | `REFLOW_ORACLE ... nothing stamped` on every pass. |
| The title carries the busy suffix (P18). | A shell or locale without it. | The oracle never finds a safe moment and never runs. Hint only. | No `REFLOW_ORACLE` lines at all. |
| ConPTY keeps no scrollback (P16). | A build starts keeping it. | Harmless: f4 keeps its own and ignores what the repaint has above the viewport. | -- |
| Exactly one repaint per `ResizePseudoConsole` call (6.19). | A build answers a resize with silence, or with two frames. | Silence: the owed count lingers and one later home-repaint is misread -- one visible frame, clamped so it cannot accumulate. Two frames: the second lands after f4's re-wrap and overwrites it, the 6.8 symptom. | `REFLOW_SUMMARY ... repaints owed` staying above zero. |

The pattern is deliberate: every part of the reflow fails *toward the hint*,
and the hint fails toward "no re-wrap", never toward lost text. Content loss
requires something outside these assumptions -- which is exactly what
happened, and why the `chars` figure in `REFLOW_WRAP` exists.

**Backward compatibility.** Microsoft's history with ConPTY is the relevant
prior: the 19045 → 22000 change (P12, CRLF-joined rows becoming
autowrap-placed whole lines) did not break the design because the design
reads a cursor model rather than terminators. The size report (P14) appeared
between builds and the reflow does not depend on it. The behaviours it does
depend on -- the frame brackets, `ESC[K` after a short row, no scrollback --
are how ConPTY has rendered since 1809 and are what Windows Terminal itself
relies on. A change there would be visible in every terminal on Windows, not
only in f4.

**Earlier and later builds.** Nothing below 19045 has been measured. ConPTY
exists from 1809 and its renderer is the same code lineage, but "same
lineage" is a hope, not a finding. 24H2/25H2 are equally unmeasured. For both,
the probe is the instrument and the ledger's O4 is the open item; the
`conptyBehaviour` table in the mock is where a third build's answers go.

**What a user's log shows without any of this.** Three lines, budgeted so a
drag costs a handful rather than hundreds: `REFLOW:` at startup names the mode
and every switch it set; `REFLOW_ABSORB:` reports the first few repaints of a
burst and every fiftieth after, with the resize and owed counts;
`REFLOW_SUMMARY:` on shutdown and every fiftieth child resize gives mode,
resizes, repaints absorbed and owed, oracle passes, and the history's rows and
characters. Between them they answer, from a `--debug` log alone, every
question each field round trip in §3 was spent on: whether the feature was on,
whether ConPTY was resized, whether its repaints were recognised, whether the
oracle ever ran, and whether the history is still there (6.20).

**The conservative switch.** `[Terminal] WindowsReflow = off` in the config
(or `F4_WIN_REFLOW=off` in the environment, which wins) returns the Windows
terminal to Horizontal Preservation: no hint, no oracle, no absorber, the
resize repaint applied as ConPTY sends it. That asks nothing of ConPTY beyond
what every build has done, and is the right answer on a build where the
`REFLOW_*` log lines show any of the assumptions above failing. `hint` is the
middle setting: no oracle passes, no console borrowed, wrap guesses only.

## 5. The same root cause, next door

The picture overlay for classic conhost (#805) was investigated in parallel
and turned out to fail for a reason of the same shape: something f4 did not
own was repainting or freezing on top of its output. There it was the console
window's input queue, coupled to f4's by a cross-process child window (F7,
measured in the field as a frozen console, F22), and the fix was the same
kind of fix -- stop depending on the other process: a top-level layered
window with no parent and no owner, only ever *read* from the console (F23).
Under Windows Terminal the picture was never being erased at all; the
terminal was simply not being asked whether it could draw (F13, F14; fixed by
reading the window class and DA1). `WINCON.md` has the full account.

## 6. What is still open

- Portability to builds other than 19045 and 22000 (O4). The probes in the
  issue threads are current: `f4probe.zip` (this document's section 1,
  automated, now including the same-size and height-only resize steps) and
  `f4imgprobe.zip` (the overlay and sixel questions, eight answers from a
  person).
- Reading the XTWINOPS size report to tie a late frame to its resize (O9).
  Two `STALE` frames were seen in one run; with the absorber covering every
  resize they no longer reach the display, but they say the size ConPTY lays
  out for can lag f4's by a step.
- The tracker of the new overlay under minimize, occlusion and foreground
  changes has not been exercised on a live f4 (Q6, Q7).
- One deliberate misreading remains, recorded rather than fixed: a `cls`
  issued in the same breath as a resize can have its repaint counted as the
  one ConPTY owed. It costs one visible, recoverable frame and never output.
- Whether a build exists whose resize repaint does not start at home. The
  probe now records `repaint.*.starts_at_home` precisely so this can be
  answered without another round trip.


## 7. Verdict: abandoned. Do not come back to this.

After eleven field runs the Windows reflow -- the `ESC[K` hint, the
wide-resize oracle, and the repaint absorber in all three of its forms -- is
removed from the codebase. This section is written so that nobody, including
the author of the next clever idea, rebuilds it.

**What the last two runs showed.** With every fix of §3 applied, a resize
during a `dir` still corrupted the Terminal Log: duplicated rows, tails of
lines placed at the column where ConPTY's buffer had them, blank stretches.
The stream explained it (6.22): ConPTY's output after a repaint is a delta
against that repaint. Gating the absorber on "idle" moved the failure rather
than removing it, and the run after that (duplicated rows, corrupted data)
was the proof. Every fix in this file made the symptom rarer; none made it
impossible, because none could.

**Why it cannot work.** The design put two owners on the same rows. ConPTY
owns its viewport and re-renders it, from a buffer that holds nothing above
the screen (P16), sending only deltas against the last frame it believes the
terminal displayed (6.22). f4 owned a re-wrapped history and a viewport
composed from it. Where the two met -- the seam -- there is no identity for a
row: nothing in the stream says "this row is that row", so every join was a
guess (the hint, the oracle's alignment, the absorber's shape rule), and each
guess was right until the next resize arrived while something was in flight.
A mechanism that is correct only when nothing is happening is not a
mechanism.

**What every other terminal on Windows does.** WezTerm, Alacritty and Windows
Terminal stand *outside* ConPTY: they are the terminal, ConPTY is the
renderer, and they display the frame it sends. The viewport reflows because
ConPTY reflows it. Their scrollback is kept as written, and the duplicated
rows after a resize that this project fought are a known, accepted ConPTY
limitation there too. f4 was the only program trying to be an application
inside ConPTY *and* a terminal with its own re-wrapped history at once.
Nobody else is in that position because it is not a position.

**What remains, deliberately.** ConPTY owns the viewport; on a resize its
repaint is applied as sent (Horizontal Preservation, the behaviour before
this work). A wrap flag is set only by the terminal's own autowrap, never
guessed from the stream. The history is bounded in logical lines rather than
rows (6.12) -- that was a real bug, independent of all this, and stays fixed.
The probes, the fake ConPTY and the ten measured findings of §1 stay as
documentation of what ConPTY does; they are the reason this section can be
written with confidence instead of regret.

**If the scrollback under Windows must ever re-wrap**, the only honest routes
are outside this design: an upstream ConPTY that re-renders more than the
viewport, or a row identity in the stream that ConPTY does not provide today.
Not a fourth condition on an absorber.

## 8. Roads not taken: the alternatives to §7, assessed

Written in the last minutes of the session that closed §7, deliberately
before any code, because §3 shows what happens when code comes first.

**A. Bypass ConPTY for programs that do not need a console.** Rejected too
fast the first time. `cmd`, `dir`, batch files and anything on the Win32
console API need ConPTY. But WSL programs, PowerShell 7 with VT output, and
the Go/Rust/Python utilities people actually run write bytes to stdout and
have never heard of a Windows console. Started with **pipes** instead of a
pseudoconsole, they hand f4 a plain VT stream, and f4 is their terminal the
way xterm is `ls`'s: the wrap is f4's own, the history is f4's own, and
there is no frame, no delta and no seam -- the whole of §7 does not apply.
Two real problems, both tractable: without a console `isatty` is false, so
colour and paging need `TERM`/`COLORTERM`/`FORCE_COLOR`, and WSL programs
should be launched through `wsl.exe` where the Linux side gives them a real
pty anyway; and the class of a program cannot be known in advance, only
chosen by kind -- `wsl.exe`, PowerShell 7+, known VT tools by pipe; `cmd`,
`.bat`, anything else by ConPTY. A shell that later spawns a VT program stays
on ConPTY, and there reflow is what it is for everyone. **The most promising
road, and cheap to try for `wsl.exe` alone.**

**B. A console scraper of f4's own, instead of ConPTY.** What winpty was:
read the buffer of a hidden console with `ReadConsoleOutput`, diff, render.
It gives full control of synchronisation -- no deltas against a frame f4 did
not show -- at the price of seeing only the buffer (no scrollback, but f4 is
the scrollback), flattening the attributes of VT programs, missing alternate
screen transitions, and polling. Real, and months of work, not a session.

**C. A window of height zero: everything goes straight to history, render
the last rows cut to the real width.** ConPTY cannot be that (minimum height
one, and it repaints a buffer rather than emitting lines). But combined with
B, or with ConPTY kept **very wide** (4000 columns, as the oracle did) and as
tall as the window, every logical line arrives whole and ConPTY never wraps
at all; f4 cuts to the window width itself, and the wrap question disappears
because nobody but f4 ever answers it. Full-screen programs -- f4 inside f4,
editors -- need a console of the real size, so they need detecting (the
alternate screen, or the child's console mode) and a real-size console when
they run. **Cheap to measure with one probe run before any code: does a
4000-wide ConPTY of window height deliver lines whole and repaint sanely.**

**C, measured (2026-08-28).** `tools/conptycprobe` ran on Windows
10.0.22000.2538 with an outer terminal of 120x30 and a 4000x30 ConPTY. It
emitted two ASCII lines of 184 and 3968 characters from an `@echo off` batch,
then resized only the height through 4000x29, 4000x30, 4000x31, and 4000x30.
Every initial and repaint check reported `whole=true`, `split=false`,
`rows=1`; a post-resize line did too, and the probe returned `PASS`.

This confirms the premise of C on this build: a very wide ConPTY can carry
these lines without answering the horizontal-wrap question, and its
height-only repaint remains coherent. It does not yet test f4's wide-console
integration, rendering/cutting to the real width, scrollback ownership,
alternate-screen or full-screen programs, width changes, or another Windows
build. The first run was a false negative caused by interactive command echo;
the probe was corrected to run the payload from an `@echo off` temporary batch
before this PASS.

**C, width-aware command follow-up (2026-08-28).** The Linux companion probe
was run in `/dev/pts/2`, with an outer size of 153x36, and compared real PTYs
of 80, 120, and 4000 columns. The result separates two classes that must not
be conflated. `ls -1` stayed at 142 one-entry-per-line records at every
width, and `git branch --column=never` stayed at 41 records. Human-oriented
column modes did react strongly: `ls -C` produced 142 lines at 80 columns,
71 at 120/153, and **one line of 3658 characters at 4000**; Git's
`branch --column=always` produced 21, 14/11, and **one line of 1439
characters**, respectively. The small `git diff --stat` fixture stayed at
two short lines at all widths, so it did not exercise a width decision.

This makes the practical risk real but bounded: a 4000-column ConPTY does
not damage ordinary newline-delimited output, but it materially changes
common human-facing listings and tables. It also disproves using a write to
the very last cell as the only detector: in this run the width-aware commands
made decisions based on 4000 without reaching column 4000 (`ls -C` stopped at
3658 and Git at 1439). The saved log was complete and ended with `END`; the
earlier apparent hang was a test-runner/pager issue, not a ConPTY result.

The Windows command probe records and then clears `DIRCMD`, invokes `dir` with
`/-p`, and starts PowerShell with `-NoProfile -NonInteractive`. The follow-up
run below showed that this is not sufficient to make a large native `dir`
listing non-interactive, so the probe also bounds its native fixture. The
tested PowerShell formatting cmdlets do not request `Out-Host -Paging` and
have no pager by default.

**C, Windows command follow-up (2026-08-28).** The Windows run was on Windows
11 Pro `10.0.22000.2538`, PowerShell `5.1.22000.2538`, with an unredirected
`120x30` window and a `120x9001` screen buffer. `TERM`, `COLUMNS`, and
`LINES` were empty, and Git was not installed. The log therefore confirms a
real Windows console run, but does not identify the outer host as ConPTY;
that distinction still needs a run from inside f4.

The 142-entry fixture exposed a second harness problem. `dir /w`, `dir /d`,
and `dir /b` all emitted repeated `Press any key to continue . . .` prompts at
screen boundaries, even though the recorded `DIRCMD` was empty and the probe
used `/-p`. They eventually returned zero, but they were not safe as
unattended measurements. The Windows probe now keeps the 142-entry fixture
for PowerShell and uses a separate, height-bounded `dir` fixture (10 entries
in this 30-row run), so the native commands complete without a keypress wait.

PowerShell's `Format-Wide`, `Format-Table`, `Get-Process | Format-Table`, and
default `Out-String` all completed without paging. The table output did
truncate the long `FullName` column to the available width, confirming that
PowerShell formatting is width-aware. The Russian filename appeared as
mojibake in the transcript under OEM code page 850; that is a separate log
encoding issue, not a width result.

**D. Own the console server: build conhost into f4.** The one road that
attacks the root rather than working around it.

Every other option in this file gropes for the wrap flag from outside.
Nothing gropes for it because it is missing -- it *exists*, and has since the
first console: `TEXT_BUFFER`'s rows carry `wrapForced`, set when a write ran
off the right edge, and `TextBuffer::Reflow` already re-wraps a whole buffer
using it. That is exactly the fact §7 says the stream cannot supply. **It is
not exported by any public console API** -- not `ReadConsoleOutput`, not
`GetConsoleScreenBufferInfoEx`, not the VT stream ConPTY emits. Only the
process that owns the buffer can read it. That is the real reason winpty,
WezTerm and this project all ended up guessing: everyone is outside the one
process that knows.

So be that process. Two facts make it plausible rather than a rewrite of
Windows:

- **conhost is open source.** `microsoft/terminal`, `src/host`, MIT. The
  buffer, the wrap flag and the reflow are all there, written and debugged by
  the people who own the format.
- **Windows already supports substituting the console server.** Since 1809
  conhost takes `--server <handle>`; ConPTY starts its own conhost exactly
  that way. The mechanism f4 would use is the one Microsoft uses, not a hack
  around it.

What f4 would gain: the wrap flag as a fact rather than an inference, and a
reflow it does not have to invent. What it costs: a C++ component in a Go
program (cgo), building `src/host` outside its own solution, three
architectures, and ownership of a console server's correctness and security.
Comparable in size to B, and strictly better in kind -- B still reads a
buffer someone else already wrapped.

ReactOS reached the same place from the other side, reimplementing `condrv`
and the CSRSS side of the protocol. That is the fallback shape if the
supported route closes, and it is genuinely a rewrite; the `--server` route
is not.

**The one-evening question to answer before any of this**: does `src/host`
build standalone, outside the `microsoft/terminal` solution, and does the
resulting binary serve a console when launched with `--server`? If yes, the
road is engineering. If no, what remains is the ReactOS-scale fork, and that
is a different decision.

Note what this does *not* fix, so nobody is surprised later: text that
arrives already wrapped by someone else -- a remote host over ssh, where the
far side's pty made the layout decision -- stays wrapped as received. Owning
the local console server gives f4 the wrap flag for locally produced output.
Nothing gives it the flag for a layout that was decided on another machine.

**D2. Proxy the console server instead of replacing it.** D says own the
server; this says stand in front of it. The seat is the same handle --
`\Device\ConDrv\Server` -- but f4 need not implement a console: it can hold
the endpoint, forward the traffic to the real conhost, and read what goes
past. **No C++ in the build, no console server of our own to get right.**

What goes past is better than what D would read, not worse. The wrap flag
is conhost's *conclusion*; the messages carry the application's *intent* --
"client 4 called `WriteConsoleW` with these 185 characters" -- and the
buffer width at that moment is known. One logical line over two rows is then
a fact, not an inference. Everything the project has tried so far looked at
the screen *after* the decision: the `ESC[K` hint read a rendered frame
backwards, a scraper (B) reads a buffer someone already wrapped, and the
oracle provoked a second frame to compare. Here the decision has not
happened yet.

It also supplies the detector that measurement C could not build: the
messages include `GetConsoleScreenBufferInfo`, so f4 sees *when a program
asks for the width* -- exactly the width-aware programs (`ls -C`, Git's
column mode, PowerShell's `Format-Table`) that made C unworkable, and which
could not be recognised by watching for a write to the last column.

**How stable is what this depends on?** Asked concretely, because "they
might change it" is not a risk assessment.

- The console *architecture* has changed three times in thirty years:
  CSRSS-hosted (NT 3.1 through XP), conhost over ALPC (Windows 7, 2009),
  and conhost over the `condrv.sys` driver (Windows 8.1, 2013). That is
  roughly once a decade, at the granularity of a Windows release family, not
  a monthly update.
- Within the ConDrv era it has been stable for **thirteen years**. The IOCTL
  set FireEye documented from a Windows 10 driver in 2017 is the same set
  named in `dep/Console/condrv.h` in `microsoft/terminal` today:
  `READ_IO`, `COMPLETE_IO`, `READ_INPUT`, `WRITE_OUTPUT`, `ISSUE_USER_IO`,
  `DISCONNECT_PIPE`, `SET_SERVER_INFORMATION`, `GET_SERVER_PID`.
- `conhost --server <handle>` has existed since 1809 (2018) and is how
  ConPTY starts conhost to this day. Microsoft depends on it in shipping
  code, which is the strongest guarantee available for something
  undocumented.
- The headers are **in the open-source repository**, vendored under
  `dep/Console` (`condrv.h`, `conmsgl1.h`, `conmsgl2.h`, `conmsgl3.h`), MIT.
  They are not *documented* -- microsoft/terminal#10463 asked for that in
  2021 and it is still open -- but they are published and they are what the
  shipping conhost is built against.

So the honest reading: the interface is undocumented but not volatile. The
risk is not a change every release; it is that when a change does come, no
compatibility promise applies and nothing announces it. That is a real risk
and it is bounded by one thing -- a proxy that fails should fall back to
plain ConPTY, not break the terminal.

**The measurement, and the probe that takes it.** `tools/condrvprobe` (a
Windows binary, no admin, changes nothing) answers three questions and
records the build, `condrv.sys` version and `conhost.exe` version they are
answers about:

1. Can an ordinary program create `\Device\ConDrv\Server`? If not, D2 is
   closed before it starts.
2. Does the driver deliver API messages, and what are their first bytes? The
   layout is undocumented, so the bytes are *recorded* rather than
   interpreted -- the only evidence of stability is the same bytes on
   another build.
3. Does the system conhost accept a handle f4 created, launched the way
   ConPTY launches it? If yes, the seat is real and the rest is forwarding.

**D2, first measurement (2026-08-28, Windows 10.0.22000, `condrv.sys`
6.2.22000.71, `conhost.exe` 6.2.22000.2538).** Two of the three questions
answered, and the two that matter answered yes.

- **The seat is available unprivileged.** `\Device\ConDrv\Server` was
  created by an ordinary user process. The obstacle that would have closed
  D2 before it began is not there.
- **The system conhost accepted a handle f4 created.** Launched as
  `conhost.exe --server <our handle> --headless -- cmd.exe`, it took the
  endpoint and kept running on it. So f4 can hold the seat and hand the work
  to the real conhost: no C++ console server of its own.
- **Question 2 failed on a probe bug, not a refusal.** `READ_IO` returned
  `ERROR_INVALID_FUNCTION` because the probe built its control codes with
  `FILE_DEVICE_CONSOLE = 0x8000` instead of `0x50`. The arithmetic is
  checkable against published numbers -- FireEye's 2017 analysis names
  `0x50000F` and `0x500013` for input-read and output-write, which are
  functions 3 and 4 under device `0x50` -- so the codes are now
  `READ_IO = 0x500006`, `SET_SERVER_INFORMATION = 0x50001F`. The probe also
  did not announce itself as a server first: `SET_SERVER_INFORMATION` hands
  the driver the event it signals, and without it a read has no meaning.
  Both are fixed; question 2 is unanswered, not answered no.

**What the run already settles about the shape of D2.** Questions 2 and 3
cannot both succeed in one process. If conhost serves the endpoint, conhost
reads the messages; if f4 reads them, conhost is not there to do the work. So
D2 is not "listen alongside" -- it is a genuine proxy: f4 holds one endpoint
facing the client, holds a second facing conhost, and forwards messages and
replies between them, reading what passes. That is more work than watching,
and it must be written down as such before anyone plans on the cheap version.

**A side note worth keeping.** `condrv.sys` reports file version
**6.2**.22000.71 -- a Windows 8 era resource, unchanged through Windows 11.
Weak evidence, but it points the same way as the interface history above:
this driver is not being rewritten.

**D2, second measurement (2026-08-28, same machine).** With the control
codes fixed, the probe got further and turned one of its own failures into
the most useful result so far.

- **f4 can take the server role.** `SET_SERVER_INFORMATION` was accepted: the
  driver took the probe's event. Holding the endpoint and *being* its server
  are different things, and both are now confirmed.
- **The server role is exclusive, measured rather than argued.** On the first
  run conhost accepted our handle and ran on it. On the second it refused
  with `0x80070016` (`ERROR_BAD_COMMAND`). The only difference between the
  runs was that the probe had claimed the server role in between. So an
  endpoint has exactly one server, first claimant wins -- which settles the
  shape of D2 for good: **not "listen alongside", but a real proxy** with two
  endpoints, f4 facing the client and conhost facing f4, messages and replies
  forwarded between them.
- **Question 2 is still open, and again on a probe fault.** A plain
  `CreateProcess` gives the child *the parent's* console, so `cmd.exe` went
  to the probe's own console and nothing arrived at the new endpoint; the
  three-second silence proved nothing. A client lands on a particular server
  only if handed that server's console handles, which are opened as child
  objects of the server handle -- `\Reference`, `\Input`, `\Output` -- and
  passed as its standard handles. The probe now does that, reports the
  NTSTATUS of each open so a failure names itself, and asks the driver
  `GET_SERVER_PID` afterwards to say whether anything actually attached.
  Question 3 now runs on a fresh, unclaimed endpoint, since the old one
  cannot answer it once we are its server.

Three necessary conditions for D2 are met -- the endpoint is creatable
unprivileged, the server role is obtainable, and conhost will serve an
endpoint f4 created. The remaining question is the one that decides the
direction: do the messages arrive, and do they carry what §8 claims they do
-- the application's intent, before anything wraps it.

**D2, third measurement (2026-08-28).** The endpoint's child objects --
`\Reference`, `\Input`, `\Output` -- all opened with `STATUS_SUCCESS`, so
the endpoint is a genuine console with real I/O objects, and conhost again
accepted a fresh (unclaimed) endpoint. But `GET_SERVER_PID` reported **no
client attached**: `cmd.exe` took the probe's own console, not ours.

The reason is worth recording, because it raises D2's price. A console client
does not pick its console from its standard handles. It attaches during
startup inside `kernelbase`, using the console handle in its inherited
`RTL_USER_PROCESS_PARAMETERS` -- which an ordinary `CreateProcess` fills with
*the parent's* console. Handing a child our `\Input` and `\Output` therefore
cannot redirect it. Windows itself does this from the kernel: `AllocConsole`
asks ConDrv to create the server process (the `0x500037` ioctl in the public
analyses). So being the server is not enough; f4 must also be able to *place*
clients on its endpoint, which is a chunk of what `AllocConsole` does.

The probe now tries every known route in a single run rather than one per
trip: standard handles (the failed baseline, kept so a report shows it
failing next to the others), the documented
`PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE` attribute, and having the probe attach
to the endpoint itself via `\Connect` so that a plain `CreateProcess` passes
that console down. It also captures a reference ConPTY session -- the bytes,
and what `mode con` reports as the width inside it -- so that whichever route
lands, the next step has something to compare console-API text against
without another measurement.

**D2, fourth and final measurement (2026-08-28): the direction is closed.**
All three attachment routes were tried in one run, and none put a client on
our endpoint. The decisive line is from route C:

    open \Connect: NTSTATUS 0xC0000008   (STATUS_INVALID_HANDLE)

The driver will not open the *client* side of a console relative to a server
handle. That side is not ours to create: the kernel makes it, when
`AllocConsole` asks ConDrv to create the process (the `0x500037` ioctl in the
public analyses). Route A failed as expected -- standard handles do not
choose a console. Route B "succeeded" at nothing relevant:
`CreatePseudoConsole` builds its own console with its own conhost inside, so
it was never pointed at our endpoint, and it is also what left an orphaned
console window on screen after the probe exited, since the pseudoconsole
outlived the child the probe killed.

So the tally for D2: the endpoint is creatable unprivileged (yes), the server
role is obtainable and exclusive (yes), conhost will serve an endpoint f4
created (yes) -- and f4 cannot place a client on it (no). Three of four, and
the fourth is the one that matters, because a console server with no clients
is furniture.

**Why the cheap version was an illusion.** D2's appeal was "do not
reimplement conhost, sit in front of it". But sitting in front requires
owning both sides of the conversation, and the client side is created by the
kernel on behalf of `AllocConsole`. To place clients f4 would have to drive
that undocumented ioctl itself -- that is, reimplement a piece of
`AllocConsole` on an interface with no compatibility promise, where a mistake
strands a console window on the user's desktop. The probe demonstrated that
failure mode by accident in its own last run. That is not a proxy; it is D
with worse guarantees and no C++ saved.

**What stands from the D2 work**, because it was not wasted:

- The seat is real and obtainable, and conhost accepts an endpoint f4
  creates. If direction D is ever built, f4 does not have to fight for the
  server role -- it can have it.
- The server role is exclusive, first claimant wins. Measured twice by
  control: conhost accepted the same handle before the probe claimed the
  role and refused it (`0x80070016`) after.
- The ConDrv control codes and the child-object names (`\Reference`,
  `\Input`, `\Output`) are confirmed working on 10.0.22000, and
  `condrv.sys` still reports a Windows 8-era file version, which is the
  strongest evidence available that this interface is not churning.

**A, and the ssh problem it has to answer.** If a user runs `ssh` from f4's
terminal rather than f4's own SSH client, the remote session inherits every
ConPTY limitation -- not because the far end is at fault (a Linux server
sends clean VT), but because `ssh.exe` is a *local* program and f4 gives it
its console. Under pipes that disappears: `ssh.exe` forwards bytes, f4 wraps
them, and the scrollback is f4's own. The catch is that `ssh.exe` learns the
window size from the console, and over pipes there is none -- so the far
end's shell would lay out for 80 columns unless the size can be passed some
other way. `tools/pipeprobe` asks that, along with the rest of what A needs,
in one run: what WSL, PowerShell 7, PowerShell 5 and cmd produce with no
console at all; whether colour survives when `TERM`/`COLORTERM` say it
should; whether `COLUMNS`/`LINES` are honoured for width; and whether
`ssh.exe` runs at all that way. One question needs a live server and is
asked of the tester directly: run `ssh <host> "stty size; tput cols"` from
f4 and from `cmd.exe`, and compare.

If `COLUMNS` turns out to be honoured, A covers remote work too and the
`ssh` case needs nothing special. If it is not, the honest options are a
session whose width is fixed at connect time, or offering to open `ssh …`
through f4's own SSH client -- an offer, like an IDE's "open in integrated
terminal", not a silent substitution.

**A, measured (2026-08-28, `tools/pipeprobe`, 10.0.22000).** The result is
weaker than the argument for A implied, and two of the four questions were
asked badly. Recorded as it stands, because the tester has run enough probes
for this issue and the remaining gaps do not need another one to be
described honestly.

| Candidate | Over pipes, no console | Reading |
| --- | --- | --- |
| `cmd /c dir`, `where.exe` | works, plain text | as expected; produces output but formats for 80 columns without a console, so it stays on ConPTY anyway |
| `cmd /c mode con` | nothing, fails | the signature of a program that genuinely needs a console -- so the split A relies on is measurable |
| **PowerShell 5.1** | **nothing, 0 bytes** | **needs a console.** It is the PowerShell present on most machines, and it cannot be routed to pipes |
| PowerShell 7 (`pwsh`) | not installed here | untested; it was A's strongest candidate |
| **WSL** | every call returned the help text | no distribution installed on this machine, so nothing was exercised |
| `ssh.exe` (OpenSSH 8.1p1) | runs: `-V` and `-G` both fine | it starts and reads config with no console, which is the necessary condition |

Two incidental findings worth keeping. `wsl.exe` writes **UTF-16LE** to a
pipe, not UTF-8 -- visible in the help text it emitted -- so any pipe route
for WSL needs a decode step. And the live-server check was asked wrongly:
`ssh host "command"` does not request a pty at all, which is why both runs
returned `Inappropriate ioctl for device` and an empty `$TERM`, identically.
The correct form is `ssh -t host "stty size; tput cols"`. That mistake taught
something anyway: **non-interactive ssh has no pty and therefore no wrapping
problem** -- only interactive sessions are at stake.

**Status of A: not disproved, not demonstrated.** Of the three programs it
was supposed to help, one needs a console after all (PowerShell 5), one was
untestable on this machine (WSL), and the third (ssh) is confirmed only to
start. A meaningful verdict needs a machine with WSL and `pwsh` installed and
one `ssh -t` comparison; until then A is a hypothesis with one necessary
condition met, and it should not be planned around.

**Where this leaves the list.** D2 is closed. D -- build conhost's own
`src/host` into f4 -- keeps its appeal precisely because Microsoft already
solved the client-attachment problem inside it, and it remains gated on the
one-evening question of whether `src/host` builds standalone (O16). A first
for `wsl.exe` and PowerShell 7 is untouched by any of this and is still the
cheapest real improvement. E is what ships.

**E. Make the Terminal Log the answer.** It already holds logical lines, so
reflow there is free, and it may be all users need from history under
Windows. The honest minimum, and what ships today while A through D are
decided.

Order to try. **A first for `wsl.exe` and PowerShell 7**, if a machine with
both can be found: there f4 would be the terminal and the wrap its own by
construction. The one measurement so far neither confirmed nor refuted it --
PowerShell 5 turned out to need a console, WSL was not installed, and ssh
was only shown to start (see "A, measured" above). **Then D2's probe run**, because it is the cheapest question with the
largest consequence: if f4 can hold the server endpoint and let the real
conhost do the work, the wrap stops being a guess and no C++ enters the
build. **D itself only if D2's seat turns out to be unavailable** -- it buys
the same fact at the price of a C++ console server. **C is demoted** to a special case inside A --
measured to work mechanically, but a 4000-column console changes what
width-aware programs *print*, not just how it is laid out (`ls -C` collapsing
to one 3658-character line), so it needs mode switching, and mode switching
is the guessing that §7 closed. **B only if D's route turns out to be shut**;
it buys winpty-grade behaviour, which is what Microsoft moved away from. **E
is what ships meanwhile.**

## 9. After §7: three ideas turned over, and where they land (2026-08-28)

Written in the "turning it over" stage, before any code, as a record of what
was considered and why only one of the three survives. Nothing here revives
the absorber; §7 stands.

**How conhost wraps, restated precisely.** By cells, never by words. A write
running off the right edge sets a per-row flag (`ROW::WasWrapForced` in
`microsoft/terminal` main) and continues on the next row; `TextBuffer::Reflow`
joins and re-cuts by that flag. Two details matter for anyone reading rows
from outside. A double-width glyph that will not fit in the last column moves
to the next row and the orphaned cell is marked `WasDoubleBytePadded`, so a
naive join would invent a space. And the deferred EOL: exactly W characters
with nothing following leaves the cursor pending and sets no flag -- the flag
appears only when the next glyph actually crosses. So "row is full" is not
"row wrapped", which is the root of P13 and is unfixable from outside.

**Idea 1: the minus-one probe.** Shrink the console by one column, compare
before and after, read the wrap points out of the difference. In a frozen
world with atomic snapshots it is sound: resolve full rows top-down against
two predictions (hard break: one trailing character; soft wrap: trailing
character plus a continuation), and the only unresolvable case -- a full row
with nothing after it -- is deferred EOL, which means "no wrap" anyway.
Three facts already in this file kill it. P16: ConPTY has no scrollback, so
the probe can only speak about the viewport, and the pain is the history
above it. 6.15/6.22: every `ResizePseudoConsole` repaints and moves the delta
base, so the instrument is the mechanism that caused §7's corruption. And the
child sees two spurious resize events. The wide-resize oracle got the same
answer in one comparison and died not from lack of information (W8: zero
disagreements with the hint) but from two owners on the same rows. The
minus-one probe inherits every cause of that death and adds a more fragile
alignment. **Rejected, and it is the same object as the oracle.**

**Idea 3: give up our own history, let ConPTY keep it and ask it to re-wrap
at 4000 on F4.** The framing is right and the mechanism is unavailable. A
"long enough buffer" cannot be arranged from outside: ConPTY's buffer *is*
the viewport (P16). "Always 4000" is C, already measured -- programs format
*for* 4000 (`ls -C` collapsing to one 3658-character line). "4000 on demand"
is the oracle, which is §7. What the idea actually asks for is a host that
owns logical lines and hands them over on request; it needs a host that can
be asked. That is idea 2.

**Idea 2: bundle a console host of our own -- but not ReactOS's.** ReactOS
reimplements the CSRSS/consrv era, before ConDrv and without ConPTY at all;
its host does not speak the protocol shipping Windows uses. The right donor
is OpenConsole: not a lookalike, but conhost itself (`src/host`, MIT). Three
things change the economics of direction D:

1. **O16 is answered by the industry, not by us.** `src/host` does build
   standalone -- the result is OpenConsole.exe, which Windows Terminal and
   wezterm have shipped for years; Alacritty took a PR that picks up a
   `conpty.dll` sitting next to the binary. Microsoft publishes a signed
   NuGet package (`Microsoft.Windows.Console.ConPTY`, 1.24.x line) containing
   both. arm64 availability in that package is unverified and is a probe
   question.
2. **No cgo.** `conpty.dll` exports the same `CreatePseudoConsole` /
   `ResizePseudoConsole` / `ClosePseudoConsole`; Go calls it through
   `NewLazyDLL` with a kernel32 fallback. D's headline cost -- C++ in f4's
   build -- disappears. This is D moved out of process; call it **D3**.
3. **The D2 measurements de-risk it.** D2 died on placing a client on an
   endpoint f4 serves. In D3 OpenConsole is the server and placement is the
   documented `PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE`: `winconpty.cpp` creates
   the server handle, opens `\Reference` from it via `CreateClientHandle`,
   and passes that reference in the attribute; the host is launched
   `--headless --width --height --signal --server`, with a side-by-side
   OpenConsole preferred over the inbox conhost. D2's first measurement --
   conhost accepting a handle f4 created -- already proved the scariest part.

**What stock bundling buys, and what it does not.** Pinning removes
cross-build variance (P6 vs P11/P12; O4), and that matters more now than when
this file was written: upstream removed VtEngine and translates Console API
calls straight to VT (#17510), and `ClosePseudoConsole` changed behaviour in
26100 (24H2) -- the inbox emitter is a generation the assumption table in §4
was never measured against. What stock does **not** buy is §7's root cause:
two owners of the viewport and no row identity. The absorber stays dead.

**Where a micro-fork earns its keep**, and it is exactly the two routes §7
called the only honest ones:

- **(a) Truth in the stream.** Guaranteed write-through of `wrapForced` rows
  including repaints; a private marker id on the repaint frame, so a repaint
  is recognised by its number rather than its silhouette; an event when a row
  leaves the viewport carrying its flag. The hint stops being an inference.
- **(b) The oracle as an API.** A retirement ring of logical lines inside the
  host (text plus `WasWrapForced` at the moment the row leaves the circular
  buffer -- the data is already in `TextBuffer`), and a request on a side
  channel: "give me the last N logical lines". This is idea 3 done properly:
  f4 re-wraps its history with no resize at all and the child sees nothing.
  Hundreds of lines of patch, not thousands, and a working prototype turns
  §7's "row identity in the stream that ConPTY does not provide" into an
  upstream pitch.

**Costs, honestly.** Carrying a fork (softened by pinning: ConDrv has been
stable ~13 years, and running one build's host on other builds is what WT and
wezterm do daily); two binaries per architecture; signing for a fork, where
the NuGet stock is already signed; MIT is compatible with BSD-3. It does not
help remote ssh -- a layout decided on the far side stays as received (the
same caveat as D). A for `wsl.exe`/PowerShell 7 and E as the baseline are
unaffected. Out of scope deliberately: `ReadProcessMemory` against another
process's `TextBuffer` offsets, and injecting hooks into clients (the ConEmu
route) -- both more fragile than D2.

**The one-evening probe, before any code.** Take the signed 1.24.x pair from
NuGet, bring up a session from Go through `conpty.dll`, and run the existing
`tools/conptyprobe` against the pinned host. One run answers three things:
whether the bundling mechanics work end to end; what the post-rewrite emitter
looks like -- there is a real chance write-through is now total and the hint
is exact on a pinned host, though the known post-rewrite issue list still
mentions a cooked-read prompt misaligning when a long line wraps, so the
neighbourhood is live; and it fills the third column of `conptyBehaviour`.
If stock is not enough, micro-fork (b) is the minimum step; (a) only if the
viewport's duplicated rows -- which even Windows Terminal accepts -- are also
to be fixed.

**Order after this section:** A (wsl/pwsh) unchanged and still cheapest; then
the D3 probe above, which is now the cheapest question with the largest
consequence, D2 having closed; D as originally written (C++ in the build) only
if bundling turns out to be impossible; C stays demoted; E is what ships.

## 10. Two corrected facts, the channel invariant, and direction F: the tall viewport (2026-08-29)

§9 closed idea 3 ("let conhost keep the history, widen to 4000 on F4") with
"a long enough buffer cannot be arranged from outside". That verdict was
wrong in an instructive way, and the correction opens the most promising
direction this file has. The philosophy these decisions serve is now §0 of
`TERMINAL.md`; this section is that philosophy applied to ConPTY.

### Corrected fact 1: the clamp is one-directional

`ApiRoutines::SetConsoleScreenBufferSizeImpl` (`src/host/getset.cpp`) rejects
a buffer *smaller* than the viewport and accepts a larger one; the
buffer==viewport coupling of pty mode lives on the viewport-resize path
(`SetViewportSize` via `_IsInPtyMode()`), not on an explicit client request.
So a taller-than-viewport buffer under ConPTY is not impossible -- a launcher
stub could set it before exec'ing the shell. It is merely *useless alone*,
because of fact 2. (Sizes are `COORD`, i.e. `SHORT`; a dimension equal to
`SHORT_MAX` is rejected, so the type ceiling is 32766. ConPTY's conhost is
headless, so the monitor-based window clamps are skipped.)

### Corrected fact 2: the real blocker was the channel, not the height

ConPTY's only output is a rendering of the **viewport**. Whatever the buffer
holds above it never enters the stream -- that is the mechanism behind P16,
and it is why a tall buffer *behind* a small viewport gains nothing, and why
the 4000-wide trick could only ever return one screen. Every dead end in
§§1-9 is this one sentence wearing different clothes.

### Direction F: make the viewport be the history

If the missing channel is "ConPTY only transmits the viewport", then make the
viewport everything: **create the ConPTY as tall as the type allows is
sensible (9,000-32,000 rows) and as wide as the real window.** f4 renders the
bottom slice; the rest *is* the scrollback. Then:

- **conhost owns the history**, in its own grid with its own wrap flags --
  idea 3 as originally stated, vindicated.
- **A width change is one `ResizePseudoConsole`:** conhost reflows the entire
  buffer with its own `TextBuffer::Reflow` and the repaint -- which covers the
  viewport, i.e. everything -- *transmits the re-wrapped history to f4*. The
  transfer channel that "did not exist" is the repaint itself.
- **F4's long lines are the original trick verbatim:** widen to 4000 for one
  frame, the repaint carries every logical line rejoined, read, restore.
- **§7's root cause dissolves by its own wording.** The fatal design put two
  owners on the same rows: f4 held a re-wrapped history *above* the viewport
  while ConPTY re-rendered the viewport, and the seam had no row identity.
  Here there is no "above". One owner (conhost) for the whole grid; f4 keeps
  a mirror and replaces it wholesale from each repaint. No seam, no absorber,
  no hint.
- **The channel invariant holds.** The child's output still reaches f4 as a
  VT byte stream -- truecolor arrives as SGR, nothing is flattened to
  `CHAR_INFO` -- and f4 still composes the screen. f4 additionally becomes a
  **geometry converter** (Hypothesis 1 of `TERMINAL.md` §0, applied to one
  more dimension): it shows the bottom slice and translates mouse and scroll
  coordinates between the visible window and the tall grid.

No fork, no bundled binary, no cgo, no undocumented interface. Stock ConPTY,
used at an angle nobody uses it at.

### What must be measured before any code (extend `tools/conptycprobe`)

1. **Ceilings.** Create 251x9000 and 251x32000; fill with history; record
   conhost RSS, creation time, and full-repaint time. (Post-#17510 the buffer
   may commit lazily; measure, do not assume.)
2. **Reflow at scale.** Fill thousands of rows, change width, verify the
   repaint carries the whole history re-wrapped, and time it. Debounce policy
   for interactive drags depends on this number.
3. **The alt-screen dance.** Full-screen programs need a console of the real
   size. Entering/leaving the alternate screen is an explicit protocol event
   in the stream (`?1049h/l`; conhost translates
   `SetConsoleActiveScreenBuffer` to the same) -- *not* the mode-guessing §7
   closed. On `h`: resize to real geometry; on `l`: resize back. Measure the
   race: a program that queries the size after `1049h` but before our resize
   lands lays out wrong once; most TUIs re-query on the resize event. Run
   vim, msedit, far, f4-in-f4.
4. **Main-screen programs that believe the window is 9000 rows tall.**
   `dir /p` and `more` page every 9000 lines (arguably fine); PowerShell 5's
   `Write-Progress` draws at the top of the *window* -- row 0, far above the
   visible slice. Enumerate, decide per case.
5. **`cls` semantics.** Clearing the screen clears the whole buffer, i.e. the
   whole history -- classic-console semantics, and the Terminal Log (E) keeps
   its own copy regardless. Confirm what the stream shows and that the mirror
   survives it.
6. **Row retirement past the ceiling.** Beyond 32k rows conhost's circular
   buffer evicts; rows leaving the top of the mirror carry whatever wrap
   state the stream gave them. Decide whether 32k is simply enough, or the
   Terminal Log absorbs the overflow.
7. **Old builds.** 19045-era emission differences (P6/P11) against a tall
   viewport; one run per build, third column of `conptyBehaviour`.

### Where this leaves the tree

**F first**: it is the only direction that satisfies idea 3 as stated, §7's
own post-mortem, and the channel invariant simultaneously, at zero packaging
cost. **D3 becomes the fallback**, not the plan: if F's probes kill it (an
unacceptable alt-screen race, a repaint cost with no debounce answer), the
bundled-OpenConsole channel from §9 buys the same facts with more machinery.
**A** stays worth it for `wsl.exe`/pwsh regardless (there f4 wraps by
construction). **E** stays the baseline and the safety net under `cls` and
retirement. **B stays closed** -- not by measurement but by the invariant:
`ReadConsoleOutput` hands f4 a composed grid, which is the overlay mode's
job, and the overlay mode already ships.

## 11. Remote sessions, restated under the invariant

The wrap question one hop out: what matters is not local-vs-remote but *who
is the terminal for the byte stream*.

| Route | Who lays out the output | Reflow |
|---|---|---|
| `ssh` typed into f4's terminal | `ssh.exe` is a local console client: the local ConPTY session lays it out; the far pty wrapped it first | no |
| f4's own SSH (NetFox/FISH+) | f4 speaks to the far side directly and is the terminal itself | **yes** |

Under direction F the `ssh`-in-terminal case inherits everything F gives
(tall history, width reflow of what the *local* session laid out), but a
layout decided by the far pty stays as received -- the caveat D already
recorded, unchanged.

**The offer, never a substitution.** When a typed command line begins with
`ssh`, f4 may offer -- once, dismissibly, like an IDE's "open in integrated
terminal" -- to open the session through NetFox instead, naming the gain:
real reflow, f4's scrollback, far2l extensions where the far side has them.
Declining runs `ssh` in the terminal exactly as typed. Silent rerouting is
banned: a terminal's one promise is that it runs what it was given.

**Windows on the far side.** FISH+ Step 17 already plans Windows hosts, and
its second form -- run `f4` itself remotely as the FISH+ helper -- is the
general answer: the remote f4 hosts the child in its own local session
(direction F applies *there*), and ships **logical lines, not a laid-out
screen**, over the FISH+ channel. The wrap is decided by an f4 on the machine
where the output was produced: the invariant, extended across the network,
and strictly better than any byte-stream terminal over `ssh` can do. Depends
on Step 17 and the Step 21 terminal channel; recorded so that "remote
Windows" is read as a reason to *extend* the invariant, never to weaken it
locally.

## 12. Direction F, first measurements (2026-08-29, `tools/termprobe`, 10.0.22000.2538)

The first run of the ladder on the machine that has answered every probe in
this file. Recorded in two parts, because a probe with bugs of its own
produced both kinds of line in the same log, and the distinction is the whole
value of the run.

### Measured, and load-bearing for F

| Question | Answer |
|---|---|
| Can a ConPTY be created at each ladder height? | **Yes, all nine, 125 to 32000 rows.** No refusal at any rung. |
| What does creation cost? | **24-104ms**, and *not* monotonic in height: 32000 rows created in 31-40ms, faster than 500. The buffer is plainly not committed up front. |
| Does history reach f4 at every height? | **Yes**: 150 of 150 marked lines at every rung, in 37-114ms including 32000. |
| What does the host cost in memory? | **~9.6MB** after filling, at the two rungs where attribution worked. |
| What does the child believe its geometry is? | **window == buffer == the full height**, at every rung: `120x32000 / 120x32000` at the top. |
| Can a client raise the buffer above the viewport under ConPTY? | **Yes** -- `SetConsoleScreenBufferSize` was accepted at every rung. |

The first three lines are F's premise and they hold: a viewport tall enough to
*be* the history is creatable, cheap, and carries what is written to it. That
is the fact §10 was built on, now measured rather than argued.

**The child-geometry line is risk 4, confirmed and no longer hypothetical.**
A program in a 32000-row ConPTY is told its window is 32000 rows tall.
`dir /p` and `more` will page on that; PowerShell's `Write-Progress` will draw
at the top of it. F must answer this, and the answer cannot be "programs
probably will not notice".

**A correction to §10's own proposal.** That section suggested detecting a
full-screen program by watching the buffer collapse to the viewport, since
`_IsAltBuffer()` makes `_IsInPtyMode()` true. Under ConPTY that detector does
not exist: the buffer already equals the viewport, and the measured alt buffer
came back the same size as the main one (`inside alt 120x1000` at height
1000). The alternate-screen detector has to be built from the stream or from
the console mode, not from geometry.

**One apparent ceiling that is not one.** At 32000 the buffer-raise was
refused with `The parameter is incorrect`. The child had asked for width 119
while the viewport was 120, and `SetConsoleScreenBufferSizeImpl` rejects any
size smaller than the viewport in either axis. That is the probe's own race
with the parent's resize, not a limit of the build.

### Not measured, despite appearing in the log

The run also produced `reflow 0B carrying 0`, `wide4000 0B` and
`alt h/l=false/false` at every rung. **These are not ConPTY findings.** The
probe's child waited on stdin between phases, that read did not block, and the
child raced through everything and exited within about a hundred
milliseconds; every resize afterwards was issued to a session that no longer
had a client. The zeroes are the probe measuring a dead child. The same fault
invalidates the `baseline` line of that run, which was read off one of those
empty repaints.

Two more lines are artefacts rather than results. Direction A's
`powershell 5.1 -> works` used `CREATE_NO_WINDOW`, which gives a child a
*hidden console* rather than no console, so it does not contradict the earlier
finding that PowerShell 5.1 needs one. And direction C's `dir /w` gave
identical output at 120 and 4000 columns because the fixture was a six-row
directory, too small to make any width decision.

All four faults are fixed in the probe (its README records them as traps);
the numbers they produced are discarded rather than reinterpreted.

### What the next run has to answer

Everything about the *width* axis, which is where F's actual claim lives:
does one `ResizePseudoConsole` make conhost re-wrap the whole tall buffer and
re-transmit it, at what byte cost and what latency, and does the 4000-column
frame rejoin the long lines. Plus the alternate-screen dance now that the
geometry detector is known not to work, and a width-aware fixture large enough
to react. Until then F has its premise and not its central claim.

**Method note, since §3 exists for this reason.** The run took 46 seconds of
its five-minute budget and no step hung. That is the supervised scheduler
working: earlier versions of this probe lost two field trips to a
`ClosePseudoConsole` that blocks until the host exits, and to fixed timeouts
that turned a dead session into five minutes of silence. Every step now runs
under a watchdog, independent heights run in parallel, results are printed as
they are produced, and a hard deadline prints the summary and exits. A probe
for this problem needs that as much as it needs its measurements.

## 13. Direction F, the width axis measured (2026-08-29, `tools/conptydump`, 10.0.22000.2538)

Section 12 established F's premise -- a tall viewport is creatable, cheap and
carries what is written to it -- and left its central claim untested. Three raw
dumps answer it, and the answer is better than the claim.

The instrument changed shape for this. `tools/conptydump` decides nothing while
it runs: it creates the pseudoconsole, starts a reader with no timeout, runs a
child, calls `ResizePseudoConsole` on a fixed schedule, and writes every byte it
receives to a file with the arrival time and a marked line at each call. Every
earlier probe here decided *during* the run whether a phase had finished, and
each of those decisions eventually produced a confident zero that was its own
bug. A dump cannot: the bytes are in the file or they are not.

### The central claim holds

**One `ResizePseudoConsole` makes conhost re-wrap and re-transmit the entire
buffer.** At 500, 2000 and 32000 rows the frame following a one-column
narrowing carried all 150 marked lines including `~F000001~`, the oldest --
not the viewport, not the tail, everything. That is the transfer channel §10
predicted and §7 said did not exist.

The frame has one shape at every height:

    ESC[?25l  ESC[8;<rows>;<cols>t  ESC[H  <content>  ESC[<lastrow>;1H  ESC[?25h

The size report (P14) is present and reports the *full* height. The frame
starts at home, as P7 said. The closing cursor position is the end of the real
content -- row 158 in a 2000-row buffer -- which incidentally tells a reader how
much of the buffer is live.

### The wrap flag arrives for free

Every line in the frame is terminated by `ESC[K CR LF`. The 608-character long
line, at a width of 119, arrives as **one unbroken run**: 600 characters
between its markers with no CRLF and no escape sequence at all. conhost emits
the logical line whole and lets the receiving terminal's autowrap place it.

So in the frame the wrap question is not ambiguous: `ESC[K CR LF` ends a
logical line, and everything between two of them is one logical line however
long. That is the fact this document has spent nine sections trying to infer,
provoke or fork its way to, delivered by an ordinary resize.

### The cost, measured exactly

    frame bytes = 3815 + 5.0 x buffer rows

Fitted on three points and accurate to one byte: 6314 at 500 rows, 13815 at
2000, 163816 at 32000, leaving 3814/3815/3816 as the content term. The five
bytes per row are literally `ESC[K CR LF` -- conhost erases and terminates
*every* row of the buffer, including the empty ones. The constant is a
*content* term and scales with what the buffer actually holds.

| Height | Frame | Latency |
|---|---|---|
| 500 | 6.3 KB | 16 ms |
| 2000 | 13.8 KB | 48 ms |
| 32000 | 164 KB | 470-520 ms |

### A hard ceiling: 4000 columns by 32000 rows wedges the host

At 500 and 2000 rows the widen to 4000 behaved like any other frame. At 32000
rows `ResizePseudoConsole` returned `S_OK` and then **nothing came back at
all** -- no frame, no further output, no EOF, and `ClosePseudoConsole` never
returned. 4000 x 32000 is 128 million cells; the sizes that worked are 8
million and 2 million. The limit is on the product and `tools/conptymatrix`
sweeps for its position.

## 14. The matrix probe, first results (2026-08-29, `tools/conptymatrix`, partial run)

`conptymatrix` replaces a list of questions with a grid -- fill x height x
width-op x line-shape x child -- because every probe before it covered the
questions its author happened to think of, and each time the finding that
mattered was one nobody had listed. Read from a partial run; the full log
supersedes this.

**Retirement is a number now.** At height 2000 with 3000 lines printed, the
oldest line surviving a reflow was **1042 of 3000**. conhost's buffer is a
plain ring: history depth equals buffer height and nothing above it is kept.
For F this sets the sizing rule -- pick the height for the scrollback depth
wanted, not for cheapness.

**Repeated resizes do not degrade.** `restore` 58169B/64ms, `narrow-again`
58151B/59ms, `restore-again` 58133B/46ms, each carrying 1994 lines with 1993
terminators and `longWhole=true`. The marker range drifts by three lines per
operation because the child keeps printing, which is the arithmetic working.

**A resize during live output is clean**: 58115B, same line count, same shape.
This is the situation that destroyed the absorber in §7. It cannot destroy
anything here, because f4 does not merge a delta into its own history -- it
replaces its mirror from a whole frame.

**P13 is confirmed as a real ambiguity.** The line emitted at exactly the
console width came back with **no `ESC[K` after it**, indistinguishable from a
wrap. This is the one hole in the otherwise-free wrap flag of §13: logical
structure is unambiguous except for lines whose length is an exact multiple of
the width, and the error is always in one direction -- a hard break read as a
wrap. No probe in this repository had ever emitted such a line before.

**The ceiling reproduces.** At 32000 the narrow resize worked (160654B, 463ms,
31993 terminators); the widen to 4000 produced zero bytes for every subsequent
operation and `ClosePseudoConsole` did not return. The probe's watchdog
terminated the host and continued, which is the behaviour that makes a sweep
possible at all.

## 15. Decision: f4 implements both paths

Two independent ways to obtain logical lines now exist, and **f4 will support
both**, not choose between them. They fail differently, which is the point.

**Path 1 -- the native frame.** Take the logical lines out of the repaint that
any width change already produces. `ESC[K CR LF` terminates a logical line;
everything between two terminators is one logical line whatever its length.
Costs nothing beyond a resize f4 was making anyway, works at every height
measured, and is exact except for the P13 case.

**Path 2 -- the wide frame.** Resize to a very large width for one frame and
read lines that are rejoined by construction. Costs one extra round trip, and
is bounded by the cell-product ceiling of §13, but it **resolves P13**: a line
that is an exact multiple of the narrow width is not an exact multiple of 4000,
so the ambiguity disappears. It is the check the native path cannot perform on
itself.

**How they combine.** The native frame is the default and carries ordinary
work. The wide frame is used where exactness is worth a round trip -- the F4
viewer or editor opening on terminal history, and as a periodic audit of the
native reading at an idle prompt, in the spirit of the old oracle but without
its fatal property: nothing here stamps flags into a history f4 also owns,
because with the tall viewport conhost owns the grid and f4 holds a mirror.

**Selection and fallback.** The width at which the wide frame is taken is
chosen against the measured ceiling rather than fixed at 4000: with the cell
product bounded, the safe width falls as the configured height rises, and a
height where no useful wide width exists simply does not offer path 2. If a
wide frame returns nothing -- the wedge signature of §13 -- f4 abandons that
path for the session and continues on the native frame. Neither path may
degrade into guessing: if both are unavailable, the terminal reflows nothing,
which is the §7 fallback and remains correct.

**What must be settled before implementation**, all of it inside what the
matrix already sweeps: where the cell ceiling actually lies, whether the
native frame's `ESC[K CR LF` reading holds for `cmd.exe` and full-screen
children as it does for ours, what the alternate screen does to both paths,
and the frame cost at a genuinely full buffer rather than a nearly empty one.

## 16. Every constant in this file is a setting

Nothing measured here is a law. `3815 + 5.0 x rows`, the 128-million-cell
ceiling, `ESC[K CR LF` as a line terminator, `ESC[8;rows;cols t` as the size
report, 4000 columns for the wide frame -- all of it is the behaviour of
**one build**, 10.0.22000.2538, and §4 already records that ConPTY changed
shape once between 19045 and 22000 and that the emitter was rewritten upstream
again after that (#17510). A build where any of these differs must be
correctable by editing the config, never by editing f4.

So the implementation rule is explicit: **no measured value from sections 12
through 15 may appear as a literal in the terminal code.** Each is a named
setting with the measured value as its default, read through the existing
`ini.GetString("Terminal", ...)` mechanism like every other f4 option, with
`F4_*` environment overrides where a tester needs to change one without
touching a file.

### The settings, and what each one is for

**Geometry.** `ConPtyHeight` is the tall viewport's row count, which §14 shows
*is* the scrollback depth -- history equals buffer height and nothing above it
survives. `ConPtyWidth` follows the real window by default but can be pinned.
`ConPtyMaxCells` is the product ceiling of §13: the width of a wide frame is
derived from it and the configured height rather than fixed, so a build with a
different limit needs one number changed. `ConPtyWideWidth` pins that width
directly for anyone who would rather set it than derive it.

**The frame grammar.** `FrameLineTerminator` (default `ESC[K CR LF`),
`FrameStartSequence` (the cursor hide), `FrameHomeSequence`, and
`FrameSizeReport` (the `ESC[8;` XTWINOPS pattern) are what §13's reading of
logical lines rests on. A build that terminates rows differently, or stops
sending a size report, is then a config change and not a rewrite. The parser
must treat these as patterns it was given, not as constants it knows.

**Behaviour and limits.** `ReflowPath` selects `native`, `wide`, `both` or
`off` -- the four states §15 defines, with `both` as the default and `off` as
the §7 fallback that reflows nothing. `WideFrameAudit` controls whether the
wide frame is taken periodically to check the native reading, and how often.
`ResizeDebounceMs` covers the interactive drag, whose cost §13 measured per
height. `FrameTimeoutMs` bounds a frame that never arrives -- the wedge
signature of §13 -- after which the wide path is dropped for the session.

**Diagnostics.** `ConPtyDumpPath`, when set, writes the raw stream the way
`tools/conptydump` does, so a user on an unmeasured build can produce the same
evidence these sections were written from without building anything.

### Why this is a requirement rather than a preference

The reflow of §7 was removed partly because a build change would have been
invisible until it produced corruption in the field. Everything above is
observable and adjustable from a config file, which is what makes an
unmeasured build -- 19045, 24H2, 25H2, whatever ships next -- a support
question instead of a release blocker. A single hard-coded `5` or `4000` puts
that property back in the bin.

## 17. The matrix, complete (2026-08-29, `tools/conptymatrix`, full run, 10.0.22000.2538)

Sixteen sessions across the full grid -- fill x height x width-op x line-shape
x child -- plus a ten-point sweep for the cell ceiling. This supersedes the
partial reading in §14 and settles every question §13 and §15 left open. The
raw dumps are the evidence; the numbers below are counted from them, not
judged.

### The cost model, corrected and now measured at a full buffer

§13 fitted `3815 + 5.0 x rows` on a nearly empty buffer and warned that the
constant was really a content term. It is, and the corrected model is:

    frame bytes = SUM(logical line lengths) + 5 x buffer rows

Both terms are now measured at three heights and two fill levels. The
per-row term is exact: an empty buffer costs `5 x rows + 654` at 500, 2000 and
32000 rows alike (3154, 10655, 160654 bytes), where 654 is the frame header
and never grows. The content term is linear in what the buffer holds: a full
buffer of ~21-character lines costs 23.8, 23.7 and 24.7 bytes per line at the
three heights -- the line plus its five-byte terminator.

| Height | Empty frame | Full frame | Latency, full |
|---|---|---|---|
| 500 | 3.1 KB | 14.3 KB | 20 ms |
| 2000 | 10.7 KB | 57.2 KB | 46 ms |
| 32000 | 160.7 KB | 949 KB | 609 ms |

A full 32000-row buffer costs just under a megabyte and 0.6 s per width
change. A full 2000-row buffer costs 57 KB and 46 ms, which is affordable
during an interactive drag with debouncing.

### History depth is exactly the buffer height

Measured at three scales by printing more than the buffer holds and reading
back the oldest surviving line:

| Height | Lines printed | Oldest surviving | Lines retained |
|---|---|---|---|
| 500 | 750 | 268 | 483 |
| 2000 | 3000 | 1042 | 1959 |
| 32000 | 40000 | 8070 | 31931 |

conhost's buffer is a plain ring: nothing above the top is kept, and the
retained count is the height less the rows consumed by wrapped lines. **This
is the sizing rule for direction F**: the configured height *is* the scrollback
depth, and it is chosen for the depth wanted, never for cheapness.

### The frame is self-describing

The count of `ESC[K CR LF` terminators in a frame is exactly the number of
logical lines it contains, and `buffer rows - terminators` is the number of
rows consumed by wrapping. The frames prove it directly: at height 500 the
narrow frame carries 493 terminators and the 4000-column frame carries 497 --
four more, because the over-wide line stopped occupying extra rows. Same
step at 2000 (1993 -> 1997). Nothing has to be inferred: the frame states its
own structure.

### The logical lines are readable from the frame, and only from the frame

Every frame in every case reported `longWhole=true`: the over-wide line
arrived as one unbroken run. But the **live stream** in the two overflow cases
reported `longWhole=false` -- while the buffer is scrolling, conhost splits
that line across its partial redraws.

So the rule for the implementation is narrow and firm: **logical structure is
read from a repaint frame, never from the live stream.** The live stream is for
display; the frame is for structure. This distinction did not exist in §13,
which had only non-scrolling fixtures.

### The wide frame is expensive, and the cost scales with height

| Height | Narrow frame | Wide (4000) frame |
|---|---|---|
| 500 | 9 ms | 144 ms |
| 2000 | 46 ms | 617 ms |
| 32000 | 473 ms | wedges the host |

The wide frame carries almost exactly the same bytes as the narrow one
(14068 vs 13998 at height 2000), so the time is not transfer -- it is conhost
rebuilding a 4000-column buffer. Six hundred milliseconds at a 2000-row
height makes the wide frame a deliberate, user-initiated operation and
nothing else. The periodic audit floated in §15 is withdrawn on this evidence.

### The cell ceiling, located

| Width x height | Cells | Result |
|---|---|---|
| 4000 x 8000 | 32 M | ok, 41.5 KB |
| 4000 x 12000 | 48 M | no output; host wedged |
| 4000 x 16000 | 64 M | no output; host wedged |
| 4000 x 24000 | 96 M | wedged, and `ClosePseudoConsole` never returns |
| 4000 x 32000 | 128 M | wedged, and `ClosePseudoConsole` never returns |

The boundary is between 32 and 48 million cells, and the failure has two
stages: first the host stops producing output, then, higher up, it also stops
being closable. A safe default should sit well under the lower bound -- 16
million cells leaves a factor of two -- and `ConPtyMaxCells` (§16) exists so
this can be moved without touching code.

### The alternate screen is invisible, confirmed

The alt-screen child entered and left `ESC[?1049h/l`, and **no frame in that
session contained either sequence**: conhost consumes them. Neither of §10's
proposed detectors survives -- not the stream, and not the geometry, since the
buffer already equals the viewport. One anomaly is recorded for whoever builds
the detector: while the child was in the alternate screen, the narrow frame
carried 3992 terminators instead of the usual 1993, which looks like both
buffers being painted in one frame.

### The child sees every width change, including 4000

The size-querying child reported `win=120x2000`, then `119x2000`, then
`4000x2000`, then back -- every resize, promptly. Width-aware programs will
therefore reformat for 4000 during a wide frame, which is the objection that
demoted direction C in §8 and is now a measured property of the wide path
rather than an argument about it.

### Ownership, re-examined: does the archive bring §7 back?

The obvious objection to reading structure out of frames is that f4 ends up
holding logical lines *and* conhost holds the grid, which is the two-owner
shape §7 died of. It is not, and the difference is worth stating precisely
because it is the difference between this design and the abandoned one.

§7 failed because f4 held history **above** the viewport and had to sew a
delta onto it. The seam was in the middle of the stream, rows had no identity,
and every join was a guess. Here a frame covers the **whole** buffer, first row
to last. Nothing is joined; the mirror is replaced. Once replaced, f4 can
re-wrap those logical lines to any width by itself, as often as it likes,
without asking conhost anything -- re-wrapping logical lines is a local
operation f4 already performs in its editor.

**Until the buffer overflows there is exactly one owner.** At a 32000-row
height that is tens of thousands of lines of work during which conhost holds
everything and f4 only reads. An archive of f4's own is needed only once rows
start being evicted.

**And when it is needed, rows are identified by ring position, not by text.**
Matching the tail of the archive against the head of a new frame by content
fails on the obvious case -- ten consecutive `------` lines match in ten
places. It is not needed: a frame is a snapshot of the ring, and the shift
between two frames follows from what the frame states about itself, the
terminator count (§17) and the final cursor position that marks the end of live
content. While the buffer has not wrapped the positions do not move at all and
there is nothing to join; once it wraps, the shift equals the growth.

**This last step is reasoning, not measurement.** Two frames bracketing a known
overflow must be captured and the arithmetic checked against them before any
code depends on it. `tools/conptymatrix` already produces both frames; the
check is a reading of dumps it can already make, not another field trip.

### Resizing while output is in flight

The case that destroyed §7 -- dragging the window corner while `dir` prints --
is measured here and is uneventful. Every session in §17 ends with a
`during-output` resize issued while the child is still writing: 58115 bytes,
1994 lines, 1993 terminators, `longWhole=true`, identical in shape to the same
frame taken at rest, at every height and every fill level. Repeated resizes
(`narrow-again`, `restore-again`) show no degradation either.

There is nothing to lose because f4 is not deciding which bytes belong to
whom. It receives a complete frame and replaces its mirror.

What a corner drag does cost is one frame per step: 57 KB and 46 ms at a
2000-row height with a full buffer, 949 KB and 609 ms at 32000. That is what
`ResizeDebounceMs` (§16) is for -- not correctness, but not shipping a
megabyte per pixel. Intermediate frames during a drag can simply be dropped,
because each frame is complete in itself; a property no delta-based design
ever had.

### Full-screen programs: the problem is geometry, not reflow

An earlier reading of this treated full-screen programs as a reflow problem.
They are not: they repaint themselves, and the history above them does not
depend on what they believe the screen size is. The problem is **geometry**. A
program told its window is 32000 rows tall will draw its frame across 32000
rows and put its bottom border thirty thousand rows above the visible slice.
Nothing breaks; everything is off-screen.

So a full-screen program needs a console of the real size while it runs, and
the detector §17 shows both proposed mechanisms cannot supply is needed for
that -- not for wrapping.

Switching the height for its duration is cheap, and this is the second
consequence of f4 owning the logical lines: the history does not have to
survive inside conhost across the switch. f4 takes a frame before the switch,
keeps the lines, and lets conhost's ring be re-cut however it likes.

**What is left of the alternate screen as a signal.** `ESC[?1049h/l` is
consumed by conhost on 10.0.22000 and appears in no frame (§17), so it is not
available as a stream signal on this build. Two qualifications, neither
measured: the post-#17510 emitter added VT passthrough and may forward it, so
24H2 and later must be re-measured before the idea is written off; and a
native Windows `vim.exe` draws through the Console API and never sends 1049 at
all, as Far does not -- the sequence is used by VT programs, `vim` under WSL,
`mc`, anything from the Unix side. A working 1049 detector would therefore
cover part of the cases, not all of them.

### The role conhost actually plays

Taken together, the above moves conhost out of the position of *holding* the
history and into a narrower one: it lays out incoming output and states the
boundaries of it. f4 reads those boundaries from frames and owns the lines
afterwards.

That is a better place to depend on a Windows component. It needs conhost to
be correct about what it just printed, not about what it printed an hour ago;
it survives a height change, an alternate-screen excursion and a ring that
evicts; and it is much closer to how far2l's terminal is built, which is the
architecture f4 is reproducing.

### Over ssh to a Windows host

Asked because the target matrix in `TERMINAL.md` §0 names Windows over ssh
explicitly. This section is reasoning from the measurements, not a measurement.

The mechanism exists. `sshd` on a Windows host runs an interactive session on
a ConPTY, sized from the `pty-req` at connection and from `window-change`
afterwards, and forwards its bytes unchanged. So f4 controls the remote
geometry through the protocol, and a repaint frame with its `ESC[K CR LF`
structure would arrive intact: the native path is available in principle.

Three things spoil it. **Volume**: a frame that costs 949 KB locally at 32000
rows costs 949 KB over the wire per width change; 57 KB at 2000 rows is
tolerable, the tall configuration is not, so remote sessions need their own
much smaller height. **Risk**: the wide frame can wedge a conhost, and wedging
*someone else's* conhost on a server is not the local case of a probe killing
its own host -- path 2 should be off by default over ssh. **Unknowns**:
everything depends on the remote `sshd` build, how it clamps sizes, and
whether it uses ConPTY at all.

Which is the same conclusion §11 reached, now with numbers behind it: **FISH+
Step 17 is strictly better.** With f4 on the far side, all of this happens
locally there -- the 949 KB never crosses the network -- and what comes back is
logical lines.

One clarification to §11 while this is being written: on a *Linux* host over
ssh there is no ConPTY and no layout being imposed, the remote pty wraps
nothing, and f4 wraps long lines itself as it always has. What stays fixed
there is column formatting like `ls -C`, which the remote program generated for
a width it was told -- exactly as it is for every other terminal.

### Why no other Windows terminal does this

Worth recording, because "nobody does it" was treated as evidence against the
idea earlier in this file.

They have no reason to. WezTerm, Alacritty and Windows Terminal *are* the
terminal: ConPTY is their renderer, they display its frame, and they keep
scrollback as it was written without ever re-wrapping it. Long lines in the
frame are of no use to them, because they do not reconstruct logical lines --
they draw screen rows. The `ESC[K CR LF` structure has been in plain sight
since 1809; there was no question, so nobody read the answer.

Their duplicated rows after a resize have the same root cause as §7, in a
milder form. Their viewport is the size of the window; what scrolls off it they
captured from the live stream into scrollback of their own. On a resize ConPTY
repaints its viewport, and that repaint contains rows the terminal already
archived -- with no row identity in the stream, the overlap cannot be
recognised. Two owners again.

The tall viewport removes it by removing the archive: nothing ever scrolls off
into separate storage, one owner holds the grid, and a frame replaces the
mirror whole.

**And the price f4 pays, which a general terminal cannot.** A tall viewport
means telling the child something untrue about its window height -- measured in
§17, the child believes whatever it is told. A universal terminal cannot do
that: it must tell every program it hosts the truth, or the first `vim` breaks.
f4 can, because it knows which program it launched and can give a full-screen
one a real-sized console. The advantage is bought precisely by *not* being a
general-purpose terminal -- and the open detector question above is the bill
for it.

## 18. Where this leaves direction F

**Direction F is feasible.** Every question §15 named as blocking is answered,
and the answers are measurements rather than arguments. What remains open is
engineering and portability, not viability.

**Established by measurement.** A tall viewport is creatable at every height to
32000 rows and cheap to create (24-104 ms). Its entire buffer is re-wrapped and
re-transmitted on any width change, oldest line included. A frame states its
own logical structure through `ESC[K CR LF` -- terminator count is the logical
line count, and rows minus terminators is the rows consumed by wrapping. The
frame cost is a formula with both terms measured,
`SUM(line lengths) + 5 x rows`. History depth equals buffer height exactly, at
three scales. Repeated resizes and resizes during live output produce frames
identical in shape to those taken at rest -- the case that destroyed §7 is
uneventful here. The cell ceiling sits between 32 and 48 million, and beyond it
the host wedges and then stops being closable.

**Known imperfect, with its bound.** A line whose length is an exact multiple
of the width arrives with no terminator and cannot be told from a wrap (P13).
The error is always in one direction, a hard break read as a wrap, at a
frequency of one line in W. The wide frame resolves it -- a multiple of 119 is
not a multiple of 4000 -- for 144 ms at a 500-row height and 617 ms at 2000.

**The shape that follows.** A tall ConPTY whose height is the scrollback depth.
f4 rendering the bottom slice and translating coordinates. Logical structure
read from repaint frames only, never from the live stream, which splits long
lines while scrolling. f4 owning those lines once read, so that re-wrapping
needs no round trip and a height change costs no history. conhost reduced to
laying out new output and stating its boundaries. The wide frame reserved for
the F4 viewer and editor, where a user pressed a key and a few hundred
milliseconds buys exactness. Every constant a setting, per §16, because all of
them describe one build of one Windows.

**Open, and none of it blocking.**

- *Detecting a full-screen program*, now that both proposed detectors are
  disproved on 22000. Needed for geometry, not for wrapping. A 1049 detector
  may return on a post-#17510 build and would cover VT programs but not native
  Console API ones.
- *What a resize does while the alternate screen is active*, beyond the doubled
  terminator count §17 recorded.
- *The ring arithmetic for the overflow case*, which is reasoned above and not
  yet checked against two frames bracketing a known overflow.
- *Other builds*: 19045, and any build carrying the rewritten emitter, where
  the frame grammar must be re-measured into the settings of §16.
- *Remote Windows over ssh*: mechanically plausible, bounded by frame volume
  and by the risk of wedging someone else's host; FISH+ Step 17 remains the
  better answer.

This is a design with measured costs, a known failure mode, a bounded error and
a fallback. That is exactly what §7 lacked, and the reason it was abandoned.
