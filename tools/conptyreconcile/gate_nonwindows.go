//go:build !windows

package main

import "fmt"

func runNativeSeedGate(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native seed gate requires Windows")
}

func runNativeSingleSeed(hostPath, reportPath string, seed uint64) error {
	_ = hostPath
	_ = reportPath
	_ = seed
	return fmt.Errorf("native single-seed probe requires Windows")
}

func runNativePartialProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native partial probe requires Windows")
}

func runNativeCommandProbe(hostPath, reportPath string) error {
	_ = hostPath
	_ = reportPath
	return fmt.Errorf("native command probe requires Windows")
}
