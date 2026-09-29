//go:build !linux

package dirwatch

import "errors"

// newNativeSource: no OS notification mechanism is used off Linux yet; Watch
// falls back to polling.
func newNativeSource(string) (*rawSource, error) {
	return nil, errors.New("dirwatch: no native watcher on this OS")
}
