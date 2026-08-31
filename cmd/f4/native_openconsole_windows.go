//go:build windows

package main

// This bridge deliberately reuses the audited standalone pinned-host probe
// while the native f4 transport is being factored into this package. It does
// not expose a console model: the returned bytes are fed through the real
// PanelsFrame -> AnsiParser -> TerminalView path by the integration test.
//
// Keeping the bridge explicit prevents the test from silently falling back to
// PTY/NewPTY (which would use the system ConPTY host). The standalone probe
// performs the pinned path/version/SHA and live-process identity checks.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type nativeOpenConsoleF4Session struct {
	InitialWidth  int                        `json:"initial_width"`
	InitialHeight int                        `json:"initial_height"`
	HostProcess   nativeHostProof            `json:"host_process"`
	RawOutput     []byte                     `json:"raw_output"`
	Events        []nativeOpenConsoleF4Event `json:"events"`
	ExitCode      uint32                     `json:"exit_code"`
	Error         string                     `json:"error,omitempty"`
}

type nativeOpenConsoleF4Event struct {
	Sequence uint64 `json:"sequence"`
	Kind     uint8  `json:"kind"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	Bytes    []byte `json:"bytes"`
}

type nativeHostProof struct {
	Path    string `json:"path"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

type nativeOpenConsoleF4Report struct {
	Host          nativeHostProof              `json:"host"`
	ExpectedInput []byte                       `json:"expected_input"`
	Sessions      []nativeOpenConsoleF4Session `json:"sessions"`
}

func nativeOpenConsoleGo() (string, error) {
	if candidate := os.Getenv("F4_GO"); candidate != "" {
		return candidate, nil
	}
	for _, candidate := range []string{
		`C:\Users\Windows\AppData\Local\Temp\f4-go1.26.6\go\bin\go.exe`,
		"go.exe",
	} {
		if candidate == "go.exe" {
			if found, err := exec.LookPath(candidate); err == nil {
				return found, nil
			}
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("Go executable is unavailable; set F4_GO for native OpenConsole gate")
}

func runNativeOpenConsoleF4Probe(reportPath string) (nativeOpenConsoleF4Report, error) {
	var report nativeOpenConsoleF4Report
	if runtime.GOOS != "windows" {
		return report, fmt.Errorf("native OpenConsole f4 gate requires Windows")
	}
	goExe, err := nativeOpenConsoleGo()
	if err != nil {
		return report, err
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		return report, err
	}
	toolDir := filepath.Join(root, "tools", "conptyreconcile")
	probeExe := filepath.Join(os.TempDir(), "f4-native-openconsole-f4-probe.exe")
	build := exec.Command(goExe, "build", "-o", probeExe, ".")
	build.Dir = toolDir
	build.Env = append(os.Environ(), "GOCACHE="+systemGoBuildCache())
	if output, err := build.CombinedOutput(); err != nil {
		return report, fmt.Errorf("build pinned native probe: %w: %s", err, output)
	}
	defer os.Remove(probeExe)
	// Both modes are part of the gate: static isolates host repaint from
	// ResizePseudoConsole, while the normal run exercises resize events.
	for _, mode := range []struct {
		flag string
		path string
	}{
		{flag: "-probe-static", path: reportPath + ".static"},
		{flag: "-probe", path: reportPath},
	} {
		run := exec.Command(probeExe, mode.flag, "-probe-report", mode.path)
		run.Dir = toolDir
		if output, err := run.CombinedOutput(); err != nil {
			return report, fmt.Errorf("run pinned native probe %s: %w: %s", mode.flag, err, output)
		}
		data, err := os.ReadFile(mode.path)
		if err != nil {
			return report, err
		}
		var part nativeOpenConsoleF4Report
		if err := json.Unmarshal(data, &part); err != nil {
			return report, fmt.Errorf("decode native probe report %s: %w", mode.flag, err)
		}
		if report.Host.Path == "" {
			report.Host = part.Host
			report.ExpectedInput = part.ExpectedInput
		} else if report.Host != part.Host || string(report.ExpectedInput) != string(part.ExpectedInput) {
			return report, fmt.Errorf("native probe identity/payload differs between static and resize runs")
		}
		report.Sessions = append(report.Sessions, part.Sessions...)
	}
	if report.Host.Path == "" || report.Host.Version == "" || report.Host.SHA256 == "" {
		return report, fmt.Errorf("native probe report has incomplete pinned host identity")
	}
	for _, session := range report.Sessions {
		if session.ExitCode != 0 || session.Error != "" {
			return report, fmt.Errorf("native session %dx%d failed: exit=%d error=%s", session.InitialWidth, session.InitialHeight, session.ExitCode, session.Error)
		}
		if session.HostProcess != report.Host {
			return report, fmt.Errorf("native session %dx%d host identity differs from report host", session.InitialWidth, session.InitialHeight)
		}
	}
	return report, nil
}

// systemGoBuildCache is intentionally the machine-wide cache mandated by
// AGENTS.md. A missing value is left empty so Go reports its own diagnostic;
// this bridge never redirects builds into the repository or a temp directory.
func systemGoBuildCache() string {
	if value := os.Getenv("GOCACHE"); value != "" {
		return value
	}
	return `C:\Users\Windows\AppData\Local\go-build`
}
