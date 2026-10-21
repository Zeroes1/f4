package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
)

const androidDriveName = "Android"
const androidPlugRingID = "android"

func init() {
	if liteBuild {
		registerAndroidDrive()
	}
}

// Lite offers installation until the loaded plugin replaces this factory.
func registerAndroidDrive() {
	sysinfo.RegisterDrive(androidDriveName, androidDriveFactory)
}

func androidDriveFactory() vfs.VFS {
	if pf := panel.FindPanelsFrameAnyScreen(); pf != nil {
		actionPlugRingFocused(pf, androidPlugRingID)
	}
	return nil
}
