//go:build !windows

package dialog

import (
	"fmt"

	"github.com/unxed/f4/vfs"
)

func setWindowsFileSparse(v vfs.VFS, path string, sparse bool) error {
	return fmt.Errorf("NTFS sparse files are unavailable on this platform")
}
