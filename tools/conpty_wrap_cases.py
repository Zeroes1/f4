"""Case matrix and stream analysis for the ConPTY logical-line probes.

Background
----------
Since microsoft/terminal#17510, conhost translates Console API writes to VT
synchronously. In `WriteCharsLegacy` (src/host/_stream.cpp) the text of a
write is handed to the VT writer verbatim, and a `\\r\\n` is appended when -
and only when - the *last* character of that write landed on the right
margin:

    const auto lastCharWrapped = _writeCharsLegacyUnprocessed(...);
    writer.WriteUTF16(chunk);
    if (lastCharWrapped) { writer.WriteUTF8("\\r\\n"); }

`lastCharWrapped` is not a guess. Three lines earlier the same value is used
to set conhost's own per-row soft-wrap bit:

    wrapped = wrapAtEOL && state.columnEnd >= state.columnLimit;
    if (wrapped) { textBuffer.SetWrapForced(cursorPosition.y, true); }

So conhost records "this row continues on the next one" in its buffer and, in
the same breath, emits into the VT stream the one sequence that means the
opposite to the terminal.

What the probe shows
--------------------
Every case below writes *only printable characters* - no CR, no LF, no tab,
no escape. Therefore **every line break that appears in the ConPTY output was
manufactured by ConPTY**; there is nothing to attribute to the application and
nothing to argue about.

The cases in `GROUP_SAME_TEXT` all write the exact same 200 characters and
produce the exact same screen. They differ only in how the write is split
across `WriteConsoleW` calls - which is an artifact of the writer's buffering
and is invisible to the user, to the application's own source code, and to the
terminal. If the resulting streams differ in where line breaks fall, then the
logical-line structure a terminal reconstructs is a function of stdio
buffering rather than of the text, and no terminal-side heuristic can repair
that, because the information needed to do so never leaves conhost.

This module is import-safe on any platform: it holds no Windows calls. Run it
directly to self-test the analyser against synthetic streams.
"""

from __future__ import annotations

import unicodedata

COLS = 80
ROWS = 25
OUTDIR = "probe-out"

# The character used to fill the payload. Any printable, single-width,
# non-control character will do.
FILL = "A"
# A double-width character for the east-asian cases.
WIDE = "\u4e00"  # CJK IDEOGRAPH ONE


class Case:
    """One probe run: a list of strings, each written with one WriteConsoleW."""

    def __init__(self, name, writes, group, note):
        self.name = name
        self.writes = list(writes)
        self.group = group
        self.note = note

    @property
    def text(self):
        return "".join(self.writes)

    @property
    def shape(self):
        sizes = [len(w) for w in self.writes]
        if len(sizes) == 1:
            return f"1 x {sizes[0]}"
        if len(set(sizes)) == 1:
            return f"{len(sizes)} x {sizes[0]}"
        if len(set(sizes[:-1])) == 1:
            return f"{len(sizes) - 1} x {sizes[0]} + {sizes[-1]}"
        return "+".join(str(n) for n in sizes)


def _chunked(s, n):
    return [s[i:i + n] for i in range(0, len(s), n)]


# ---------------------------------------------------------------------------
# Group 1: identical text and identical screen, different write chunking.
#
# 200 is deliberately not a multiple of 80, so a single bulk write never ends
# on the margin and never triggers the injection. 15 does not divide 80 or
# 160; 16 divides both. That is the whole difference between the two.
# ---------------------------------------------------------------------------
_PAYLOAD = FILL * 200

GROUP_SAME_TEXT = [
    Case("bulk-200", [_PAYLOAD], "same-text",
         "one write - no write boundary ever lands on the margin"),
    Case("chunk15-200", _chunked(_PAYLOAD, 15), "same-text",
         "15-char writes - 80 and 160 are not multiples of 15"),
    Case("chunk16-200", _chunked(_PAYLOAD, 16), "same-text",
         "16-char writes - 80 and 160 ARE multiples of 16"),
    Case("char-200", list(_PAYLOAD), "same-text",
         "unbuffered, one character per write"),
]

# ---------------------------------------------------------------------------
# Group 2: single write, length sweep, to pin down the rule itself.
# ---------------------------------------------------------------------------
GROUP_LENGTHS = [
    Case(f"len-{n}", [FILL * n], "lengths",
         f"{n} chars, ends at column {n % COLS if n % COLS else COLS}")
    for n in (COLS - 1, COLS, COLS + 1, 2 * COLS - 1, 2 * COLS, 2 * COLS + 1)
]

# ---------------------------------------------------------------------------
# Group 3: double-width text. A row can end one cell short of the margin and
# still be a wrap, which is the case that defeats width-counting heuristics on
# the terminal side.
# ---------------------------------------------------------------------------
GROUP_WIDE = [
    Case("cjk-39", [WIDE * 39], "wide", "78 columns, short of the margin"),
    Case("cjk-40", [WIDE * 40], "wide", "80 columns, exactly on the margin"),
    Case("cjk-pad", [FILL + WIDE * 40], "wide",
         "1 + 80 columns: the last glyph cannot fit, row ends one cell short"),
]

CASES = GROUP_SAME_TEXT + GROUP_LENGTHS + GROUP_WIDE
BY_NAME = {c.name: c for c in CASES}


# ---------------------------------------------------------------------------
# Stream analysis
# ---------------------------------------------------------------------------

def _char_width(ch):
    if unicodedata.combining(ch):
        return 0
    return 2 if unicodedata.east_asian_width(ch) in ("W", "F") else 1


class Parsed:
    """Printable text of a VT stream plus where the breaks fell in it."""

    def __init__(self, segments, breaks, controls):
        self.segments = segments        # printable runs between breaks
        self.breaks = breaks            # offsets into `text` of each break
        self.controls = controls        # any other control chars seen

    @property
    def text(self):
        return "".join(self.segments)

    @property
    def lengths(self):
        return [len(s) for s in self.segments]

    @property
    def columns(self):
        return [sum(_char_width(c) for c in s) for s in self.segments]


def parse_stream(raw):
    """Strip escape sequences; return the printable text and break offsets.

    Only `\\r\\n`, a bare `\\n` and a bare `\\r` followed by `\\n` count as
    breaks. A bare `\\r` is a carriage return and is recorded but does not
    split a segment, since ConPTY uses it for cursor positioning.
    """
    if isinstance(raw, bytes):
        text = raw.decode("utf-8", errors="replace")
    else:
        text = raw

    segments = [""]
    breaks = []
    controls = []
    total = 0
    i = 0
    n = len(text)

    while i < n:
        ch = text[i]

        if ch == "\x1b":
            j = i + 1
            if j < n and text[j] == "[":          # CSI
                j += 1
                while j < n and not (0x40 <= ord(text[j]) <= 0x7E):
                    j += 1
                i = j + 1
                continue
            if j < n and text[j] in "]PX^_":      # OSC / DCS / SOS / PM / APC
                j += 1
                while j < n:
                    if text[j] == "\x07":
                        j += 1
                        break
                    if text[j] == "\x1b" and j + 1 < n and text[j + 1] == "\\":
                        j += 2
                        break
                    j += 1
                i = j
                continue
            i = min(j + 1, n)                     # ESC + one byte
            continue

        if ch == "\r":
            if i + 1 < n and text[i + 1] == "\n":
                breaks.append(total)
                segments.append("")
                i += 2
                continue
            controls.append(("CR", total))
            i += 1
            continue

        if ch == "\n":
            breaks.append(total)
            segments.append("")
            i += 1
            continue

        if ord(ch) < 0x20 or ord(ch) == 0x7F:
            controls.append((f"0x{ord(ch):02x}", total))
            i += 1
            continue

        segments[-1] += ch
        total += 1
        i += 1

    return Parsed(segments, breaks, controls)


def strip_padding(text, payload):
    """Drop spaces the stream may have added to pad a row.

    No case payload contains a space, so this cannot remove real content.
    """
    return text.replace(" ", "") if " " not in payload else text


def structure(parsed, payload):
    """Row lengths as a terminal would end up with them, ignoring padding."""
    return tuple(len(strip_padding(s, payload)) for s in parsed.segments)


def describe(case, parsed):
    """One-line summary of what a case produced."""
    payload = case.text
    got = strip_padding(parsed.text, payload)
    rows = list(structure(parsed, payload))

    if got == payload:
        extra = ""
    elif payload in got:
        extra = f"  (+{len(got) - len(payload)} chars from the stream)"
    else:
        return (f"  {case.name:<14} writes {case.shape:<14} "
                f"-> payload MISMATCH: {len(got)} of {len(payload)} chars")

    if not parsed.breaks:
        note = "no break injected - logical line intact"
    else:
        note = f"{len(parsed.breaks)} break(s) injected at {parsed.breaks}"
    return (f"  {case.name:<14} writes {case.shape:<14} "
            f"-> rows {rows}  {note}{extra}")


def summarise_same_text(results):
    """Verdict for the group where every case emits identical text.

    `results` maps case name -> Parsed. Returns (ok, lines) where ok is True
    when every case produced the same break structure, i.e. when nothing is
    wrong.
    """
    lines = []
    cases = [c for c in GROUP_SAME_TEXT if c.name in results]
    if len(cases) < 2:
        return True, ["  (not enough cases ran to compare)"]

    payloads = {c.text for c in cases}
    if len(payloads) != 1:
        lines.append("  BUG IN PROBE: the cases do not share one payload")
        return True, lines
    payload = cases[0].text

    texts = {strip_padding(results[c.name].text, payload) for c in cases}
    if texts != {payload}:
        lines.append("  inconclusive: the recovered text differs between cases,")
        lines.append("  so the streams cannot be compared on structure alone.")
        return True, lines

    shapes = {c.name: structure(results[c.name], payload) for c in cases}
    distinct = sorted(set(shapes.values()))
    if len(distinct) == 1:
        lines.append(f"  every case arrived as rows {list(distinct[0])} "
                     "- nothing to report")
        return True, lines

    lines.append(f"  all {len(cases)} cases wrote the same {len(payload)} "
                 "characters and produced the same screen.")
    lines.append("  the payload contains no CR, no LF and no escape sequence, "
                 "so every")
    lines.append("  break below was manufactured by ConPTY:")
    lines.append("")
    for c in cases:
        rows = list(shapes[c.name])
        breaks = results[c.name].breaks
        where = (f"rows {rows}, breaks at {breaks}" if breaks
                 else f"rows {rows}, no break")
        lines.append(f"    {c.name:<14} ({c.shape:<14} writes)  {where}")
    lines.append("")
    lines.append("  the streams differ, so the logical-line structure a "
                 "terminal reconstructs")
    lines.append("  depends on how the writer happened to buffer, not on what "
                 "was written.")
    lines.append("  a terminal cannot recover the difference: the wrap bit "
                 "conhost set on")
    lines.append("  those rows (TextBuffer::SetWrapForced) is not in the "
                 "stream.")
    return False, lines


def summarise_lengths(results):
    lines = []
    for c in GROUP_LENGTHS:
        if c.name not in results:
            continue
        p = results[c.name]
        n = len(c.text)
        expect = "break expected" if n % COLS == 0 else "no break expected"
        actual = f"{len(p.breaks)} break(s)"
        agree = "as predicted" if bool(p.breaks) == (n % COLS == 0) else "DIVERGES"
        lines.append(f"  {n:>4} chars on {COLS} columns: {actual:<14} "
                     f"({expect}, {agree})")
    return lines


# ---------------------------------------------------------------------------
# Self-test - runs anywhere, no console needed.
# ---------------------------------------------------------------------------

def _self_test():
    esc = "\x1b[?9001h\x1b[?1004h\x1b[?25l\x1b[2J\x1b[m\x1b[H"
    osc = "\x1b]0;C:\\Windows\\SYSTEM32\\cmd.exe\x07"

    p = parse_stream(esc + "A" * 200 + osc)
    assert p.lengths == [200], p.lengths
    assert p.breaks == [], p.breaks

    p = parse_stream(esc + "A" * 80 + "\r\n" + "A" * 80 + "\r\n" + "A" * 40)
    assert p.lengths == [80, 80, 40], p.lengths
    assert p.breaks == [80, 160], p.breaks

    p = parse_stream(("A" * 16) * 5 + "\r\n" + ("A" * 16) * 5 + "\r\n" + "A" * 40)
    assert p.breaks == [80, 160], p.breaks

    p = parse_stream("\u4e00" * 40 + "\r\n")
    assert p.lengths == [40, 0], p.lengths
    assert p.columns == [80, 0], p.columns

    p = parse_stream("A" * 5 + "\rB" * 1)
    assert p.text == "AAAAAB", p.text
    assert p.breaks == [] and p.controls == [("CR", 5)], (p.breaks, p.controls)

    # A structure difference across the same-text group must be reported.
    results = {
        "bulk-200": parse_stream("A" * 200),
        "chunk15-200": parse_stream("A" * 200),
        "chunk16-200": parse_stream("A" * 80 + "\r\n" + "A" * 80 + "\r\n" + "A" * 40),
        "char-200": parse_stream("A" * 80 + "\r\n" + "A" * 80 + "\r\n" + "A" * 40),
    }
    ok, lines = summarise_same_text(results)
    assert not ok
    assert any("manufactured by ConPTY" in ln for ln in lines)

    # Identical structure everywhere must report nothing.
    same = {c.name: parse_stream("A" * 200) for c in GROUP_SAME_TEXT}
    ok, lines = summarise_same_text(same)
    assert ok, lines

    total = sum(len(c.writes) for c in CASES)
    print(f"self-test ok: {len(CASES)} cases, {total} writes")


if __name__ == "__main__":
    _self_test()
