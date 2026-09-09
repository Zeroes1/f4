# Coverage task: `internal/vtvibe`

Origin: Section 22.3 custom task. At task start, Codecov reported 61.22%
overall coverage and 54.49% for `internal/vtvibe` (813 lines). The provider
file itself was only 4.61% covered.

The added tests exercise OpenAI-compatible Chat and Models requests, endpoint
normalization and authorization, local versus remote key handling, malformed
and empty provider responses, typed content decoding, HTTP error formatting,
retry cancellation, and retry backoff. HTTP behavior uses local `httptest`
servers only.
