# conptyreconcile source audit ledger

This ledger is part of the probe.  It is normative for the mock and is
reviewed before any test run.  The only Microsoft source revision allowed in
the port is:

`e9b4e2e18fb1b9cee6839969d42cd0f95d228926`

Source archive: `https://github.com/microsoft/terminal/tree/e9b4e2e18fb1b9cee6839969d42cd0f95d228926`.

The source manifest records SHA-256 values for every public source file read
by the port.  A line marked `MS port` must have its control flow, ordering,
defaults, and boundary behavior checked against that exact file before the
mock gate.  `Input invariant` is not host behavior and cannot be used to make
a mock pass.  `Gap` blocks the gate until resolved or explicitly isolated as
the documented reconstruction permitted by the plan.

## Port map

| Mock area | Exact pinned source | Symbols/behavior | Status |
|---|---|---|---|
| Unicode widths | `src/types/CodepointWidthDetector.cpp`, `src/types/inc/CodepointWidthDetector.hpp`, `src/types/convert.cpp` | `GetWidth`, `IsWide`, `_lookupGlyphWidth`, `_extractCodepoint`, fallback cache, `GetQuickCharWidth`, Unicode 13.0.0 override table | MS port |
| UTF-16 grouping | `src/types/Utf16Parser.cpp`, `src/types/inc/Utf16Parser.hpp` | `ParseNext`, `Parse`, leading/trailing surrogate tests | MS port |
| cell storage | `src/buffer/out/CharRowCell.cpp/.hpp`, `CharRowCellReference.cpp/.hpp`, `UnicodeStorage.cpp/.hpp`, `DbcsAttribute.hpp` | reset/erase, stored glyphs, DBCS flags, coordinate-keyed storage and remap | MS port |
| attribute rows | `src/buffer/out/AttrRow.cpp/.hpp` | `til::small_rle` run storage, `Reset`, `Resize`, `SetAttrToEnd`, `Replace`, and `GetHyperlinks` | MS port |
| rows | `src/buffer/out/CharRow.cpp/.hpp`, `Row.cpp/.hpp` | measure bounds, glyph access, row wrap and double-byte-padding flags, cell writes | MS port |
| text buffer | `src/buffer/out/textBuffer.cpp/.hpp` | `_AssertValidDoubleByteSequence`, `_PrepareForDoubleByteSequence`, `InsertCharacter`, `IncrementCursor`, `NewlineCursor`, `IncrementCircularBuffer`, `Reflow` | MS port |
| legacy write path | `src/host/_stream.cpp/.h`, `src/host/outputStream.cpp/.hpp`, `src/host/cmdline.h`, `src/host/stream.cpp/.h`, `src/host/misc.cpp` | `AdjustCursorPosition`, `WriteCharsLegacy`, processed controls, delayed EOL, backspace accounting and bisect check | MS port |
| parser | `src/terminal/parser/stateMachine.cpp/.hpp`, `OutputStateMachineEngine.cpp/.hpp` | ground/escape/CSI/OSC/DCS state persistence and dispatch limits | MS port |
| VT dispatch | `src/terminal/adapter/adaptDispatch.cpp/.hpp`, `adaptDefaults.hpp`, `ITermDispatch.hpp` | cursor movement, erase, tab, scroll, alternate screen, DEC modes | MS port |
| OSC color parsing and base64 | `src/types/utils.cpp`, `src/types/colorTable.cpp`, `src/types/inc/{utils,colorTable}.hpp`, `src/terminal/parser/base64.cpp/.hpp` | XParse/X.Org colors, OSC 4/10/11/12/52 helper branches, strict base64 state machine | MS port plus isolated conversion boundary |
| VT emission | `src/renderer/vt/VtSequences.cpp`, `paint.cpp`, `XtermEngine.cpp/.hpp`, `vtrenderer.hpp` | cursor movement optimization, delayed EOL, UTF-8 line paint and frame order | MS port |
| host launch | `src/winconpty/winconpty.cpp/.h`, `device.h`, `src/server/DeviceHandle.cpp/.h`, `src/server/WinNTControl.cpp/.h`, `src/cascadia/TerminalConnection/ConptyConnection.cpp` | pinned adjacent `OpenConsole.exe` process, ConDrv handles, attached-client attribute, and ConPTY resize/close lifecycle | MS API path with pinned-host gate |
| bidi assertion | No text-buffer reorder function exists in the pinned execution path | Preserve UTF-8/UTF-16 order as an input invariant; no bidi visual oracle is invented | Input invariant |

## Isolated documented boundary

The implementation of Windows `MultiByteToWideChar` is not present in the
pinned OpenConsole tree.  The pinned source calls that OS API from
`ConvertInputToUnicode`, `ConvertOutputToUnicode`, and `Base64::s_Decode`; the
mock's UTF-8 conversion boundary therefore reconstructs only the documented
`CP_UTF8`, flags-zero conversion result (including UTF-16 surrogate output and
failure for invalid input). No other terminal or host implementation is used
to fill this boundary. This reconstruction is isolated in `ms_utf8.go` and
the final conversion in `ms_base64.go`.

`AdaptDispatch::DesignateCodingSystem` also calls the Windows
`GetConsoleOutputCP` and `SetConsoleOutputCP` APIs. Their implementation is
outside OpenConsole; the mock keeps that dependency behind the recorded
`outputCodePage` state and preserves the pinned call/order/success branches
for ISO-2022 and UTF-8. This is an external API seam, not a replacement
terminal implementation, and the Windows host gate must still exercise the
actual pinned call path.

The pinned source also delegates custom hyperlink identity to
`std::hash<std::wstring_view>`. That standard-library implementation is not
present in OpenConsole and its numeric result is not specified by the C++
standard. The isolated `textBuffer.getHyperlinkID` reconstruction therefore
preserves only the documented equality relation `(custom id, URI)`; it must not
be treated as an exact numeric MS-port, and it remains a blocking boundary for
the final gate.

## Three-pass self-audit record

The self-audit is a gate, not a test result.  It must be generated by
`audit.go` after all source-port files are final and before `go test`, and it
must contain three independent passes:

1. symbol/path pass: every mock symbol maps to one exact source symbol;
2. transition pass: every branch, default, limit, and ordering decision is
   checked against the pinned source text;
3. negative pass: search the mock and report for another host, a stale dump,
   an old source revision, an equivalent implementation, or an unrecorded
   reconstruction.

An audit pass is `PASS` only when its report contains no unresolved `Gap`.
The absence of a panic is not an audit result.  A failed or missing pass
blocks all test execution.

### Current construction state

No audit result is recorded here.  The referenced `audit-pass-*.json` files
are not part of this checkout, and the current executable audit still returns
`transition/control-flow=FAIL`.  Consequently no mock test, fuzz run, seed
run, or host run is accepted as a result while this ledger contains open
items.

## Known open audit items while the port is being written

This section is intentionally explicit during construction.  It must be
empty before the mock test gate.

- `pending`: parser and adapter dispatch must be checked line-by-line against
  the pinned state machine and `AdaptDispatch` after their transcription is
  complete.
- `pending`: the current UTF-8 ingress has to be checked against the pinned
  UTF-16 conversion boundary; raw bytes must not be treated as C1 controls.
- `pending`: `AdaptDispatch::DesignateCodingSystem` now records the pinned
  code-page calls and GL/GR updates, but the external Windows API seam still
  needs a direct check in the Windows-host path; a guessed code-page result is
  not admissible.
- `pending`: `WriteCharsLegacy` still depends on the pinned screen, cursor,
  margin, and conversion services that are not yet represented by the mock;
  its missing branches must not be replaced by a convenient buffer-only
  implementation.
- `pending`: `OutputCellIterator` still needs a direct audit of iterator
  advancement, surrogate grouping, DBCS lead/trail conversion, and attribute
  behavior. `ATTR_ROW` run operations have now been isolated in the mock and
  mapped to the pinned implementation; that change is not itself a test
  result.
- `pending`: `TextBuffer::Reflow` now carries the pinned optional
  `PositionInformation` and `lastCharacterViewport` branches, but the direct
  source comparison still has to verify every failure/exception path before
  this row can become `MS port`.
- `pending`: the Windows ConPTY launcher must be checked against the pinned
  `winconpty.cpp` path and must reject every executable except the pinned
  SHA-256.
- `pending`: `DeviceHandle.cpp` is part of the launcher path and must be
  ported together with the pinned `winconpty.cpp`; a public `CreatePseudoConsole`
  fallback is not allowed.
- `pending`: source-backed frame emission must be checked against the pinned
  `XtermEngine::_MoveCursor`, `VtEngine::_PaintUtf8BufferLine`, and
  `VtSequences.cpp`; a frame made by a test fixture is not evidence.
- `pending`: OSC parsing now has the pinned helper control flow, but the
  custom hyperlink numeric identity depends on the unavailable
  `std::hash<std::wstring_view>` implementation described above.
- `pending`: RIS/HardReset now has the pinned operation order in the mock,
  but still requires the direct source comparison, including the ConPTY
  false-return/pass-through branch and the non-text soft-font boundary.
- `blocked`: the stock pinned ConPTY API exposes one untagged output byte
  stream. `ReadFile` on that pipe cannot identify which bytes came from the
  client/live output and which bytes are renderer repaint output when they
  are interleaved. A timestamp, read boundary, marker, parser heuristic, or
  resize boundary would be a non-source inference and is forbidden by the
  plan. The current host recorder therefore keeps the bytes as
  `streamObservedOutput` and must not claim a live/frame split or pass the
  host gate until a source-backed channel exists.
