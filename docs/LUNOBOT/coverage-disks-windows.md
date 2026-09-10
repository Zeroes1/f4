# Coverage: vfs/disks_windows.go

Status: Windows-only tests cover the safe helper and preflight paths without opening or enumerating a physical disk.

Covered behavior:
- adding and preserving the Windows device-path prefix;
- obtaining a size from a seekable file and restoring its offset;
- rejecting an embedded-NUL device path before native I/O;
- honoring a canceled context before device enumeration.

Invariant: disk discovery and size probing remain best-effort and never turn a failed native probe into a fabricated device size.
