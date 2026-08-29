# termprobe — one run, every surviving direction

This probe collects, in a single pass, the measurements that **all** the live
directions of `docs/CONPTY_RESEARCH.md` depend on — not only the most likely
one. A direction should be chosen on a table, not on a hunch, and a tester
should not be asked for a second round trip because the first probe was
scoped to whichever idea looked best that day.

| Direction | What termprobe measures for it |
|---|---|
| **F — tall viewport** | The height ladder, and everything that changes along it |
| **D3 — bundled OpenConsole** | Versions of every binary; the whole ladder re-runnable against a bundled `conpty.dll` |
| **A — pipes** | Which shells and tools produce anything with no console; whether `COLUMNS` is honoured |
| **C — permanently wide** | What a width-aware program prints at 120 vs 4000 columns |
| baseline | P6/P11/P12/P14 re-asked on this build: wrap shape, size report, repaint-starts-at-home |

## The height ladder

The rungs are **125, 250, 500, 1000, 2000, 4000, 8000, 16000, 32000**, and
the lower ones matter as much as the ceiling: the useful answer is not "what
is the maximum" but "where does the cost turn". A 32000-row console that
takes two seconds to reflow is worse than a 4000-row one that takes forty
milliseconds, and only the ladder shows where that crossover is.

Each rung is a fresh pseudoconsole, so one failure cannot poison the next.
Per rung the probe records:

- **create**: whether `CreatePseudoConsole` accepts the height, and how long it takes
- **host memory**: the working set of *this session's* conhost/OpenConsole, after creation and after the history is written
- **history**: how many of the individually numbered lines arrive, and their lowest and highest index — a repaint that carries only the tail is a different fact from one that carries everything
- **reflow**: a one-column width change, then the bytes, the wall time, and *which* history lines the repaint carried. This is direction F's central claim — that the repaint is the transfer channel for the re-wrapped history
- **wide frame**: a widen to 4000, and whether the long line comes back as a single row (the F4 trick), then a restore and a re-count
- **the child's own view**: what `GetConsoleScreenBufferInfo` tells the program about window and buffer size, at start and inside the alternate screen
- **`SetConsoleScreenBufferSize` taller**: whether a client can raise the buffer above the viewport under ConPTY, which `getset.cpp` suggests it can
- **alternate screen**: whether `?1049h` / `?1049l` appear in the stream as clean protocol events

The verdict line names three ceilings: the tallest console that could be
created, the tallest whose width-change repaint still carried the whole
history, and the tallest that did so inside the repaint budget (`-budget-ms`,
250 by default). Those three numbers are the input to F's design.

## The child is the probe itself

Driving `cmd.exe` through a pseudoconsole means fighting command echo, the
prompt, code pages and pagers — each of which cost an earlier probe in this
repository a false negative. So the child is this same binary in `-emit`
mode: it prints numbered lines, one long line, a truecolor run, reports the
geometry it sees, tries the taller buffer, and enters and leaves the
alternate screen on cue. `cmd.exe` and PowerShell are still exercised, but as
subjects in the width-aware section rather than as the instrument.

Markers are ASCII and `~`-delimited, because an earlier transcript came back
as mojibake under OEM code page 850 and a marker that cannot survive the
transport is not a marker.

## Running it

```sh
GOOS=windows GOARCH=amd64 go build -o termprobe.exe ./tools/termprobe
```

```
termprobe.exe                      # the full ladder, all directions
termprobe.exe -quick               # 125..2000 only, for a fast first look
termprobe.exe -heights 500,4000    # a specific pair
termprobe.exe -skip-pipes          # skip direction A
termprobe.exe -bundled             # use conpty.dll placed next to the exe
```

The report goes to the console and to `termprobe-<pid>.log`; send the log.
The probe owns and terminates its own pseudoconsoles and children, writes no
setting, needs no administrator, and touches no console but the ones it made.

To measure D3, drop the signed `OpenConsole.exe` + `conpty.dll` pair from the
`Microsoft.Windows.Console.ConPTY` NuGet package next to the exe and run
twice, with and without `-bundled`; the two logs diff into an answer about
whether pinning the host changes any of the above.

## Reading a failure

Every number is printed next to what produced it, and unreliable rungs say so
in a `note:` line rather than reporting a zero. That is deliberate: this
project has already lost four field runs to counters that could not observe
their own subject and returned zeros that read as evidence. If a rung notes
that the child never reported `DONE`, everything below it on that line is
suspect and should be re-run, not interpreted.

The portable half (`analysis.go`) is unit-tested and runs on any OS:
`go test ./tools/termprobe`.
