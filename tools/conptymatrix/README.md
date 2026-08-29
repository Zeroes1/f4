# conptymatrix — a grid, not a list of questions

Every probe in this repository before this one covered whichever questions its
author had in mind that day, and each time the interesting result turned out to
be the one nobody had thought to ask: the constant term that was really a
*content* term, the buffer that was never actually full, the line that was
never exactly the width of the console. A list of questions cannot be complete,
because it is written by the same person who is missing something.

So this probe does not have a list. It has axes, and it runs their product.

| Axis | Values |
|---|---|
| fill | empty, small (150 lines), full (height−10), overflow (1.5× height) |
| height | 500, 2000, 32000 |
| width op | narrow-by-one, wide (4000), restore, narrow again, restore again, resize during live output |
| line shape | short, **exactly the console width**, three times the width, double-width CJK, 24-bit colour |
| child | our emitter, `cmd.exe /c dir /w`, alternate screen, size-querying |
| product | 4000 columns against ten heights, to locate where the host wedges |

Line shapes cost nothing extra: every session prints all of them, so every
combination of fill and height is also a test of every shape.

## What it decides while running

Nothing. Fixed schedules, a reader with no timeout, raw bytes to a file. The
summary is computed afterwards from those bytes. Probes that decided *during*
a run whether a phase had finished produced confident zeroes that were their
own bugs; this one cannot, because a mistake in reading a dump costs a re-read
rather than another trip to the machine under test.

## The two things it is built to catch

**Retirement.** The overflow cases print more lines than the buffer holds. The
summary reports the *oldest* line still present after a reflow, so eviction is
a number rather than an assumption.

**The P13 ambiguity.** One line per session is emitted at exactly the console
width — the single case where a full row followed by `ESC[K` cannot be told
from a wrap. No probe here had ever actually emitted one. The summary reports
what followed it rather than judging it.

## Safety

`ClosePseudoConsole` blocks until the host exits on this build, and a
4000×32000 resize has been observed to wedge it permanently: `S_OK`, then no
bytes, no EOF, and a close that never returns. Every teardown here runs the
close on its own goroutine and terminates the host directly if it does not come
back within three seconds, so a wedged case costs one host rather than
accumulating them across twenty sessions.

## Running it

```
conptymatrix.exe                    # the whole grid
conptymatrix.exe -only height=500   # one slice
conptymatrix.exe -only product      # just the ceiling sweep
```

It writes `conptymatrix-<pid>.log` (the summary) and one
`conptymatrix-<case>.txt` raw dump per case, each capped at 4 MB — the counts
in the summary always use the full stream, so a truncated dump never changes a
number.
