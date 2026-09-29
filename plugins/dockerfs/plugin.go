// Package dockerfs shows Docker containers in a file panel: the containers are
// the folders at the top, and inside one is its own file system, read through
// the Engine API's archive endpoint (the one `docker cp` uses).
//
// The panel is read-only for now. It needs a reachable daemon (unix socket or
// tcp:// DOCKER_HOST) and nothing else: no Docker CLI, no SDK, no CGO.
package dockerfs

import (
	"errors"

	"github.com/unxed/f4/vfs"
)

// driveName is what the drive menu (Alt+F1) calls the panel.
const driveName = "Docker"

// Plugin registers the Docker drive.
type Plugin struct{}

// NewPlugin constructs the built-in Docker plugin.
func NewPlugin() *Plugin { return &Plugin{} }

func (*Plugin) GetName() string { return "Docker" }

// Init only registers the drive: no connection is made until the panel is
// opened, so a machine without Docker pays nothing.
func (*Plugin) Init(api vfs.HostAPI) error {
	if api == nil {
		return errors.New("Docker: nil host API")
	}
	api.RegisterDrive(driveName, func() vfs.VFS { return newDockerVFS(clientFromEnv) })
	return nil
}

func (*Plugin) Close() error { return nil }
