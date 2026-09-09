# Coverage task: `internal/vtvibe` VFS paths

Origin: Section 22.3 custom task. After the provider coverage step, Codecov
reported 61.31% overall coverage and 67.77% for `internal/vtvibe` (813 lines).
The remaining low-coverage area was the in-memory VFS and tree adapter.

The added tests cover path normalization and titles, directory listing and
aliases, stat/open behavior, writable-path rules, mutations and protection,
mount lifecycle methods, and reader/writer boundary failures. All test data is
held in an in-memory session.
