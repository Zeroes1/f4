//go:build !linux

package main

import (
	"errors"
	"time"
)

// Record is only implemented for Linux; other hosts have their own probes.
func Record([]string, time.Duration, Options) (Recording, error) {
	return Recording{}, errors.New("record is implemented on Linux only")
}

// HostProbes is only implemented for Linux.
func HostProbes([]string, HostOptions) ([]Result, error) {
	return nil, errors.New("host probes are implemented on Linux only")
}
