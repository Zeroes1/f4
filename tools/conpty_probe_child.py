"""Write one probe case to the console it is attached to, and nothing else.

Run inside a pseudoconsole by conpty_probe.py / conpty_probe_bundled.py:

    python tools/conpty_probe_child.py <case-name> [side-file]

Why this exists rather than `cmd /c echo`:

* `echo` gives no control over how the text is split into WriteConsoleW
  calls, and that split is the whole subject of the experiment;
* `echo <text> >CONOUT$` appends a space before the redirection operator,
  which shifts the payload by one column;
* `echo` always appends CRLF, so the output would contain an application
  line break and every break in the stream would need attributing.

Here the payload is written with WriteConsoleW and contains only printable
characters, so any CR or LF in the resulting ConPTY stream came from ConPTY.

The console mode is forced to the legacy path - ENABLE_PROCESSED_OUTPUT and
ENABLE_WRAP_AT_EOL_OUTPUT set, ENABLE_VIRTUAL_TERMINAL_PROCESSING cleared -
because with VT processing on, writes go through WriteCharsVT (passthrough)
and never reach the code under test.

Diagnostics go to the side file, never to the console, so the stream stays
clean. Exit code 0 means the payload was written in full.
"""

import ctypes
import os
import sys
import traceback
from ctypes import wintypes

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import conpty_wrap_cases as cases  # noqa: E402

GENERIC_READ = 0x80000000
GENERIC_WRITE = 0x40000000
FILE_SHARE_READ = 0x00000001
FILE_SHARE_WRITE = 0x00000002
OPEN_EXISTING = 3
INVALID_HANDLE_VALUE = wintypes.HANDLE(-1).value

ENABLE_PROCESSED_OUTPUT = 0x0001
ENABLE_WRAP_AT_EOL_OUTPUT = 0x0002
ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004
DISABLE_NEWLINE_AUTO_RETURN = 0x0008

EXIT_OK = 0
EXIT_USAGE = 2
EXIT_FAILED = 3


class COORD(ctypes.Structure):
    _fields_ = [("X", ctypes.c_short), ("Y", ctypes.c_short)]


class SMALL_RECT(ctypes.Structure):
    _fields_ = [("Left", ctypes.c_short), ("Top", ctypes.c_short),
                ("Right", ctypes.c_short), ("Bottom", ctypes.c_short)]


class CONSOLE_SCREEN_BUFFER_INFO(ctypes.Structure):
    _fields_ = [("dwSize", COORD),
                ("dwCursorPosition", COORD),
                ("wAttributes", wintypes.WORD),
                ("srWindow", SMALL_RECT),
                ("dwMaximumWindowSize", COORD)]


k32 = ctypes.WinDLL("kernel32", use_last_error=True)
k32.CreateFileW.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD,
                            ctypes.c_void_p, wintypes.DWORD, wintypes.DWORD,
                            wintypes.HANDLE]
k32.CreateFileW.restype = wintypes.HANDLE
k32.WriteConsoleW.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                              ctypes.POINTER(wintypes.DWORD), ctypes.c_void_p]
k32.WriteConsoleW.restype = wintypes.BOOL
k32.GetConsoleMode.argtypes = [wintypes.HANDLE, ctypes.POINTER(wintypes.DWORD)]
k32.GetConsoleMode.restype = wintypes.BOOL
k32.SetConsoleMode.argtypes = [wintypes.HANDLE, wintypes.DWORD]
k32.SetConsoleMode.restype = wintypes.BOOL
k32.GetConsoleScreenBufferInfo.argtypes = [
    wintypes.HANDLE, ctypes.POINTER(CONSOLE_SCREEN_BUFFER_INFO)]
k32.GetConsoleScreenBufferInfo.restype = wintypes.BOOL
k32.CloseHandle.argtypes = [wintypes.HANDLE]
k32.CloseHandle.restype = wintypes.BOOL


def _cursor(handle):
    info = CONSOLE_SCREEN_BUFFER_INFO()
    if not k32.GetConsoleScreenBufferInfo(handle, ctypes.byref(info)):
        return None
    return info


def main():
    if len(sys.argv) < 2:
        print(__doc__)
        return EXIT_USAGE

    name = sys.argv[1]
    side = sys.argv[2] if len(sys.argv) > 2 else None
    notes = [f"case:   {name}", f"python: {sys.version.split()[0]}"]

    def flush_notes(code):
        notes.append(f"exit:   {code}")
        if side:
            try:
                os.makedirs(os.path.dirname(side) or ".", exist_ok=True)
                with open(side, "w", encoding="utf-8") as f:
                    f.write("\n".join(notes) + "\n")
            except OSError:
                pass
        return code

    case = cases.BY_NAME.get(name)
    if case is None:
        notes.append(f"error:  unknown case, known: {sorted(cases.BY_NAME)}")
        return flush_notes(EXIT_USAGE)

    handle = k32.CreateFileW("CONOUT$", GENERIC_READ | GENERIC_WRITE,
                             FILE_SHARE_READ | FILE_SHARE_WRITE, None,
                             OPEN_EXISTING, 0, None)
    if not handle or handle == INVALID_HANDLE_VALUE:
        notes.append(f"error:  CreateFileW(CONOUT$) -> {ctypes.get_last_error()}")
        return flush_notes(EXIT_FAILED)

    try:
        mode = wintypes.DWORD()
        if k32.GetConsoleMode(handle, ctypes.byref(mode)):
            notes.append(f"mode in:  0x{mode.value:08x}")
            wanted = ((mode.value | ENABLE_PROCESSED_OUTPUT
                       | ENABLE_WRAP_AT_EOL_OUTPUT)
                      & ~ENABLE_VIRTUAL_TERMINAL_PROCESSING
                      & ~DISABLE_NEWLINE_AUTO_RETURN)
            if wanted != mode.value and k32.SetConsoleMode(handle, wanted):
                notes.append(f"mode out: 0x{wanted:08x} (legacy write path)")
            else:
                notes.append(f"mode out: 0x{mode.value:08x} (unchanged)")
        else:
            notes.append(f"warn:   GetConsoleMode -> {ctypes.get_last_error()}")

        before = _cursor(handle)
        if before:
            notes.append(f"buffer: {before.dwSize.X}x{before.dwSize.Y}, "
                         f"window {before.srWindow.Right - before.srWindow.Left + 1}"
                         f"x{before.srWindow.Bottom - before.srWindow.Top + 1}")
            notes.append(f"cursor before: ({before.dwCursorPosition.X}, "
                         f"{before.dwCursorPosition.Y})")

        written_total = 0
        for chunk in case.writes:
            buf = ctypes.create_unicode_buffer(chunk)
            written = wintypes.DWORD()
            ok = k32.WriteConsoleW(handle, buf, len(chunk),
                                   ctypes.byref(written), None)
            if not ok:
                notes.append(f"error:  WriteConsoleW -> {ctypes.get_last_error()}")
                return flush_notes(EXIT_FAILED)
            written_total += written.value

        notes.append(f"writes: {len(case.writes)} call(s), "
                     f"{written_total} of {len(case.text)} chars")

        after = _cursor(handle)
        if after:
            notes.append(f"cursor after:  ({after.dwCursorPosition.X}, "
                         f"{after.dwCursorPosition.Y})")

        if written_total != len(case.text):
            return flush_notes(EXIT_FAILED)
        return flush_notes(EXIT_OK)
    finally:
        k32.CloseHandle(handle)


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception:
        sys.stderr.write(traceback.format_exc())
        sys.exit(EXIT_FAILED)
