//go:build !windows

package netbrowse

// enumerateNetwork has nothing to ask off Windows.
func enumerateNetwork(*resource) ([]resource, error) { return nil, errUnsupported }
