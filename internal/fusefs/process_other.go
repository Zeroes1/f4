//go:build plan9 || js

package fusefs

import "os"

// processExists relies on FindProcess, which really does look the process up
// on the platforms this file covers.
func processExists(pid int) bool {
	p, err := os.FindProcess(pid)
	return err == nil && p != nil
}
