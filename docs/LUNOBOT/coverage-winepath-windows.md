# Coverage: internal/gui/winepath_windows.go

Status: test coverage for the platform-independent helper paths is added in the companion Windows test.

Covered behavior:
- conversion of NUL-terminated UTF-8 and UTF-16 buffers;
- empty paths, embedded-NUL conversion failures, and the unavailable-translation result;
- the zero-pointer freeProcessHeap guard.

Invariant: Wine path conversion remains best-effort. If Wine exports are unavailable or reject a path, callers receive ("", false) and retain their existing fallback behavior.
