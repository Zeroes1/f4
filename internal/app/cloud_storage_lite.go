package app

import (
	"github.com/unxed/f4/internal/panel"
	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
)

const cloudStorageDriveName = "CloudFox"
const cloudStoragePlugRingID = "cloudfox"

func init() {
	if liteBuild {
		registerCloudStorageLiteDrive()
	}
}

// Lite offers installation until the loaded plugin replaces this factory.
func registerCloudStorageLiteDrive() {
	sysinfo.RegisterDrive(cloudStorageDriveName, cloudStorageLiteDriveFactory)
}

func cloudStorageLiteDriveFactory() vfs.VFS {
	if pf := panel.FindPanelsFrameAnyScreen(); pf != nil {
		actionPlugRingFocused(pf, cloudStoragePlugRingID)
	}
	return nil
}
