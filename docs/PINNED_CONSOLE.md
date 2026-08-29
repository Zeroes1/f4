# The pinned console host

**The host f4 bundles and the host every measurement must be made against:**

| | |
|---|---|
| Binary | `OpenConsole.exe`, x64 |
| Version | `1.12.10983.0` |
| SHA-256 | `14e0857b37f6c5e5e90bab786a4db8fceb4166afe75e617519d942656976481e` |
| Source tag | [`v1.12.10982.0`](https://github.com/microsoft/terminal/tree/v1.12.10982.0), commit `e9b4e2e18fb1b9cee6839969d42cd0f95d228926` |
| Origin | `Microsoft.WindowsTerminal_Win11_1.12.10983.0_8wekyb3d8bbwe.msixbundle`, `CascadiaPackage_1.12.10983.0_x64.msix` |

Every port in `tools/conptyreconcile` cites this file. A port that cites any
other version of Microsoft's source is a violation of THE RULE, not a detail:
between this tag and today's `main` the console's buffer model was replaced
(`CharRow` became `ROW` with `_charOffsets`) and the frame emitter was
rewritten (`VtEngine` became `VtIo::Writer`, which emits neither `ESC[K` nor
the XTWINOPS size report). Code ported from `main` describes a console that
nobody runs.

## Why this version and no other

The maintainer's machine is Windows 11 Pro `10.0.22000.2538`. Every
measurement this project rests on -- above all the long lines that motivated
the whole direction -- was taken there. We have no evidence about any other
build, so bundling anything else would mean shipping behaviour we have never
observed.

`v1.12.10982.0` is the public release whose OpenConsole matches that machine's
console generation, and matching it is checked rather than assumed: the field
dumps in `tools/conptyreconcile/testdata` carry the XTWINOPS size report at
the head of a repaint frame and `ESC[K` after short rows but not after full
ones (P14, P6). A candidate that does not reproduce that signature is the
wrong candidate.

## How the probe uses it

`conptyreconcile.exe` creates its pseudoconsole through the ported
`winconpty.cpp` path (`msconpty.go`), which prefers an `OpenConsole.exe`
sitting beside the executable and falls back to the inbox `conhost.exe`. Drop
the two files in one directory and run; there is no flag.

Every log now opens with the host it actually used. If the header says
`INBOX conhost -- NOT the pinned host`, the run measured the machine rather
than the bundled host, and its results answer a different question.

## What is still open

The inbox conhost of a given Windows build cannot be mapped to a public
commit: inbox binaries are built from Microsoft's internal OS repository and
that mapping is not published. This is exactly why the pin is a bundled
OpenConsole from a public tag rather than a Windows build number -- the tag is
knowable, the inbox commit is not.
