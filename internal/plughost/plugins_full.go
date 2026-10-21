//go:build !lite

package plughost

import (
	androidfs "github.com/unxed/f4/plugins/android"
	"github.com/unxed/f4/plugins/archive"
	"github.com/unxed/f4/plugins/cloudfox"
	"github.com/unxed/f4/plugins/dockerfs"
	iosfs "github.com/unxed/f4/plugins/ios"
	"github.com/unxed/f4/plugins/k8sfs"
	"github.com/unxed/f4/plugins/mongofs"
	"github.com/unxed/f4/plugins/netfox"
)

// optionalVFSPlugins registers the plugins included in the full build.
// Lite builds replace archives and networking with smaller providers and
// install Android, iOS and cloud storage separately through PlugRing.
func optionalVFSPlugins() []Plugin {
	return []Plugin{
		&archive.ArchivePlugin{},
		androidfs.NewPlugin(),
		iosfs.NewPlugin(),
		cloudfox.NewPlugin(),
		&netfox.NetFoxPlugin{},
		// Docker containers as a read-only drive (f4#1663); talks to the
		// daemon over its unix socket with the standard library only.
		dockerfs.NewPlugin(),
		// Kubernetes namespaces, pods and containers as a read-only drive
		// (f4#1663); the API server over the standard library and x/net.
		k8sfs.NewPlugin(),
		// MongoDB databases, collections and documents as a read-only drive
		// (f4#1663); the wire protocol is spoken directly, no driver.
		mongofs.NewPlugin(),
	}
}

// BuiltinPlugRingItem reports whether a downloadable first-party plugin is
// already included in this build, so an installed copy cannot replace it.
func BuiltinPlugRingItem(id string) bool {
	switch id {
	case "android", "ios", "cloudfox":
		return true
	default:
		return false
	}
}
