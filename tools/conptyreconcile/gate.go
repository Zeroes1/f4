package main

import (
	"fmt"
	"path/filepath"
)

// runNativeGate is deliberately a composition of native runs. There is no
// simulated terminal path: each stage must pass through the pinned host.
func runNativeGate(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-gate.json")
	}
	if err := runNativeProbe(hostPath, reportPath+".static", false); err != nil {
		return fmt.Errorf("static pinned-host stage: %w", err)
	}
	if err := runNativeProbe(hostPath, reportPath+".dynamic", true); err != nil {
		return fmt.Errorf("dynamic pinned-host stage: %w", err)
	}
	return fmt.Errorf("native gate incomplete: native transport artifacts passed, but logical history, reflow, extreme-condition, command, and 300-session assertions are not implemented")
}
