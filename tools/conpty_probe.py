"""Probe the in-box ConPTY for manufactured line breaks, through pywinpty.

Same case matrix as conpty_probe_bundled.py, but driven through pywinpty
instead of raw CreatePseudoConsole. That keeps a second, independent consumer
in the picture: whatever shows up here is what a third-party pty wrapper
sees, not an artifact of how this repo calls the Win32 API.

The payload of every case is printable text only - no CR, no LF, no tab, no
escape sequence - so every break in the output was inserted by ConPTY. See
conpty_wrap_cases.py for the reasoning and for the code in conhost this is
aimed at.

Meant to run on a real Windows host (see the conpty-probe workflow). Output
goes to probe-out/, which the workflow uploads as an artifact.
"""

import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import conpty_wrap_cases as cases  # noqa: E402

COLS = cases.COLS
ROWS = cases.ROWS
OUTDIR = cases.OUTDIR
CHILD = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                     "conpty_probe_child.py")


def collect(proc):
    """Drain a PtyProcess until the child is gone and the pipe is quiet."""
    out = []
    while True:
        try:
            chunk = proc.read()
        except EOFError:
            break
        if not chunk:
            if not proc.isalive():
                break
            continue
        out.append(chunk)
    return "".join(out)


def hexdump(data, limit=512):
    lines = []
    clipped = data[:limit]
    for off in range(0, len(clipped), 16):
        row = clipped[off:off + 16]
        hexpart = " ".join(f"{b:02x}" for b in row)
        txt = "".join(chr(b) if 32 <= b < 127 else "." for b in row)
        lines.append(f"    {off:04x}  {hexpart:<47}  {txt}")
    if len(data) > limit:
        lines.append(f"    ... {len(data) - limit} more bytes")
    return "\n".join(lines)


def main():
    from winpty import Backend, PtyProcess
    import winpty

    os.makedirs(OUTDIR, exist_ok=True)
    report = []

    def say(line=""):
        print(line, flush=True)
        report.append(line)

    say(f"pywinpty {getattr(winpty, '__version__', '?')}")
    say(f"python   {sys.version.split()[0]}")
    say(f"pty      {COLS}x{ROWS}")
    say(f"cases    {len(cases.CASES)}")
    say()

    results = {}
    group = None
    for case in cases.CASES:
        if case.group != group:
            group = case.group
            say(f"--- {group} ---")

        side = os.path.join(OUTDIR, f"child-pywinpty-{case.name}.txt")
        argv = f'"{sys.executable}" "{CHILD}" {case.name} "{side}"'
        try:
            proc = PtyProcess.spawn(argv, dimensions=(ROWS, COLS),
                                    backend=Backend.ConPTY)
        except Exception as exc:
            say(f"  {case.name:<14} FAILED to spawn: {exc}")
            continue

        stream = collect(proc)
        exit_code = proc.wait()
        raw = stream.encode("utf-8", errors="replace")
        with open(os.path.join(OUTDIR, f"stream-pywinpty-{case.name}.bin"),
                  "wb") as f:
            f.write(raw)

        parsed = cases.parse_stream(raw)
        results[case.name] = parsed
        say(cases.describe(case, parsed))
        if exit_code:
            say(f"                 child exited {exit_code}")
    say()

    say("--- sample dump: the two cases that write the same 200 chars ---")
    for name in ("bulk-200", "chunk16-200"):
        path = os.path.join(OUTDIR, f"stream-pywinpty-{name}.bin")
        if os.path.exists(path):
            with open(path, "rb") as f:
                say(f"  {name}:")
                say(hexdump(f.read()))
    say()

    say("--- same text, different buffering ---")
    _ok, lines = cases.summarise_same_text(results)
    for line in lines:
        say(line)
    say()
    say("--- length sweep ---")
    for line in cases.summarise_lengths(results):
        say(line)
    say()

    with open(os.path.join(OUTDIR, "report.txt"), "w", encoding="utf-8") as f:
        f.write("\n".join(report) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
