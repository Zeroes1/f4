//go:build !linux

package main

import (
	"errors"
	"time"
)

// Record is only implemented for Linux; other hosts have their own probes.
func Record([]string, time.Duration) ([]byte, int, error) {
	return nil, 0, errors.New("record is implemented on Linux only")
}
