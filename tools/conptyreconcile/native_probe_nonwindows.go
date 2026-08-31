//go:build !windows

package main

import (
	"fmt"
	"runtime"
)

func runNativeProbe(hostPath, reportPath string) error {
	return fmt.Errorf("native OpenConsole probe requires Windows (current OS: %s)", runtime.GOOS)
}
