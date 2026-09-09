# Coverage task: `plugins/sqlite`

Origin: Section 22.3 custom task. At task start, Codecov reported 61.06%
overall coverage and 64.78% for `plugins/sqlite` (869 lines).

The added tests exercise SQLite display formatting and truncation, selected
database path validation, browser state boundaries, the F4 cell-edit gesture,
empty-schema refreshes, and idempotent plugin closing. They use temporary
SQLite databases and the in-memory silent UI screen; no external tools are
required.
