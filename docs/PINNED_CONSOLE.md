# The pinned console host

**The host used by the standalone probe, and the only host any measurement may
be made against:**

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
