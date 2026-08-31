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


def _check(ok, what):
    if not ok:
        err = ctypes.get_last_error()
        raise OSError(f"{what} failed, GetLastError={err}")


def load_create_pseudo_console(dll_path):
    """Return (callable, exported_name) from the given conpty.dll."""
    dll = ctypes.WinDLL(dll_path, use_last_error=True)
    for name in ("ConptyCreatePseudoConsole", "CreatePseudoConsole"):
        try:
            fn = getattr(dll, name)
        except AttributeError:
            continue
        fn.restype = ctypes.c_long  # HRESULT
        fn.argtypes = [COORD, wintypes.HANDLE, wintypes.HANDLE,
                       wintypes.DWORD, ctypes.POINTER(ctypes.c_void_p)]
        return dll, fn, name
    raise OSError(f"no CreatePseudoConsole export found in {dll_path}")


def run(dll_path):
    dll, create_pc, export_name = load_create_pseudo_console(dll_path)

    sa = SECURITY_ATTRIBUTES()
    sa.nLength = ctypes.sizeof(sa)
    sa.bInheritHandle = True

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

    _check(k32.CreateProcessW(
        None, ctypes.create_unicode_buffer(CMD), None, None, False,
        EXTENDED_STARTUPINFO_PRESENT, None, None,
        ctypes.byref(siex), ctypes.byref(pi)), "CreateProcessW")

    # Our copies of the pseudoconsole's ends must go, or the read never ends.
    k32.CloseHandle(in_read)
    k32.CloseHandle(out_write)

    chunks = []
    buf = (ctypes.c_char * 4096)()
    got = wintypes.DWORD()
    while True:
        ok = k32.ReadFile(out_read, buf, 4096, ctypes.byref(got), None)
        if not ok or got.value == 0:
            if ctypes.get_last_error() in (0, ERROR_BROKEN_PIPE):
                break
            break
        chunks.append(bytes(buf[:got.value]))

    k32.CloseHandle(pi.hThread)
    k32.CloseHandle(pi.hProcess)
    k32.CloseHandle(in_write)
    k32.CloseHandle(out_read)
    return b"".join(chunks), export_name


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
        raw, export_name = run(dll_path)
    except Exception:
        import traceback
        say("--- FAILED ---")
        say(traceback.format_exc())
        with open(f"{OUTDIR}/report-{tag}.txt", "w", encoding="utf-8") as f:
            f.write("\n".join(report) + "\n")
        raise

    say(f"export used: {export_name}")
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
