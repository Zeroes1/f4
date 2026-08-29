# conptyreconcile — recovering the boundaries conhost loses

A frame carries logical lines, terminated by `ESC[K CR LF` — except when a line
exactly fills its rows, which gets no terminator and arrives glued to the line
after it (P13, measured in `docs/CONPTY_RESEARCH.md` §17). The live stream is
the mirror image: it terminates such a line with a plain CRLF, but splits long
lines while the buffer scrolls. Neither source alone is enough; together they
are exact.

This tool implements that correction and checks it against ground truth on a
port-backed mock and under fuzzing; the Windows binary also captures a real
ConPTY. The mock is not treated as proof by itself: its edge cases are pinned
to the research and to captured failures.

## How the correction works

Not by matching text. The randomised tests show why: a line of 120 `+` followed
by one of 360 `+` is byte-identical to the reverse, so any content-only rule
picks one arbitrarily. It works **by order**, walking the live sequence and
using the emitter's own rule — a frame run is a sequence of live lines where
every one but the last filled its rows, and the last did not.

Two consequences fall out for free. The width each line was written at is read
from the size reports in the stream, so a resize *while output is arriving* is
handled by construction rather than by a special case. And a blank line printed
immediately after a line that fills the width — which vanishes from the frame
without trace — is recovered, because the live sequence still has it.

## Running it

```
conptyreconcile.exe                          # one fixed case
conptyreconcile.exe -fuzz 10                 # ten randomised rounds
conptyreconcile.exe -fuzz 5 -resize-during-output
conptyreconcile.exe -lines-only              # line reconstruction only
conptyreconcile.exe -seed 12345              # replay one round
```

The default invocation is the end-to-end check: a tall console, a frame read
out of it,
the mirror built from that frame, the visible slice at the real window size,
the coordinate mapping over that slice, scroll clamping, re-wrap without a new
frame, and the geometry switch for a full-screen program. Each stage is
compared against something computed independently, so a pass means the stages
agree rather than that nothing crashed. The same pipeline runs on the mock in
`TestPipelineFromFrameToVisibleSlice`.

Every round prints its seed. A seed that fails on Windows replays against the
mock on any machine — `go test ./tools/conptyreconcile` — with no Windows
involved, because both sides use the same generator.

The tool writes `conptyreconcile-<height>.log` and a raw dump beside it, and
waits for Enter before closing so a run from Explorer leaves something to read.

## What the tests are for

The mock reproduces the measured grammar and carries regressions for the field
captures: an empty buffer costs five bytes per row, and terminator counts match
the field captures at 500 and 2000 rows. The supplied 2000-row run was not
green: at write width 120 and frame width 119, two preceding full rows shift a
following CJK line during the ported reflow, producing 58 glyphs, a Microsoft
display-padding space, and two glyphs. The correction now consumes that byte
only when the live sequence proves it is padding, and the captured seed has a
dedicated replay test. Jitter is built in — the live stream is fed in random
chunks at random offsets, including inside escape sequences — because a parser
that only works on whole frames passes on a mock and fails in the field.

Seven fuzz targets cover the properties that must hold on any input: the report
never panics and never comes back empty, the correction never loses or invents
a character and only ever splits, and the stream splitter preserves the text
exactly.

**Every one of the following was a real bug or an uncovered mock boundary,
found here rather than by a tester**, which is the whole point of the
arrangement:

- the correction keyed on the width of the *frame* instead of the width the
  lines were *written* at — found by replaying a field capture; it made the
  correction do nothing at all while looking like it ran
- a bare `ESC` was skipped by one byte instead of two — found by the fuzzer in
  seconds, on the input `\x1b0`
- OSC was terminated only at BEL, so the window title conhost sends
  (`ESC ] 0 ; <path> BEL`) leaked into the first logical line and broke the
  whole alignment — found by replaying a field dump
- a blank line following an exact-width line disappeared silently
- content matching split identical runs at the wrong point — found by the
  randomised rounds, and the reason the algorithm is order-based
- the expected list omitted the end marker the child itself prints, so a
  correct run reported one line short — the mock had never modelled a child
  that prints more than the fixture
- the frame writer's wide-cell edge padding was absent, so a 120-to-119 repaint
  differed by one literal space inside a globally reflowed run — the current
  mock models only this documented `WriteInfos` case and the reconciler
  consumes it only with proof from the live sequence

## The stream is read with a cursor, not with rules about it

`grid.go` applies the stream to a screen with a cursor, autowrap, delayed EOL,
scrolling and a scrollback, and marks a row as wrapped when *this code* wraps
it -- which is how Windows Terminal obtains the same flag, and mirrors
`ROW::SetWrapForced` and the `if (newX >= newWidth)` in `TextBuffer::Reflow`.
Logical lines are then rows joined by that flag. Double-width glyphs that do
not fit at the end of a row move down whole and leave a padding cell, which is
`ROW::SetDoubleBytePadded` and must not become a space when the line is
reassembled.

It replaced a set of rules I wrote about what the stream "usually" looks like
-- a CRLF followed by a cursor move is a continuation, and so on. A real
capture broke them at once: a 119-column line in a 120-column console ended
with **no newline at all**, followed by an absolute `ESC[30;1H`, because
conhost repositioned the cursor rather than emitting a newline and a blank
row. The rules read that as one continued line and produced 130 logical lines
where 151 were printed; the cursor model produces 151 of 151 on the same
bytes.

The research document had already said this -- finding P11: *read the bytes
with a cursor model, not a CRLF split*. Writing rules instead cost a field run.

## Everything with a Microsoft original is a port of it

`ucd.go` is `src/types/CodepointWidthDetector.cpp` from `microsoft/terminal`:
the four-stage trie over all 1,114,112 codepoints, the grapheme join rules and
the lookup, reproduced unchanged with the MIT notice retained. Only the syntax
is Go, and the file is generated from their source rather than typed.

It replaced a short table of "wide" ranges written from memory, which was
wrong in a way that mattered: conhost decides where a row ends, and if this
disagrees with conhost then it disagrees about where a line exactly fills its
rows -- which is exactly where conhost merges two logical lines into one.

The port also exposed a second error in the same place. `ucdToCharacterWidth`
returns an enum, not a column count: the value 3 means *ambiguous* and is
replaced by `_ambiguousWidth`, whose default is 1. Reading it as a width made
every Cyrillic line three times too wide. That is ported too, along with the
U+FE0F rule and the clamp to two, and `ambiguousWidth` is a setting here
because `SetAmbiguousWidth` is one there.

## A real command under a corner drag

The generated fixture is deterministic, which is what makes a ground-truth
comparison possible and also what makes it unlike anything a user runs.

```
conptyreconcile.exe -cmd "dir /s C:\Windows\System32" -drag 40
```

runs a real command and resizes the console forty times, at random widths, at
random intervals, while it prints. There is no ground truth, so it checks the
invariants that hold for any content -- output arrived, the correction only
splits, re-wrapping holds at *every* width the drag used, coordinates stay
inside the mirror, scrolling stays clamped -- and logs the whole timeline of
resizes with byte counts so a failure can be read rather than reproduced.

The same exercise runs here on a real Unix pty against `ls -laR /usr`
(`TestRealCommandUnderACornerDrag`). A pty is not ConPTY and wraps nothing, so
that test cannot check the reconstruction; what it does check is everything
that is not ConPTY-specific, over two megabytes of real output and forty live
resizes. On this machine: 42,940 logical lines, every width held, every
coordinate inside the mirror.

## What the mock did not model, and now does

Asked directly after a passing run: which parts of the original were
reproduced inaccurately. The audit found four old gaps plus two new boundaries;
each is either covered now or explicitly recorded as outside this text oracle.

**Widths were counted in bytes.** Everything measured `len(s)`, which is right
only for ASCII. A Cyrillic line is twice as many bytes as cells, so "the length
is a multiple of the width" -- the rule that decides whether conhost merged a
line -- fired in the wrong places, and wrapping cut characters in half.
conhost counts cells and gives a wide glyph two, with a separate flag for the
padding cell left when one will not fit (`ROW::WasDoubleBytePadded`). `cells.go`
is the model catching up; the generator now emits Cyrillic and CJK so the path
is exercised rather than assumed. The fuzzer immediately found an infinite loop
in the new code -- a single glyph wider than a one-column window -- which is why
every row split goes through one guarded helper.

**Output interleaved with a frame was not modelled.** The live stream and the
frame were built as separate blocks and butted together, so the
`-resize-during-output` run was less well covered than its PASS suggested: in
reality the child keeps printing *while* the repaint is written.
`FrameInterleaved` does that now.

**Eviction dropped whole lines.** conhost's ring evicts *rows*, so the top of a
frame can begin partway through a wrapped line, where an aligner looking for
whole lines cannot find its start. `fitRows` models it and a test pins the
behaviour.

**The stream had no window title and no colour.** `ESC]0;…BEL` is what leaked
into the first logical line of a real capture and broke everything; the mock
never had it, so the mock never caught it. The title and BEL/ST parser coverage
are now present. SGR attributes remain an explicit non-port because this tool
verifies text boundaries, not colour runs; the omission is recorded in the
Microsoft-port headers rather than guessed at.

**The incremental parser was not actually exercised.** The end-to-end test
reassembled the live chunks before parsing, so it did not test a read ending in
the middle of UTF-8, OSC, or CSI. It now feeds those chunks directly and pins
whole-buffer versus one-byte equivalence.

## The full-screen detector

All four layers of §17, with the platform-specific part behind a
`ProcessLister` interface so the decision logic is tested on any machine:

1. **the process tree** — deterministic, and the only layer that is not an
   inference. f4 spawns the shell, not the program the user runs in it, so the
   watcher walks descendants and matches image names against a configurable
   list. A failed enumeration keeps the previous answer rather than flipping
   the geometry; a cycle in reported parents cannot hang it; the geometry
   returns only when the *last* watched program exits.
2. **`ESC[?1049h/l`** — available where the emitter forwards it, which is not
   10.0.22000.
3. **frame signals** — the doubled terminator count and content spanning the
   whole viewport. Labelled a heuristic, consulted last.
4. **the user's key** — no detector may be the last word.

The Windows run exercises layer 1 with `cmd.exe`, which every machine has:
watching for `vim.exe` would make the check untestable exactly where it needs
testing, and which names denote full-screen programs is a setting anyway.

## The terminal core

`terminal.go` is the f4 side: the mirror of logical lines, the reflow, the
bottom slice, the screen-to-mirror coordinate mapping, scroll clamping, the
geometry decision and the layered full-screen detector. It is deliberately
portable — none of it touches Windows — so all of it is tested and fuzzed here,
and the Windows run only has to confirm that the real ConPTY agrees.

It stores text, not attributes: colour travels in the stream as SGR and is
applied by f4's renderer at draw time. The reflow only has to get the
boundaries right, and boundaries are what conhost loses.

The last two are the pattern worth remembering: a fixed fixture and a mock that
only models what its author thought of will both agree with a wrong
implementation. Randomised rounds and a real capture will not.
