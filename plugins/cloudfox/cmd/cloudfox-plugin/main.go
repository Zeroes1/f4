// Command cloudfox-plugin is the standalone subprocess entry point for
// CloudFox (f4#1178, part 1 of 4 of the plan posted at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851218447).
//
// It links the same S3 / Google Drive / Yandex Disk / WebDAV provider code
// that used to be registered in-process by internal/plughost's full build
// (plugins/cloudfox), now built into its own binary via plugins/cloudfox's
// own go.mod so the cloud SDKs it needs -- aws-sdk-go-v2,
// google.golang.org/api, golang.org/x/oauth2, golang.org/x/net/webdav,
// github.com/zalando/go-keyring (~30 MB together) -- never touch f4's own
// module graph or binary, in either the full or the lite build. f4 launches
// this binary as a native OS subprocess and talks to it over the standard
// F4-RPC transport (docs/PLUGINS.md, sdk/f4plugin), exactly like any other
// subprocess plugin (see plugins/dummy_rpc/main.go for the minimal
// reference example this mirrors).
//
// See plugring-manifest.json in this directory for the declarative
// manifest PlugRing needs to offer this as an installable plugin -- wiring
// that manifest into an actual download/install flow (building and
// publishing release binaries per platform, adding a catalog or
// first-party "Install cloud storage support" entry) is part 2/3 of the
// plan above, not part 1.
package main

import (
	"github.com/unxed/f4/internal/config"
	"github.com/unxed/f4/plugins/cloudfox"
	"github.com/unxed/f4/sdk/f4plugin"
)

func main() {
	f4plugin.Run(cloudfox.NewRPCPlugin(cloudfox.Options{
		ConfigDir: config.GetF4ConfigDir(),
		Portable:  config.IsPortableProfile(),
	}))
}
