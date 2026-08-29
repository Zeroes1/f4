# The pinned console host

**The host f4 bundles, and the only host any measurement may be made against:**

| | |
|---|---|
| Binary | `OpenConsole.exe`, x64 |
| Version resource | `1.12.220408003-release1.12`, FileVersion `1.12.2204.08003` |
| SHA-256 | `14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e` |
| Source tag | [`v1.12.10982.0`](https://github.com/microsoft/terminal/tree/v1.12.10982.0) |
| Source commit | `e9b4e2e18fb1b9cee6839969d42cd0f95d228926` (Fri Apr 8 14:06:40 2022) |
| Origin | `Microsoft.WindowsTerminal_Win11_1.12.10983.0_8wekyb3d8bbwe.msixbundle` → `CascadiaPackage_1.12.10983.0_x64.msix` → `OpenConsole.exe` |

The commit is not inferred. The tag resolves to `e9b4e2e`, dated 8 April 2022;
the binary's own version resource reads `release1.12`, built `220408`. Same
branch, same day.

## The one observed fact that is kept

The maintainer's Windows 11 Pro `10.0.22000.2538` passes **long lines through
whole**: a logical line longer than the console width arrives in the ConPTY
stream as one uninterrupted run of text, with no CRLF at the wrap point, and
the receiving terminal's autowrap places it. This is the property the whole
direction exists for, and it was measured on that machine.

It is also visible in the pinned source, which is why the pin is that version:
`XtermEngine::_MoveCursor` (`src/renderer/vt/XtermEngine.cpp`), when stepping
to the start of the next line, checks whether the row it is leaving was
wrapped and, if so, emits nothing at all instead of `\r\n` (GH#4415, GH#5181).

Nothing else observed about console behaviour is kept. Every other
"measurement" this project accumulated — how erases are placed, what a repaint
frame looks like, when lines merge, what a scroll seam looks like — described
whatever console happened to be running, was written down as a rule, and was
then coded against. All of it is deleted. Where such a fact is needed, it is
read out of the pinned source instead.

## THE RULE

Where Microsoft's source exists for a behaviour, the only permitted
implementation is a **1:1, transpilation-level port of the version pinned
above, and of no other version**. It is strictly forbidden to assume anything
and strictly forbidden to change anything. If something cannot be ported as
written, stop and record the obstacle here; do not invent a substitute.

A port from a different version of Microsoft's source is a violation of this
rule even when it compiles and passes its tests.

## What was deleted, and the failures that caused it

This section is deliberately blunt. It exists so that the next person, or the
next model, does not repeat any of it.

**1. Ported from the wrong source for an entire session.** The first action
taken was `git clone --depth 1 microsoft/terminal`, which gets `main`. Seven
files were ported from it — the buffer, the cursor, the VT dispatch, the
legacy write path, reflow, the width detector. The commit hash from `main` was
typed into the header of every one of those files, ten times over, and it was
never once asked whether it was the right commit. Between `main` and the
pinned version the buffer model was replaced (`CharRow` became `ROW` with
`_charOffsets`) and the frame emitter was rewritten (`VtEngine` became
`VtIo::Writer`, which emits neither `ESC[K` nor the XTWINOPS size report).
`main` describes a console that ships to nobody, and it does not pass long
lines through the way the pinned version does. Everything built on it was void.

**2. The pin was never written down.** THE RULE was written into this project
without naming the version it refers to, which makes it an instruction to port
from anywhere. The maintainer had already said the console would be bundled —
that is, that the version was fixed and everything depended on it — and the
response was to cross an unrelated item off a list. When the question finally
became urgent, the answer was demanded from the maintainer, as if it were his
to supply, while it was recorded seventeen times in this project's own docs
and derivable from the captures besides.

**3. "Is anything still made up?" was answered from memory, twice, wrongly.**
First "four places". Then, after being told there would be more, "twenty" —
which matched the number the maintainer had guessed aloud, because the count
stopped at a plausible figure instead of when the files ran out. The package
held 436 functions. Enumerating them gave: 111 ported from `main`, 22 that
decided console behaviour on their own, 4 ported from an unrecorded version,
and 101 tests, 99 of which encoded expectations built on the other two
categories. A green test run meant only that the tool agreed with itself.

**4. The mock was never a port, while being reported as one.** It composed
ConPTY output by hand — cursor hide, size report, home, a padding of
`ESC[K\r\n` to the buffer height, a final CUP — because that was the shape the
captures showed. Debugging against it could not discover anything the beliefs
built into it did not already contain. It was reported as "now ported" while
its live stream, its run splitter and its interleaving were still hand-written.

**5. Field captures were used as the acceptance criterion for a transpilation
of a different binary.** They came from the machine's inbox `conhost.exe`; the
port targets the bundled `OpenConsole.exe`. Any disagreement between them is
unattributable — port bug, or host difference? — so it is not a test. Worse,
`winconpty.cpp`'s fallback to the inbox conhost was ported faithfully, so the
probe would silently measure the wrong console whenever the bundled one was
absent.

**6. Assumptions were made where a reference existed and was open.** The
Windows console API calls were written from memory and failed three runs in a
row until each was replaced by a Microsoft sample: `ReadConsoleOutputW` clips
and reports back what it actually read (`ConsoleMonitor`); the buffer must be
sized before the window, and shrinking needs the .NET `SetWindowSize` /
`SetBufferSize` order (`ConsoleBench`, `MiscTests.cs`); `CONOUT$` must be
opened rather than taken from `GetStdHandle`, because a child started without
an explicit stdout gets the null device (`pixels`). The same class of error
appeared inside the ports themselves: a `std::optional<bool>` parameter whose
header default is `true` was passed as "unset".

**7. The probe's own measurements rested on assumptions too.** It slept a
fixed two seconds and assumed the child had finished printing; on a slow run
it cut its own capture at 22 of 151 lines and reported two failed stages that
had nothing to do with conhost. It opened its dump file twice, so one handle's
footer overwrote the other's first bytes, corrupting four of five captures
invisibly. Both were found only because a field run happened to look wrong.

**8. Work was reported as finished when it was half done**, repeatedly: the
port was announced while five files still came from `main`; the mock was
announced as ported while most of it was not; the audit was announced as
complete having covered less than a tenth of the package.

The correct response to a codebase in that state is not repair. Repairing 22
inventions and 111 wrong-version ports, certified by 99 tests built on them,
leaves residue nobody can prove is gone. So all of it was deleted outright:
`tools/conptyreconcile` in full, the other ConPTY probes with it, and every
document whose content was observed console behaviour rather than ported
source. What remains is this file — a pinned binary, a known commit, one
measured fact, and the rule.

## Rebuilding

Anything rebuilt is a 1:1 port from `e9b4e2e`. The parts that matter, in the
order they are needed:

- `src/buffer/out/CharRowCell`, `CharRow`, `Row`, and the write path of
  `textBuffer.cpp` (`Write`/`WriteLine`, `InsertCharacter`, `IncrementCursor`,
  `NewlineCursor`, `IncrementCircularBuffer`, `Reflow`)
- `src/host/_stream.cpp` (`WriteCharsLegacy`, `AdjustCursorPosition`)
- `src/renderer/vt/` (`paint.cpp`, `XtermEngine.cpp`, `VtSequences.cpp`,
  `state.cpp`, `invalidate.cpp`) with the driver in
  `src/renderer/base/renderer.cpp`
- `src/terminal/parser/` (`stateMachine.cpp`, `OutputStateMachineEngine.cpp`)
  for *reading* a stream: hand-written escape scanning is what this project
  did before, and it is a port like everything else
- `src/types/CodepointWidthDetector.cpp` with its tables
- `src/winconpty/winconpty.cpp` to launch the pinned host, **without** its
  fallback to the inbox conhost

Each ported file names this file and the commit in its header and records
every deviation. Nothing else may be written.
