# Issue #1184 solution review — office documents opened as archives

Report: `.docx`, `.xlsx` and probably other office formats do not start on
Enter; the panel enters them as archives instead, because they are ZIP
containers. The default action must be opening by extension association.

## Cause

`ArchiveProvider.CanOpen` asks `zipper/archive.DetectFormat` about the name
first. For `report.docx` that answers `""`, so the probe added for
self-extracting archives takes over: `findEmbeddedArchive` scans the file for
the `PK\x03\x04`, `7z…` and `Rar!…` signatures. An office document carries the
ZIP signature at offset 0, so the probe reports an archive and `CanOpen`
returns true.

`PanelEnterAllowed` then refused ordinary Enter only for `offset > 0`, the
executable-stub case. At offset 0 it allowed Enter, and `list.go` mounted the
document as an archive. Measured on a real `.docx`:

| step | result |
| --- | --- |
| `DetectFormat("report.docx")` | `""` |
| `findEmbeddedArchive` | `{format: zip, suffix: .zip, offset: 0}`, found |
| `CanOpen` | true |
| `PanelEnterAllowed` | true — the bug |

Everything reachable this way shares the shape: `.docx`, `.xlsx`, `.pptx`,
`.odt`, `.jar`, `.apk`, `.epub`. A double click hits the same code, because
`frame.go` turns it into `VK_RETURN`. The content probe arrived with
"archive: open local SFX containers for browsing" and was gated behind
Ctrl+PgDn for `offset > 0` only, which is the gap these files fell into.

## Change

`PanelEnterAllowed` now decides on the name first, on the rule that the name
outranks the content for the default action:

- the name declares an archive format — Enter opens the archive, as before.
  `nameDeclaresArchive` answers from the name alone, covering what
  `DetectFormat` selects an engine from plus the split-volume names
  (`name.7z.001`, `name.z01`, `name.r00`) that only the content probe
  recognizes. `DetectFormat` itself is not used for this: it falls back to
  reading the file when the extension says nothing, and it is handed a bare
  base name here, so its answer would depend on the process working directory;
- the name declares something else (`.docx`, `.jar`, `.exe`) — Enter goes to
  that extension's association, and Ctrl+PgDn still browses the container;
- the name has no extension at all — nothing to associate, so the content
  decides, minus the self-extracting case, exactly as before;
- the parent is not the local file system — unchanged. Inside another archive
  or on a remote one there is no association and no system opener to hand
  Enter to, so the content stays the only thing to go on.

## Tests

- `plugins/archive/issue1184_test.go`: documents and packages keep `CanOpen`
  (Ctrl+PgDn) but lose ordinary Enter; archive names, split volumes and
  extension-less archives keep it; an SFX stays out of it with and without an
  extension; a table for `nameDeclaresArchive`.
- `internal/panel/issue1184_test.go`: Enter on a `.docx` row is not consumed
  by the panel, reaches `Execute` once, and leaves the VFS alone, while
  Ctrl+PgDn mounts the archive VFS.

Both fail before the change and pass after it. Nothing here is
platform-specific: the reproduction and the fix were measured on Linux.
