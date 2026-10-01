# CI: TestDesktopEntryMatchesThePackagedOne on Windows

Run 36864825515 (HEAD d9f9a85): `internal/install` failed on `windows/amd64 A-D`
and `windows/arm64`; every line of the two launchers matched once `Exec=`,
`TryExec=` and `X-F4-Managed=` were stripped, so the difference was invisible:
the Windows checkout (core.autocrlf) gave `packaging/linux/org.unxed.f4.desktop`
CRLF endings while `DesktopEntry` emits LF.

Fix: `*.desktop text eol=lf` in `.gitattributes`. The test is the regression test.

## Open, not fixed here

`windows/arm64` also failed `internal/terminal`
`TestConPTYPackageKeepsLongLinesWhole/powershell`: `powershell.exe` did not
finish in 90s, then the temp `conpty.dll` could not be unlinked (still loaded).
Looks like a slow or hung PowerShell on the arm64 runner, not tied to this
change; it was not seen on amd64. Check whether it repeats before touching it.
