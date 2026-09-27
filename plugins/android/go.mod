module github.com/unxed/f4/plugins/android

go 1.26.6

// f4#1178 Android plugin, part 1 of 4 (mirrors plugins/cloudfox's own part 1
// of 4; plan and this ticket's Android-specific decision:
// https://github.com/unxed/f4/issues/1178#issuecomment-5851392645).
//
// plugins/android (ADB device browsing over shell-v2/FISH+ or ADB Sync) is
// its own module for the same reason cloudfox and (eventually) iOS are: a
// single, uniform "download this plugin on demand" mechanism across all
// three, per the owner's explicit request in the comment above -- NOT
// because this package carries unique heavy dependencies of its own. It
// does not: plugins/android's own files import nothing beyond the standard
// library (net, os/exec, encoding/*, ...) and this repo's own vfs,
// sdk/f4plugin and netfox packages -- see rpc_plugin.go's package comment
// for the full accounting, including plugins/netfox, which android/device.go
// and fish_pool.go reuse for the FISH+ session/pool machinery (the same
// pattern plugins/multiarc uses for archivers: wrapping an existing in-repo
// package rather than vendoring its own copy).
//
// That reuse is also why -tags lite matters for this module specifically,
// unlike plugins/cloudfox (which does not depend on netfox at all):
// plugins/netfox itself is a shared package, not its own module, and its
// non-FISH+ files (FTP, SFTP, the SSH dialer, Pageant) are gated
// //go:build lite/!lite rather than split out. Building this module's own
// binary WITHOUT -tags lite would silently link golang.org/x/crypto/ssh,
// github.com/pkg/sftp, github.com/jlaffaye/ftp and github.com/kbolino/pageant
// into android-plugin even though plugins/android's own code never
// references any of them (see device.go/fish_pool.go: only
// netfox.FishVFS/fishplus, never netfox's FTP/SFTP/SSH-dialer types). The
// build-android-plugin CI job in .github/workflows/build.yml therefore
// passes -tags lite, which keeps this module's actual external dependency
// surface down to go-runewidth, vtinput and vtui (plus vtui's own ffi/goffi
// forks below) -- see cmd/f4/lite_deps_test.go's
// TestLiteBuildExcludesHeavyNetworkDependencies for the same mechanical
// check applied to the main lite build.
//
// This go.mod is intentionally not hand-tidied: per this repo's own policy
// (no local `go build`/`go mod tidy` -- see the build-android-plugin CI job
// in .github/workflows/build.yml, mirroring build-cloudfox-plugin), the
// require block below was written by reading plugins/android's (and, via
// plugins/netfox, its transitive) imports rather than by running the Go
// toolchain. That CI job runs `go mod tidy` before building, which is the
// actual source of truth for the resulting go.mod/go.sum; committing its
// output back (or correcting anything it changes here) is expected
// follow-up, not a sign this file is wrong on arrival.
require (
	github.com/mattn/go-runewidth v0.0.15
	github.com/unxed/f4 v0.0.0
	github.com/unxed/vtinput v0.1.8
	github.com/unxed/vtui v0.1.367
)

replace github.com/unxed/f4 => ../..

// Same forks the root module uses (see ../../go.mod) -- vtui transitively
// needs ffi.Available, which only exists in unxed/pureffi, not upstream
// ebitengine/purego. Without these, `go mod tidy` here resolves the
// vanilla upstream modules instead and the build fails with
// "undefined: ffi.Available".
replace github.com/ebitengine/purego => github.com/unxed/pureffi v0.1.20

replace github.com/ebitengine/hideconsole => ../../internal/hideconsole

replace github.com/go-webgpu/goffi => github.com/unxed/goffi v0.1.11
