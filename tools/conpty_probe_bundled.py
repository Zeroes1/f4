"""Probe ConPTY for manufactured line breaks, in-box and bundled.

Runs the case matrix from conpty_wrap_cases.py through a pseudoconsole and
reports, for every case, where line breaks landed in the VT stream.

The payload of every case is printable text only - no CR, no LF, no tab, no
escape sequence - so every break in the output was inserted by ConPTY. See
conpty_wrap_cases.py for the reasoning and for the code in conhost this is
aimed at.

The same code drives both pseudoconsoles: "system" exercises the in-box
CreatePseudoConsole as a control, and the bundled conpty.dll exercises a
post-#17510 build. conpty.dll spawns OpenConsole.exe from its own directory,
so both files must sit side by side.

Usage: python tools/conpty_probe_bundled.py <path-to-conpty.dll> [tag]

Meant to run on a real Windows host (see the conpty-probe workflow). Output
goes to probe-out/, which the workflow uploads as an artifact.
"""

import ctypes
import os
import sys
import threading
import time
from ctypes import wintypes

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import conpty_wrap_cases as cases  # noqa: E402

COLS = cases.COLS
ROWS = cases.ROWS
OUTDIR = cases.OUTDIR
CHILD = os.path.join(os.path.dirname(os.path.abspath(__file__)),
                     "conpty_probe_child.py")

PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x00020016
EXTENDED_STARTUPINFO_PRESENT = 0x00080000
STILL_ACTIVE = 259


class COORD(ctypes.Structure):
    _fields_ = [("X", ctypes.c_short), ("Y", ctypes.c_short)]


class STARTUPINFOW(ctypes.Structure):
    _fields_ = [
        ("cb", wintypes.DWORD),
        ("lpReserved", wintypes.LPWSTR),
        ("lpDesktop", wintypes.LPWSTR),
        ("lpTitle", wintypes.LPWSTR),
        ("dwX", wintypes.DWORD),
        ("dwY", wintypes.DWORD),
        ("dwXSize", wintypes.DWORD),
        ("dwYSize", wintypes.DWORD),
        ("dwXCountChars", wintypes.DWORD),
        ("dwYCountChars", wintypes.DWORD),
        ("dwFillAttribute", wintypes.DWORD),
        ("dwFlags", wintypes.DWORD),
        ("wShowWindow", wintypes.WORD),
        ("cbReserved2", wintypes.WORD),
        ("lpReserved2", ctypes.POINTER(ctypes.c_byte)),
        ("hStdInput", wintypes.HANDLE),
        ("hStdOutput", wintypes.HANDLE),
        ("hStdError", wintypes.HANDLE),
    ]


class STARTUPINFOEXW(ctypes.Structure):
    _fields_ = [
        ("StartupInfo", STARTUPINFOW),
        ("lpAttributeList", ctypes.c_void_p),
    ]


class PROCESS_INFORMATION(ctypes.Structure):
    _fields_ = [
        ("hProcess", wintypes.HANDLE),
        ("hThread", wintypes.HANDLE),
        ("dwProcessId", wintypes.DWORD),
        ("dwThreadId", wintypes.DWORD),
    ]


k32 = ctypes.WinDLL("kernel32", use_last_error=True)

# Spell out every signature. Without argtypes, ctypes pushes plain Python
# ints as 32-bit, while UpdateProcThreadAttribute declares dwFlags and
# Attribute as DWORD_PTR: the call then "succeeds" with a mangled attribute
# and the child quietly keeps the parent's console instead of the pty.
PSIZE_T = ctypes.POINTER(ctypes.c_size_t)
k32.CreatePipe.argtypes = [ctypes.POINTER(wintypes.HANDLE),
                           ctypes.POINTER(wintypes.HANDLE),
                           ctypes.c_void_p, wintypes.DWORD]
k32.CreatePipe.restype = wintypes.BOOL
k32.InitializeProcThreadAttributeList.argtypes = [
    ctypes.c_void_p, wintypes.DWORD, wintypes.DWORD, PSIZE_T]
k32.InitializeProcThreadAttributeList.restype = wintypes.BOOL
k32.UpdateProcThreadAttribute.argtypes = [
    ctypes.c_void_p, ctypes.c_size_t, ctypes.c_size_t, ctypes.c_void_p,
    ctypes.c_size_t, ctypes.c_void_p, PSIZE_T]
k32.UpdateProcThreadAttribute.restype = wintypes.BOOL
k32.DeleteProcThreadAttributeList.argtypes = [ctypes.c_void_p]
k32.DeleteProcThreadAttributeList.restype = None
k32.CreateProcessW.argtypes = [
    wintypes.LPCWSTR, wintypes.LPWSTR, ctypes.c_void_p, ctypes.c_void_p,
    wintypes.BOOL, wintypes.DWORD, ctypes.c_void_p, wintypes.LPCWSTR,
    ctypes.POINTER(STARTUPINFOEXW), ctypes.POINTER(PROCESS_INFORMATION)]
k32.CreateProcessW.restype = wintypes.BOOL
k32.ReadFile.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                         ctypes.POINTER(wintypes.DWORD), ctypes.c_void_p]
k32.ReadFile.restype = wintypes.BOOL
k32.WaitForSingleObject.argtypes = [wintypes.HANDLE, wintypes.DWORD]
k32.WaitForSingleObject.restype = wintypes.DWORD
k32.GetExitCodeProcess.argtypes = [wintypes.HANDLE,
                                   ctypes.POINTER(wintypes.DWORD)]
k32.GetExitCodeProcess.restype = wintypes.BOOL
k32.CloseHandle.argtypes = [wintypes.HANDLE]
k32.CloseHandle.restype = wintypes.BOOL
k32.GetStdHandle.argtypes = [wintypes.DWORD]
k32.GetStdHandle.restype = wintypes.HANDLE
k32.SetHandleInformation.argtypes = [wintypes.HANDLE, wintypes.DWORD,
                                     wintypes.DWORD]
k32.SetHandleInformation.restype = wintypes.BOOL

HANDLE_FLAG_INHERIT = 0x1
STD_HANDLES = (-10, -11, -12)  # input, output, error


def detach_parent_std_handles():
    """Stop our own stdio from reaching the child.

    Attaching a pseudoconsole gives the child the right *console* - cmd.exe
    sets its title through it - but its std handles still come from us. When
    the parent's stdout is a pipe (a CI log, say) the child writes there and
    the pty stream carries nothing but the ConPTY handshake. EchoCon never
    hits this because its parent owns a real console.
    """
    for std in STD_HANDLES:
        h = k32.GetStdHandle(std)
        if h and h != wintypes.HANDLE(-1).value:
            k32.SetHandleInformation(h, HANDLE_FLAG_INHERIT, 0)


def _check(ok, what):
    if not ok:
        err = ctypes.get_last_error()
        raise OSError(f"{what} failed, GetLastError={err}")


def load_conpty(dll_path):
    """Return (dll, create, close, exported_name).

    dll_path may be "system" to exercise the in-box pseudoconsole through
    exactly the same code, which is the control for this experiment.
    """
    dll = k32 if dll_path == "system" else ctypes.WinDLL(dll_path,
                                                        use_last_error=True)
    create = close = None
    used = None
    for name in ("ConptyCreatePseudoConsole", "CreatePseudoConsole"):
        if hasattr(dll, name):
            create = getattr(dll, name)
            used = name
            break
    if create is None:
        raise OSError(f"no CreatePseudoConsole export found in {dll_path}")
    create.restype = ctypes.c_long  # HRESULT
    create.argtypes = [COORD, wintypes.HANDLE, wintypes.HANDLE,
                       wintypes.DWORD, ctypes.POINTER(ctypes.c_void_p)]
    for name in ("ConptyClosePseudoConsole", "ClosePseudoConsole"):
        if hasattr(dll, name):
            close = getattr(dll, name)
            break
    if close is not None:
        close.restype = None
        close.argtypes = [ctypes.c_void_p]
    return dll, create, close, used


class Result:
    def __init__(self, raw, exit_code, note):
        self.raw = raw
        self.exit_code = exit_code
        self.note = note


def run(conpty, command, timeout_s=20, quiesce_s=0.75):
    """Run `command` in a fresh pseudoconsole and return everything it emits."""
    _dll, create_pc, close_pc, _export = conpty

    in_read = wintypes.HANDLE()
    in_write = wintypes.HANDLE()
    out_read = wintypes.HANDLE()
    out_write = wintypes.HANDLE()
    _check(k32.CreatePipe(ctypes.byref(in_read), ctypes.byref(in_write),
                          None, 0), "CreatePipe(in)")
    _check(k32.CreatePipe(ctypes.byref(out_read), ctypes.byref(out_write),
                          None, 0), "CreatePipe(out)")

    hpc = ctypes.c_void_p()
    hr = create_pc(COORD(COLS, ROWS), in_read, out_write, 0, ctypes.byref(hpc))
    if hr != 0:
        raise OSError(f"CreatePseudoConsole returned HRESULT "
                      f"0x{hr & 0xFFFFFFFF:08x}")

    size = ctypes.c_size_t(0)
    k32.InitializeProcThreadAttributeList(None, 1, 0, ctypes.byref(size))
    attrs = (ctypes.c_byte * size.value)()
    _check(k32.InitializeProcThreadAttributeList(attrs, 1, 0,
                                                 ctypes.byref(size)),
           "InitializeProcThreadAttributeList")
    _check(k32.UpdateProcThreadAttribute(
        attrs, 0, ctypes.c_size_t(PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE),
        hpc, ctypes.sizeof(hpc), None, None), "UpdateProcThreadAttribute")

    siex = STARTUPINFOEXW()
    siex.StartupInfo.cb = ctypes.sizeof(STARTUPINFOEXW)
    siex.lpAttributeList = ctypes.cast(attrs, ctypes.c_void_p)
    pi = PROCESS_INFORMATION()

    # CreatePseudoConsole dups these into conhost, so drop our copies now
    # (this is what samples/ConPTY/EchoCon does); otherwise nothing ever
    # signals end-of-stream on the read side.
    k32.CloseHandle(in_read)
    k32.CloseHandle(out_write)

    detach_parent_std_handles()

    _check(k32.CreateProcessW(
        None, ctypes.create_unicode_buffer(command), None, None, False,
        EXTENDED_STARTUPINFO_PRESENT, None, None,
        ctypes.byref(siex), ctypes.byref(pi)), "CreateProcessW")

    # The pseudoconsole keeps the write end open until it is closed, so a
    # plain blocking read here would hang forever once the child exits.
    # Read on a thread, wait for the child, then close the pseudoconsole:
    # that drops the write end and the reader falls out on a broken pipe.
    chunks = []

    def reader():
        buf = (ctypes.c_char * 4096)()
        got = wintypes.DWORD()
        while True:
            ok = k32.ReadFile(out_read, buf, 4096, ctypes.byref(got), None)
            if not ok or got.value == 0:
                return
            chunks.append(bytes(buf[:got.value]))

    th = threading.Thread(target=reader, daemon=True)
    th.start()

    waited = k32.WaitForSingleObject(pi.hProcess, int(timeout_s * 1000))
    code = wintypes.DWORD(STILL_ACTIVE)
    k32.GetExitCodeProcess(pi.hProcess, ctypes.byref(code))

    # The child exiting does not mean its output has been rendered and
    # pushed through yet; closing the pseudoconsole right away truncates
    # the stream. Wait for the byte count to stop growing instead.
    settled = 0.0
    last = -1
    while settled < quiesce_s:
        now = len(chunks)
        if now != last:
            last = now
            settled = 0.0
        time.sleep(0.05)
        settled += 0.05

    if close_pc is not None:
        close_pc(hpc)
    th.join(5)

    k32.DeleteProcThreadAttributeList(ctypes.cast(attrs, ctypes.c_void_p))
    k32.CloseHandle(pi.hThread)
    k32.CloseHandle(pi.hProcess)
    k32.CloseHandle(in_write)
    k32.CloseHandle(out_read)

    note = f"wait={waited} exit={code.value} chunks={len(chunks)}"
    if close_pc is None:
        note += "; no ClosePseudoConsole export, stream may be truncated"
    if th.is_alive():
        note += "; reader still blocked, reporting what arrived"
    return Result(b"".join(chunks), code.value, note)


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


def probe(label, dll_path, say, keep_dumps):
    """Run every case against one pseudoconsole; return {case name: Parsed}."""
    say(f"================ {label} ================")
    say(f"dll: {dll_path}")
    try:
        conpty = load_conpty(dll_path)
    except OSError as exc:
        say(f"  FAILED to load: {exc}")
        say()
        return {}
    say(f"export used: {conpty[3]}")
    say(f"pty: {COLS}x{ROWS}")
    say()

    results = {}
    group = None
    for case in cases.CASES:
        if case.group != group:
            group = case.group
            say(f"--- {group} ---")
        side = os.path.join(OUTDIR, f"child-{label}-{case.name}.txt")
        command = f'"{sys.executable}" "{CHILD}" {case.name} "{side}"'
        try:
            res = run(conpty, command)
        except OSError as exc:
            say(f"  {case.name:<14} FAILED: {exc}")
            continue

        if keep_dumps:
            with open(os.path.join(OUTDIR, f"stream-{label}-{case.name}.bin"),
                      "wb") as f:
                f.write(res.raw)

        parsed = cases.parse_stream(res.raw)
        results[case.name] = parsed
        say(cases.describe(case, parsed))
        if res.exit_code != 0:
            say(f"                 child exited {res.exit_code} ({res.note})")
    say()

    say("--- sample dump: the two cases that write the same 200 chars ---")
    for name in ("bulk-200", "chunk16-200"):
        path = os.path.join(OUTDIR, f"stream-{label}-{name}.bin")
        if keep_dumps and os.path.exists(path):
            with open(path, "rb") as f:
                say(f"  {name}:")
                say(hexdump(f.read()))
    say()

    say("--- same text, different buffering ---")
    ok, lines = cases.summarise_same_text(results)
    for line in lines:
        say(line)
    say()
    say("--- length sweep ---")
    for line in cases.summarise_lengths(results):
        say(line)
    say()
    return results


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        return 2
    bundled = os.path.abspath(sys.argv[1])
    tag = sys.argv[2] if len(sys.argv) > 2 else "bundled"

    os.makedirs(OUTDIR, exist_ok=True)
    report = []

    def say(line=""):
        print(line, flush=True)
        report.append(line)

    say(f"python  {sys.version.split()[0]}")
    say(f"cases   {len(cases.CASES)}")
    say()

    for label, path in (("system", "system"), (tag, bundled)):
        try:
            probe(label, path, say, keep_dumps=True)
        except Exception:
            import traceback
            say(f"--- {label} FAILED ---")
            say(traceback.format_exc())
        with open(os.path.join(OUTDIR, f"report-{tag}.txt"), "w",
                  encoding="utf-8") as f:
            f.write("\n".join(report) + "\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
