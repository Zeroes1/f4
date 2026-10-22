//go:build !windows

package unpack

import "github.com/unxed/f4/vfs"

// sudoAvailable says whether a step the system refused is retried through
// the root dispatcher that sudo starts.
func sudoAvailable() bool {
	return vfs.GetSudoClient().IsAvailable()
}
