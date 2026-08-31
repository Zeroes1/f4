"""Probe a bundled (post-rewrite) ConPTY for injected line breaks.

Same experiment as conpty_probe.py, but instead of the in-box pseudoconsole
it loads a conpty.dll brought along with the repo, so we can compare the
behaviour of a modern ConPTY against the one shipped with the OS.

conpty.dll spawns OpenConsole.exe from its own directory, so both files must
sit side by side.

Usage: python tools/conpty_probe_bundled.py <path-to-conpty.dll> [tag]
"""

import ctypes
import os
import sys
import threading
import time
from ctypes import wintypes

COLS = 80
ROWS = 25
FILL = 100  # > COLS, so the line must wrap once
OUTDIR = "probe-out"
CMD = f'cmd /c echo {"A" * FILL}'

PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE = 0x00020016
EXTENDED_STARTUPINFO_PRESENT = 0x00080000
ERROR_BROKEN_PIPE = 109


class COORD(ctypes.Structure):
    _fields_ = [("X", ctypes.c_short), ("Y", ctypes.c_short)]


class SECURITY_ATTRIBUTES(ctypes.Structure):
    _fields_ = [
        ("nLength", wintypes.DWORD),
        ("lpSecurityDescriptor", ctypes.c_void_p),
        ("bInheritHandle", wintypes.BOOL),
    ]


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
k32.CloseHandle.argtypes = [wintypes.HANDLE]
k32.CloseHandle.restype = wintypes.BOOL


def _check(ok, what):
    if not ok:
        err = ctypes.get_last_error()
        raise OSError(f"{what} failed, GetLastError={err}")


def load_conpty(dll_path):
    """Return (dll, create, close, exported_name) from the given conpty.dll."""
    dll = ctypes.WinDLL(dll_path, use_last_error=True)
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


def run(dll_path, timeout_s=20):
    dll, create_pc, close_pc, export_name = load_conpty(dll_path)

    in_read = wintypes.HANDLE()
    in_write = wintypes.HANDLE()
    out_read = wintypes.HANDLE()
    out_write = wintypes.HANDLE()
    _check(k32.CreatePipe(ctypes.byref(in_read), ctypes.byref(in_write), None, 0),
           "CreatePipe(in)")
    _check(k32.CreatePipe(ctypes.byref(out_read), ctypes.byref(out_write), None, 0),
           "CreatePipe(out)")

    hpc = ctypes.c_void_p()
    hr = create_pc(COORD(COLS, ROWS), in_read, out_write, 0, ctypes.byref(hpc))
    if hr != 0:
        raise OSError(f"{export_name} returned HRESULT 0x{hr & 0xFFFFFFFF:08x}")

    size = ctypes.c_size_t(0)
    k32.InitializeProcThreadAttributeList(None, 1, 0, ctypes.byref(size))
    attrs = (ctypes.c_byte * size.value)()
    _check(k32.InitializeProcThreadAttributeList(attrs, 1, 0, ctypes.byref(size)),
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

    _check(k32.CreateProcessW(
        None, ctypes.create_unicode_buffer(CMD), None, None, False,
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

    # The child exiting does not mean its output has been rendered and
    # pushed through yet; closing the pseudoconsole right away truncates
    # the stream. Wait for the byte count to stop growing instead.
    settled = 0
    last = -1
    while settled < 3.0:
        now = len(chunks)
        if now != last:
            last = now
            settled = 0
        time.sleep(0.1)
        settled += 0.1

    if close_pc is not None:
        close_pc(hpc)
    th.join(5)

    k32.DeleteProcThreadAttributeList(ctypes.cast(attrs, ctypes.c_void_p))
    k32.CloseHandle(pi.hThread)
    k32.CloseHandle(pi.hProcess)
    k32.CloseHandle(in_write)
    k32.CloseHandle(out_read)

    note = f"child wait -> {waited}, {len(chunks)} chunk(s)"
    if close_pc is None:
        note += "; no ClosePseudoConsole export, stream may be truncated"
    if th.is_alive():
        note += "; reader still blocked, reporting what arrived"
    return b"".join(chunks), export_name, note


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
    if len(sys.argv) < 2:
        print(__doc__)
        sys.exit(2)
    dll_path = os.path.abspath(sys.argv[1])
    tag = sys.argv[2] if len(sys.argv) > 2 else "bundled"

    os.makedirs(OUTDIR, exist_ok=True)
    report = []

    def say(line=""):
        print(line, flush=True)
        report.append(line)

    say(f"conpty.dll  {dll_path}")
    say(f"python      {sys.version.split()[0]}")
    say(f"pty         {COLS}x{ROWS}, echoing {FILL} 'A'")
    say()

    try:
        raw, export_name, note = run(dll_path)
    except Exception:
        import traceback
        say("--- FAILED ---")
        say(traceback.format_exc())
        with open(f"{OUTDIR}/report-{tag}.txt", "w", encoding="utf-8") as f:
            f.write("\n".join(report) + "\n")
        raise

    say(f"export used: {export_name}")
    say(f"note:        {note}")
    say()

    with open(f"{OUTDIR}/stream-{tag}.bin", "wb") as f:
        f.write(raw)

    say("--- raw stream ---")
    say(hexdump(raw))
    say(f"(total {len(raw)} bytes, dump truncated at 2048)")
    say()

    text = raw.decode("utf-8", errors="replace")
    runs = [len(r) for r in text.split("\r\n") if "A" in r]
    say("--- analysis ---")
    say(f"runs of text between CRLFs: {runs}")

    if any(n >= FILL for n in runs):
        say(f"VERDICT: long line passed through whole ({FILL} chars, no break)")
    elif COLS in runs:
        say(f"VERDICT: break injected at column {COLS}")
    else:
        say("VERDICT: unrecognised - see the dump above")

    with open(f"{OUTDIR}/report-{tag}.txt", "w", encoding="utf-8") as f:
        f.write("\n".join(report) + "\n")


if __name__ == "__main__":
    main()
