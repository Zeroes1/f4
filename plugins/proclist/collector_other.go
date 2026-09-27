//go:build !linux

package proclist

import (
	"errors"

	"github.com/unxed/f4/vfs"
)

// Supported reports whether this build can collect a real process list. v1
// (f4#312 part 1 of 4) is Linux-only by design; part 2 adds the other
// platforms (Windows via Toolhelp/NtQuerySystemInformation, macOS/*BSD via
// sysctl(KERN_PROC), none of it through WMI -- see plugin.go's package
// comment).
func Supported() bool { return false }

// newProcListPanel is unreachable in practice: Plugin.Init checks
// Supported() before ever registering it as a panel provider's Open
// callback. It still needs a real, correctly-typed body so this file
// satisfies the same shape as collector_linux.go's, the way
// plugins/ios/core_access_stub.go mirrors core_access_supported.go.
func newProcListPanel(vfs.PanelContext) (vfs.PanelController, error) {
	return nil, errors.New("ProcList: the process list is only available on Linux in this version")
}
