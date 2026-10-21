package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
)

const iosDriveName = "iOS"
const iosPlugRingID = "ios"

func init() {
	if liteBuild {
		registerIOSPluginMenuDrive()
	}
}

// Lite offers installation until the loaded plugin replaces this factory.
func registerIOSPluginMenuDrive() {
	sysinfo.RegisterDrive(iosDriveName, iosPluginMenuDriveFactory)
}

func iosPluginMenuDriveFactory() vfs.VFS {
	if pf := panel.FindPanelsFrameAnyScreen(); pf != nil {
		actionPlugRingFocused(pf, iosPlugRingID)
	}
	return nil
}
