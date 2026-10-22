// Package cicheck holds the rules the CI workflows under .github must keep
// and nothing else would catch. actionlint, which the Quality job runs,
// checks that a workflow is valid; a valid workflow can still save a cache
// from a pull request or drop a step a release depends on. The rules live in
// this package's tests, so `go test ./...` enforces them on every run.
package cicheck
