//go:build !windows

package dialog

import (
	"fmt"

	"github.com/unxed/f4/vfs"
)

func setWindowsFileCompression(v vfs.VFS, path string, compressed bool) error {
	return fmt.Errorf("NTFS compression is available only on Windows")
}
