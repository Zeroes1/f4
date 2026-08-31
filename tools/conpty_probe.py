"""Probe: does ConPTY inject a line break at the wrap point?

Runs `cmd /c echo <N chars>` inside a pseudoconsole of a known width and
reports where line breaks appear in the resulting VT stream.

Meant to be run on a real Windows host (see the conpty-probe workflow),
not locally on Linux.

Everything printed here is also written to probe-out/, which the workflow
uploads as an artifact.
"""

import os
import sys

from winpty import PtyProcess

COLS = 80
ROWS = 25
FILL = 100  # > COLS, so the line must wrap once
OUTDIR = "probe-out"


def collect(proc):
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


def hexdump(data, limit=2048):
    lines = []
    data = data[:limit]
    for off in range(0, len(data), 16):
        row = data[off:off + 16]
        hexpart = " ".join(f"{b:02x}" for b in row)
        txt = "".join(chr(b) if 32 <= b < 127 else "." for b in row)
        lines.append(f"  {off:04x}  {hexpart:<47}  {txt}")
    return "\n".join(lines)


def main():
    import winpty

    os.makedirs(OUTDIR, exist_ok=True)
    report = []

    def say(line=""):
        print(line, flush=True)
        report.append(line)

    say(f"pywinpty {getattr(winpty, '__version__', '?')}")
    say(f"python   {sys.version.split()[0]}")
    say(f"pty      {COLS}x{ROWS}, echoing {FILL} 'A'")
    say()

    proc = PtyProcess.spawn(
        f"cmd /c echo {'A' * FILL}",
        dimensions=(ROWS, COLS),
    )
    stream = collect(proc)
    raw = stream.encode("utf-8", errors="replace")

    with open(f"{OUTDIR}/stream.bin", "wb") as f:
        f.write(raw)

    say("--- raw stream ---")
    say(hexdump(raw))
    say(f"(total {len(raw)} bytes, dump truncated at 2048)")
    say()

    runs = [len(r) for r in stream.split("\r\n") if "A" in r]
    say("--- analysis ---")
    say(f"runs of text between CRLFs: {runs}")

    if any(n >= FILL for n in runs):
        say(f"VERDICT: long line passed through whole ({FILL} chars, no break)")
    elif COLS in runs:
        say(f"VERDICT: break injected at column {COLS}")
    else:
        say("VERDICT: unrecognised - see the dump above")

    with open(f"{OUTDIR}/report.txt", "w", encoding="utf-8") as f:
        f.write("\n".join(report) + "\n")


if __name__ == "__main__":
    main()
