"""Write 200 'A' to CONOUT$ in <n>-char WriteConsoleW calls, nothing else.

    python tools/conpty_probe_child.py <n>

No CR, no LF, no escape sequence in the payload, so any line break in the
ConPTY output came from ConPTY.
"""

import ctypes
import sys
from ctypes import wintypes

TOTAL = 200
LEGACY_MODE = 0x0003  # PROCESSED_OUTPUT | WRAP_AT_EOL; VT processing off, so
                      # the write goes through WriteCharsLegacy, not passthrough

k32 = ctypes.WinDLL("kernel32", use_last_error=True)
k32.CreateFileW.argtypes = [wintypes.LPCWSTR, wintypes.DWORD, wintypes.DWORD,
                            ctypes.c_void_p, wintypes.DWORD, wintypes.DWORD,
                            wintypes.HANDLE]
k32.CreateFileW.restype = wintypes.HANDLE  # a HANDLE is 64-bit; without this
                                           # ctypes truncates it to an int
k32.SetConsoleMode.argtypes = [wintypes.HANDLE, wintypes.DWORD]
k32.WriteConsoleW.argtypes = [wintypes.HANDLE, ctypes.c_void_p, wintypes.DWORD,
                              ctypes.POINTER(wintypes.DWORD), ctypes.c_void_p]

n = int(sys.argv[1])
h = k32.CreateFileW("CONOUT$", 0xC0000000, 3, None, 3, 0, None)
if h == wintypes.HANDLE(-1).value:
    sys.exit(f"CreateFileW(CONOUT$) failed: {ctypes.get_last_error()}")
k32.SetConsoleMode(h, LEGACY_MODE)

text = "A" * TOTAL
for i in range(0, TOTAL, n):
    chunk = text[i:i + n]
    written = wintypes.DWORD()
    if not k32.WriteConsoleW(h, ctypes.create_unicode_buffer(chunk),
                             len(chunk), ctypes.byref(written), None):
        sys.exit(f"WriteConsoleW failed: {ctypes.get_last_error()}")
