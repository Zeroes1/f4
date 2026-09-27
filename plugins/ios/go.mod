module github.com/unxed/f4/plugins/ios

go 1.26.6

// f4#1178 iOS plugin, part 1 of 4 (mirrors the CloudFox pattern: see
// plugins/cloudfox/go.mod and the plan/decision at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851326218 and
// https://github.com/unxed/f4/issues/1178#issuecomment-5851392645).
//
// iOS is its own module so go-ios (github.com/danielpaulus/go-ios) and its
// entirely separate dependency chain -- gvisor.dev/gvisor (a full userspace
// TCP/IP stack), quic-go, vishvananda/netlink+netns, songgao/water,
// miekg/dns, grandcat/zeroconf, howett.net/plist, go.mozilla.org/pkcs7,
// software.sslmate.com/src/go-pkcs12, golang.zx2c4.com/wintun (~49 MB in the
// module cache) -- never enter the main github.com/unxed/f4 module's build
// list, in either the full or the lite build. It still uses the main
// module's vfs and sdk/f4plugin packages (a plain in-repo path, not a
// separate checkout), hence the replace below.
//
// This go.mod is intentionally not hand-tidied: per this repo's own policy
// (no local `go build`/`go mod tidy` -- see the build-ios-plugin CI job in
// .github/workflows/build.yml), the require block below was written by
// reading plugins/ios's imports rather than by running the Go toolchain.
// That CI job runs `go mod tidy` before building, which is the actual
// source of truth for the resulting go.mod/go.sum; committing its output
// back (or correcting anything it changes here) is expected follow-up, not
// a sign this file is wrong on arrival.
require (
	github.com/Masterminds/semver v1.5.0
	github.com/danielpaulus/go-ios v1.2.2-0.20260805152531-ebec9a0b076c
	github.com/google/uuid v1.6.0
	github.com/unxed/f4 v0.0.0
	github.com/unxed/vtinput v0.1.8
)

replace github.com/unxed/f4 => ../..
