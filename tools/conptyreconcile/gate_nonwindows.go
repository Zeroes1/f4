//go:build !windows

package main

import "fmt"

func runNativeSeedGate(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native seed gate requires Windows")
}
