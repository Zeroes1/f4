//go:build !windows

package sysinfo

import "os"
import "github.com/unxed/f4/vfs"

func GetPlatformDrives() []DriveEntry {
	home, _ := os.UserHomeDir()
	return []DriveEntry{
		{Name: "/ Root", Factory: func() vfs.VFS { return vfs.NewOSVFS("/") }},
		{Name: "~ Home", Factory: func() vfs.VFS { return vfs.NewOSVFS(home) }},
		{Name: "Physical Disks (/dev)", Factory: func() vfs.VFS { return vfs.NewDisksVFS() }},
	}
}
