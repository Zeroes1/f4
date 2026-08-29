# conptyreconcile — recovering the boundaries conhost loses

A frame carries logical lines, terminated by `ESC[K CR LF` — except when a line
exactly fills its rows, which gets no terminator and arrives glued to the line
after it (P13, measured in `docs/CONPTY_RESEARCH.md` §17). The live stream is
the mirror image: it terminates such a line with a plain CRLF, but splits long
lines while the buffer scrolls. Neither source alone is enough; together they
are exact.

This tool implements that correction and checks it against ground truth — on a
mock, under fuzzing, and against a real ConPTY.

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
conptyreconcile.exe -seed 12345              # replay one round
```

Every round prints its seed. A seed that fails on Windows replays against the
mock on any machine — `go test ./tools/conptyreconcile` — with no Windows
involved, because both sides use the same generator.

The tool writes `conptyreconcile-<height>.log` and a raw dump beside it, and
waits for Enter before closing so a run from Explorer leaves something to read.

## What the tests are for

The mock reproduces the measured grammar and is validated against real dumps:
an empty buffer costs five bytes per row, and terminator counts match the field
captures at 500 and 2000 rows. Jitter is built in — the stream is cut into
random chunks at random offsets, including inside escape sequences — because a
parser that only works on whole frames passes on a mock and fails in the field.

Three fuzz targets cover the properties that must hold on any input: the report
never panics and never comes back empty, the correction never loses or invents
a character and only ever splits, and the stream splitter preserves the text
exactly.

**Every one of the following was a real bug, found here rather than by a
tester**, which is the whole point of the arrangement:

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

The last two are the pattern worth remembering: a fixed fixture and a mock that
only models what its author thought of will both agree with a wrong
implementation. Randomised rounds and a real capture will not.
