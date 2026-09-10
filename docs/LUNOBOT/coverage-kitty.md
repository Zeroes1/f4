# Kitty graphics coverage

This change adds focused tests for the Kitty graphics receiver in internal/terminal/kitty.go.

The tests cover malformed control values and chunked payloads, invalid image data,
safe file and temporary-file handling, display errors, image-number lookup, replacement,
range deletion, and orphan cleanup. Validation runs in the repository GitHub Actions workflow.