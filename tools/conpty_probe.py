"""Does ConPTY invent line breaks? Same output, four write sizes.

A child writes 200 'A' into an 80-column pseudoconsole, in chunks of 200,
15, 16 and 1 characters. Same text, same screen, four different ways of
handing it to WriteConsoleW. The payload has no CR, no LF and no escape
sequence, so every line break in the ConPTY output was put there by ConPTY.

Since microsoft/terminal#17510, WriteCharsLegacy appends a `\\r\\n` when the
last character of a write lands on the right margin, so whether the logical
line survives depends on whether the write size divides 80.

    python tools/conpty_probe.py [path\\to\\conpty.dll]

With no argument, only the in-box pseudoconsole is probed. Given a
conpty.dll, that one is probed too, as a comparison; conpty.dll spawns
OpenConsole.exe from its own directory, so both files must sit side by side.

Runs on Windows only. Output also goes to probe-out/, which the workflow
uploads as an artifact.
"""

import ctypes
import os
import re
import sys
import threading
import time
from ctypes import wintypes

COLS, ROWS = 80, 25
TOTAL = 200
CHUNKS = (200, 15, 16, 1)
MODES = ("default", "legacy")
OUTDIR = "probe-out"
CHILD = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                     "conpty_probe_child.py")

PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x00020016
EXTENDED_STARTUPINFO_PRESENT = 0x00080000


class COORD(ctypes.Structure):
    _fields_ = [("X", ctypes.c_short), ("Y", ctypes.c_short)]


class STARTUPINFOW(ctypes.Structure):
    _fields_ = [("cb", wintypes.DWORD), ("lpReserved", wintypes.LPWSTR),
                ("lpDesktop", wintypes.LPWSTR), ("lpTitle", wintypes.LPWSTR),
                ("dwX", wintypes.DWORD), ("dwY", wintypes.DWORD),
                ("dwXSize", wintypes.DWORD), ("dwYSize", wintypes.DWORD),
                ("dwXCountChars", wintypes.DWORD),
                ("dwYCountChars", wintypes.DWORD),
                ("dwFillAttribute", wintypes.DWORD), ("dwFlags", wintypes.DWORD),
                ("wShowWindow", wintypes.WORD), ("cbReserved2", wintypes.WORD),
                ("lpReserved2", ctypes.POINTER(ctypes.c_byte)),
                ("hStdInput", wintypes.HANDLE), ("hStdOutput", wintypes.HANDLE),
                ("hStdError", wintypes.HANDLE)]


class STARTUPINFOEXW(ctypes.Structure):
    _fields_ = [("StartupInfo", STARTUPINFOW), ("lpAttributeList", ctypes.c_void_p)]


class PROCESS_INFORMATION(ctypes.Structure):
    _fields_ = [("hProcess", wintypes.HANDLE), ("hThread", wintypes.HANDLE),
                ("dwProcessId", wintypes.DWORD), ("dwThreadId", wintypes.DWORD)]


k32 = ctypes.WinDLL("kernel32", use_last_error=True)

# Spell out every signature. Without argtypes ctypes pushes plain ints as
# 32-bit, while UpdateProcThreadAttribute takes DWORD_PTR: the call then
# "succeeds" with a mangled attribute and the child quietly keeps the
# parent's console instead of the pty.
PSIZE_T = ctypes.POINTER(ctypes.c_size_t)
k32.CreatePipe.argtypes = [ctypes.POINTER(wintypes.HANDLE),
                           ctypes.POINTER(wintypes.HANDLE),
                           ctypes.c_void_p, wintypes.DWORD]
k32.InitializeProcThreadAttributeList.argtypes = [ctypes.c_void_p, wintypes.DWORD,
                                                  wintypes.DWORD, PSIZE_T]
k32.UpdateProcThreadAttribute.argtypes = [ctypes.c_void_p, ctypes.c_size_t,
                                          ctypes.c_size_t, ctypes.c_void_p,
                                          ctypes.c_size_t, ctypes.c_void_p,
                                          PSIZE_T]
k32.DeleteProcThreadAttributeList.argtypes = [ctypes.c_void_p]
k32.CreateProcessW.argtypes = [wintypes.LPCWSTR, wintypes.LPWSTR,
                               ctypes.c_void_p, ctypes.c_void_p, wintypes.BOOL,
                               wintypes.DWORD, ctypes.c_void_p, wintypes.LPCWSTR,
                               ctypes.POINTER(STARTUPINFOEXW),
                               ctypes.POINTER(PROCESS_INFORMATION)]
k32.ReadFile.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                         ctypes.POINTER(wintypes.DWORD), ctypes.c_void_p]
k32.GetStdHandle.restype = wintypes.HANDLE
k32.GetExitCodeProcess.argtypes = [wintypes.HANDLE,
                                   ctypes.POINTER(wintypes.DWORD)]
k32.SetHandleInformation.argtypes = [wintypes.HANDLE, wintypes.DWORD,
                                     wintypes.DWORD]


def load_conpty(path):
    """Return (CreatePseudoConsole, ClosePseudoConsole) from `path`.

    `path` may be "system" to use the in-box pseudoconsole through exactly
    the same code, which is the control for this experiment.
    """
    dll = k32 if path == "system" else ctypes.WinDLL(path, use_last_error=True)
    create = getattr(dll, "ConptyCreatePseudoConsole",
                     getattr(dll, "CreatePseudoConsole", None))
    close = getattr(dll, "ConptyClosePseudoConsole",
                    getattr(dll, "ClosePseudoConsole", None))
    resize = getattr(dll, "ConptyResizePseudoConsole",
                     getattr(dll, "ResizePseudoConsole", None))
    if create is None:
        raise OSError(f"no CreatePseudoConsole export in {path}")
    create.restype = ctypes.c_long  # HRESULT
    create.argtypes = [COORD, wintypes.HANDLE, wintypes.HANDLE, wintypes.DWORD,
                       ctypes.POINTER(ctypes.c_void_p)]
    close.argtypes = [ctypes.c_void_p]
    resize.restype = ctypes.c_long  # HRESULT
    resize.argtypes = [ctypes.c_void_p, COORD]
    return create, close, resize


def run(conpty, chunk, mode, during=None, timeout_s=20):
    """Run the child in a fresh pseudoconsole.

    Returns (stream, console mode the child actually had). The child reports
    the mode through its exit code so nothing extra lands in the stream.
    """
    create_pc, close_pc, _resize_pc = conpty
    hin_r, hin_w = wintypes.HANDLE(), wintypes.HANDLE()
    hout_r, hout_w = wintypes.HANDLE(), wintypes.HANDLE()
    k32.CreatePipe(ctypes.byref(hin_r), ctypes.byref(hin_w), None, 0)
    k32.CreatePipe(ctypes.byref(hout_r), ctypes.byref(hout_w), None, 0)

    hpc = ctypes.c_void_p()
    hr = create_pc(COORD(COLS, ROWS), hin_r, hout_w, 0, ctypes.byref(hpc))
    if hr:
        raise OSError(f"CreatePseudoConsole -> 0x{hr & 0xFFFFFFFF:08x}")

    size = ctypes.c_size_t(0)
    k32.InitializeProcThreadAttributeList(None, 1, 0, ctypes.byref(size))
    attrs = (ctypes.c_byte * size.value)()
    k32.InitializeProcThreadAttributeList(attrs, 1, 0, ctypes.byref(size))
    k32.UpdateProcThreadAttribute(
        attrs, 0, ctypes.c_size_t(PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE),
        hpc, ctypes.sizeof(hpc), None, None)

    siex = STARTUPINFOEXW()
    siex.StartupInfo.cb = ctypes.sizeof(STARTUPINFOEXW)
    siex.lpAttributeList = ctypes.cast(attrs, ctypes.c_void_p)
    pi = PROCESS_INFORMATION()

    # CreatePseudoConsole dups these into conhost, so drop our copies now
    # (this is what samples/ConPTY/EchoCon does); otherwise nothing ever
    # signals end-of-stream on the read side.
    k32.CloseHandle(hin_r)
    k32.CloseHandle(hout_w)

    # Attaching a pseudoconsole gives the child the right console, but its
    # std handles still come from us. When our stdout is a pipe (a CI log)
    # the child writes there and the pty carries only the handshake.
    for std in (-10, -11, -12):
        h = k32.GetStdHandle(std)
        if h and h != wintypes.HANDLE(-1).value:
            k32.SetHandleInformation(h, 1, 0)  # HANDLE_FLAG_INHERIT off

    cmd = (chunk if isinstance(chunk, str)
           else f'"{sys.executable}" "{CHILD}" {chunk} {mode}')
    if not k32.CreateProcessW(None, ctypes.create_unicode_buffer(cmd), None,
                              None, False, EXTENDED_STARTUPINFO_PRESENT, None,
                              None, ctypes.byref(siex), ctypes.byref(pi)):
        raise OSError(f"CreateProcessW -> {ctypes.get_last_error()}")

    # The pseudoconsole holds the write end open, so a blocking read here
    # would hang forever once the child exits. Read on a thread, wait for
    # the child, then close the pseudoconsole to drop the write end.
    out = []

    def reader():
        buf = (ctypes.c_char * 4096)()
        got = wintypes.DWORD()
        while k32.ReadFile(hout_r, buf, 4096, ctypes.byref(got), None) and got.value:
            out.append(bytes(buf[:got.value]))

    th = threading.Thread(target=reader, daemon=True)
    th.start()
    if during is not None:
        during(hpc, lambda: sum(len(c) for c in out))
    k32.WaitForSingleObject(pi.hProcess, int(timeout_s * 1000))

    # The child exiting does not mean its output has been pushed through
    # yet; wait for the byte count to stop growing before closing.
    settled, last = 0.0, -1
    while settled < 0.75:
        if len(out) != last:
            last, settled = len(out), 0.0
        time.sleep(0.05)
        settled += 0.05

    got_mode = wintypes.DWORD()
    k32.GetExitCodeProcess(pi.hProcess, ctypes.byref(got_mode))
    close_pc(hpc)
    th.join(5)
    k32.DeleteProcThreadAttributeList(ctypes.cast(attrs, ctypes.c_void_p))
    for h in (pi.hThread, pi.hProcess, hin_w, hout_r):
        k32.CloseHandle(h)
    return b"".join(out), got_mode.value


CRLF = b"\r\n"
ESCAPES = re.compile(r"\x1b\][^\x07]*\x07|\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b.")


def rows(raw):
    """Strip escape sequences, return the length of each row of text."""
    text = ESCAPES.sub("", raw.decode("utf-8", errors="replace"))
    return [len(r) for r in text.split("\r\n")]


def probe(label, path, say):
    say(f"=== {label} ===")
    conpty = load_conpty(path)
    for mode in MODES:
        shapes = {}
        seen = set()
        for chunk in CHUNKS:
            raw, got_mode = run(conpty, chunk, mode)
            with open(f"{OUTDIR}/stream-{label}-{mode}-{chunk}.bin", "wb") as f:
                f.write(raw)
            got = rows(raw)
            shapes[chunk] = tuple(got)
            seen.add(got_mode)
            calls = -(-TOTAL // chunk)
            plural = "write " if calls == 1 else "writes"
            say(f"  {calls:>3} {plural} of {chunk:<4} -> "
                f"{len(got)} logical line{'' if len(got) == 1 else 's'}: {got}"
                f"{'' if len(got) == 1 else f'   <- {len(got) - 1} CRLF added by ConPTY'}")
        modes = "/".join(f"0x{m:04x}" for m in sorted(seen))
        head = f"  ^ console mode {modes}"
        if len(set(shapes.values())) == 1:
            say(f"{head}: every write size gave {list(shapes[CHUNKS[0]])}")
        else:
            say(f"{head}: the line structure depends on the write size")
        say("")


def probe_cmd(label, conpty, say):
    """What does a real Console API app get? cmd's `type` of a 160-char line.

    160 is two full rows, so the write ends exactly on the margin. On the
    legacy path that earns an injected CRLF on top of the one the file
    already has; on the passthrough path there is only the file's own.
    """
    path = os.path.abspath(f"{OUTDIR}/longline.txt")
    with open(path, "w", newline="\r\n") as f:
        f.write("A" * 160 + "\n")
    # We clear HANDLE_FLAG_INHERIT on our own std handles so the child does
    # not write into the CI log, which leaves cmd without a usable stdout.
    # Our Python child opens CONOUT$ itself; cmd has to be redirected.
    raw, _ = run(conpty, f'cmd /c type "{path}" >CONOUT$', "default")
    with open(f"{OUTDIR}/stream-{label}-cmd-type.bin", "wb") as f:
        f.write(raw)
    breaks = raw.count(b"\r\n")
    got = rows(raw)
    say(f"  cmd /c type (one 160-char line) -> {breaks} CRLF in the stream, "
        f"rows {got}")
    if sum(got) < 160:
        say("    only {} chars of text arrived - cmd never wrote to the pty, "
            "this run measured nothing".format(sum(got)))
    else:
        say("    1 = the file's own newline, passthrough; "
            "2 = one was injected, legacy path")
    say("")


def probe_resize(label, conpty, say):
    """Write one 200-char line, then resize the pty underneath it.

    #17510 says in its own description that forcing the wrap this way
    breaks text reflow on window resize. Nothing above ever resized, so
    that part was never measured. Here the child writes 200 characters as
    one line and then just waits; the parent widens the pty to 100 columns
    and narrows it to 60, recording where in the stream each resize
    happened. What ConPTY repaints in between is the answer: 200 characters
    in a row means the line survived, rows split by CRLF means the repaint
    turned a wrap into a line ending.
    """
    _create, _close, resize_pc = conpty
    marks = []
    errors = []

    def during(hpc, offset):
        # Python takes seconds to start on Windows, and the bundled path
        # spawns its own OpenConsole first. Resizing on a timer resized an
        # empty screen, so wait for the payload to actually show up.
        deadline = time.time() + 20
        while offset() < 200 and time.time() < deadline:
            time.sleep(0.1)
        time.sleep(1.0)
        for cols in (100, 60):
            marks.append((offset(), cols))
            hr = resize_pc(hpc, COORD(cols, ROWS))
            if hr:
                errors.append(f"resize to {cols} -> 0x{hr & 0xFFFFFFFF:08x}")
            time.sleep(2.0)
        marks.append((offset(), None))

    child = f'"{sys.executable}" "{CHILD}" 200 default 40'
    raw, _mode = run(conpty, child, "default", during=during, timeout_s=45)
    with open(f"{OUTDIR}/stream-{label}-resize.bin", "wb") as f:
        f.write(raw)

    say("  200 chars written at 80 columns, then resized:")
    for err in errors:
        say(f"    ResizePseudoConsole FAILED: {err}")
    if not marks or marks[0][0] < 200:
        say("    the payload never reached us before the resize; "
            "this run measured nothing")
    start = 0
    for i, (offset, cols) in enumerate(marks):
        piece = raw[start:offset]
        start = offset
        what = ("before any resize" if i == 0
                else f"after resizing to {marks[i - 1][1]}")
        say(f"    {what:<24} {len(piece):>4} bytes, "
            f"{piece.count(CRLF)} CRLF, rows {rows(piece)}")
    tail = raw[start:]
    if tail:
        say(f"    {'after the last resize':<24} {len(tail):>4} bytes, "
            f"{tail.count(CRLF)} CRLF, rows {rows(tail)}")
    say("    a repaint that keeps the line whole shows one long run; "
        "one that splits it")
    say("    shows rows of the new width separated by CRLF")
    say("")


def main():
    os.makedirs(OUTDIR, exist_ok=True)
    report = []

    def say(line=""):
        print(line, flush=True)
        report.append(line)

    say(f"python {sys.version.split()[0]}, pty {COLS}x{ROWS}, "
        f"{TOTAL} chars per run")
    say("'default' leaves the console mode as ConPTY set it; "
        "'legacy' clears VT processing.")
    say("")
    targets = [("system", "system")]
    if len(sys.argv) > 1:
        targets.append(("bundled", os.path.abspath(sys.argv[1])))

    for label, path in targets:
        try:
            probe(label, path, say)
            probe_cmd(label, load_conpty(path), say)
            probe_resize(label, load_conpty(path), say)
        except Exception as exc:
            say(f"  FAILED: {exc}")
            say("")

    with open(f"{OUTDIR}/report.txt", "w", encoding="utf-8") as f:
        f.write("\n".join(report) + "\n")


if __name__ == "__main__":
    main()
