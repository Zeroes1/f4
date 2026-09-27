//go:build !lite

package plughost

import (
	"github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/plugins/netfox"
	sqliteplugin "github.com/unxed/f4/plugins/sqlite"
)

// optionalVFSPlugins are the VFS providers a lite build cuts down or drops
// (f4#1178): the native-library archive plugin goes entirely, netfox keeps
// only FISH+ (over a subprocess ssh dialer instead of this build's
// golang.org/x/crypto/ssh one) in place of the FTP/SFTP/FISH+ trio here,
// and the SQLite client goes entirely. The SQLite client is the last thing
// that would link github.com/ncruces/go-sqlite3 into a lite build once
// internal/sheet and unxed/tar's archive index have their sqlite-free
// backends there (store_lite.go, tarindex_simple), and that engine alone is
// about 7 MB of the binary. See plugins_lite.go for the other half of this
// build tag's single point of truth.
//
// Cloud storage (plugins/cloudfox: S3, Google Drive, Yandex Disk, WebDAV)
// no longer lives here at all, in either build. It moved out to its own
// module and its own subprocess RPC plugin binary
// (plugins/cloudfox/cmd/cloudfox-plugin, plugins/cloudfox/go.mod) so its
// ~30 MB of cloud SDKs (aws-sdk-go-v2, google.golang.org/api,
// golang.org/x/oauth2, github.com/zalando/go-keyring) never enter this
// module's build list, full build included (f4#1178, part 1 of the plan at
// https://github.com/unxed/f4/issues/1178#issuecomment-5851218447). f4
// itself still runs it exactly the way it runs any other native plugin --
// see docs/PLUGINS.md and PlugRing (internal/plughost/plugring.go); wiring
// an actual install path is part 2/3 of that plan, not done here.
func optionalVFSPlugins() []Plugin {
	return []Plugin{
		&archive.ArchivePlugin{},
		&netfox.NetFoxPlugin{},
		sqliteplugin.NewPlugin(),
	}
}
