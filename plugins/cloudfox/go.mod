module github.com/unxed/f4/plugins/cloudfox

go 1.26.6

// f4#1178 part 1 of 4 (plan: https://github.com/unxed/f4/issues/1178#issuecomment-5851218447).
//
// CloudFox is its own module so its cloud SDKs (aws-sdk-go-v2,
// google.golang.org/api, golang.org/x/oauth2, golang.org/x/net/webdav,
// github.com/zalando/go-keyring, ~30 MB together) never enter the main
// github.com/unxed/f4 module's build list. It still uses the main module's
// vfs and sdk/f4plugin packages (a plain in-repo path, not a separate
// checkout), hence the replace below.
//
// This go.mod is intentionally not hand-tidied: per this repo's own policy
// (no local `go build`/`go mod tidy` -- see the build-cloudfox-plugin CI
// job in .github/workflows/build.yml), the require block below was written
// by reading plugins/cloudfox's imports rather than by running the Go
// toolchain. That CI job runs `go mod tidy` before building, which is the
// actual source of truth for the resulting go.mod/go.sum; committing its
// output back (or correcting anything it changes here) is expected
// follow-up, not a sign this file is wrong on arrival.
require (
	github.com/aws/aws-sdk-go-v2 v1.43.7
	github.com/aws/aws-sdk-go-v2/config v1.32.38
	github.com/aws/aws-sdk-go-v2/credentials v1.19.37
	github.com/aws/aws-sdk-go-v2/feature/s3/manager v1.22.42
	github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager v0.3.15
	github.com/aws/aws-sdk-go-v2/service/s3 v1.107.3
	github.com/aws/smithy-go v1.27.8
	github.com/google/uuid v1.6.0
	github.com/unxed/f4 v0.0.0
	github.com/unxed/vtinput v0.1.8
	github.com/unxed/vtui v0.1.367
	github.com/vmihailenco/msgpack/v5 v5.4.1
	github.com/zalando/go-keyring v0.2.8
	golang.org/x/crypto v0.56.0
	golang.org/x/net v0.58.0
	golang.org/x/oauth2 v0.36.0
	google.golang.org/api v0.264.0
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
