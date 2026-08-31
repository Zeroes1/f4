//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type seedGateReport struct {
	Mode        string               `json:"mode"`
	Host        pinnedHostIdentity   `json:"host"`
	SeedCount   int                  `json:"seed_count"`
	Sessions    []nativeProbeSession `json:"sessions"`
	CompletedAt time.Time            `json:"completed_at"`
}

func runNativeSeedGate(hostPath, reportPath string) error {
	resolved, err := ensureProbeHost(hostPath)
	if err != nil {
		return err
	}
	identity, err := verifyPinnedHost(resolved)
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	report := seedGateReport{Mode: "pinned-conpty-seeds", Host: identity}
	widths := []int{1, 79, 80, 81, 121}
	for i := 0; i < 300; i++ {
		seed := uint64(i + 1)
		width := widths[i%len(widths)]
		workload := seedWorkload(seed, width)
		begin := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_BEGIN__", seed)
		end := fmt.Sprintf("__PINNED_CONPTY_PROBE_SEED_%016x_END__", seed)
		command := fmt.Sprintf(`"%s" -emit-seed %016x -emit-probe-width %d`, executable, seed, width)
		session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, width, 25, true, workload, command, []string{begin, end})
		report.Sessions = append(report.Sessions, session)
		artifactDir := reportPath + ".sessions"
		if err := os.MkdirAll(artifactDir, 0o755); err != nil {
			return err
		}
		artifact := filepath.Join(artifactDir, fmt.Sprintf("%03d-%dx25.raw", i+1, width))
		if err := os.WriteFile(artifact, session.RawOutput, 0o644); err != nil {
			return err
		}
		if runErr != nil {
			_ = writeJSON(reportPath, report)
			return fmt.Errorf("seed %d width %d: %w", seed, width, runErr)
		}
	}
	report.SeedCount = len(report.Sessions)
	report.CompletedAt = time.Now().UTC()
	return writeJSON(reportPath, report)
}
