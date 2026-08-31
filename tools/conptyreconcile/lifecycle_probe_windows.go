//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

type lifecycleCaseReport struct {
	Name          string `json:"name"`
	CloseOrder    string `json:"close_order"`
	ExpectedWait  string `json:"expected_wait"`
	ObservedWait  string `json:"observed_wait"`
	ChildExited   bool   `json:"child_exited"`
	HostExited    bool   `json:"host_exited"`
	HandlesClosed bool   `json:"handles_closed"`
}

type lifecycleProbeReport struct {
	Mode        string                `json:"mode"`
	Host        pinnedHostIdentity    `json:"host"`
	Cases       []lifecycleCaseReport `json:"cases"`
	CompletedAt time.Time             `json:"completed_at"`
}

// runNativeLifecycleProbe exercises normal EOF, bounded cancellation, and an
// output-pipe break. It intentionally uses the low-level pinned handles so the
// close order is observable and no reader goroutine can hide a leak.
func runNativeLifecycleProbe(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-lifecycle.json")
	}
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
	cases := []struct {
		name, command, order string
		wait                 time.Duration
		breakOutput          bool
	}{
		{"startup-eof", `cmd.exe /d /q /c exit /b 0`, "host-first", 10 * time.Second, false},
		{"empty-eof", `cmd.exe /d /q /c exit /b 0`, "pipes-first", 10 * time.Second, false},
		{"cancel-timeout", `powershell.exe -NoLogo -NoProfile -NonInteractive -Command "Start-Sleep -Seconds 5"`, "host-first", 250 * time.Millisecond, false},
		{"broken-pipe", `powershell.exe -NoLogo -NoProfile -NonInteractive -Command "Start-Sleep -Seconds 5"`, "pipes-first", 250 * time.Millisecond, true},
	}
	report := lifecycleProbeReport{Mode: "pinned-conpty-lifecycle", Host: identity}
	for _, item := range cases {
		caseReport, caseErr := runManualLifecycleCase(resolved, executable, item.name, item.command, item.order, item.wait, item.breakOutput)
		if caseErr != nil {
			return caseErr
		}
		report.Cases = append(report.Cases, caseReport)
	}
	report.CompletedAt = time.Now().UTC()
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	for _, item := range report.Cases {
		if item.ExpectedWait == "exit" && (item.ObservedWait != "exit" || !item.ChildExited || !item.HostExited || !item.HandlesClosed) {
			return fmt.Errorf("lifecycle EOF case failed: %+v", item)
		}
		if item.ExpectedWait == "timeout" && (item.ObservedWait != "timeout" || !item.ChildExited || !item.HostExited || !item.HandlesClosed) {
			return fmt.Errorf("lifecycle timeout case failed: %+v", item)
		}
	}
	fmt.Printf("native lifecycle probe complete: %s cases=%d\n", reportPath, len(report.Cases))
	return nil
}

func runManualLifecycleCase(hostPath, executable, name, command, order string, wait time.Duration, breakOutput bool) (lifecycleCaseReport, error) {
	result := lifecycleCaseReport{Name: name, CloseOrder: order}
	pty, err := createPinnedPseudoConsole(hostPath, 512, 25)
	if err != nil {
		return result, err
	}
	hostPID, _, err := verifyPinnedHostProcess(pty.hostProcess, hostPath)
	if err != nil {
		pty.close()
		pty.closePipes()
		return result, err
	}
	if err := attachPinnedClient(pty, command); err != nil {
		pty.close()
		pty.closePipes()
		return result, err
	}
	childPID := pty.childPID
	event, waitErr := windows.WaitForSingleObject(pty.childProcess, uint32(wait/time.Millisecond))
	if waitErr != nil {
		terminatePinnedClient(pty)
		pty.close()
		pty.closePipes()
		return result, waitErr
	}
	if event == windows.WAIT_OBJECT_0 {
		result.ExpectedWait = "exit"
		result.ObservedWait = "exit"
		_ = windows.CloseHandle(pty.childProcess)
		pty.childProcess = 0
	} else if event == uint32(windows.WAIT_TIMEOUT) {
		result.ExpectedWait = "timeout"
		result.ObservedWait = "timeout"
		if breakOutput && pty.output != 0 {
			_ = windows.CloseHandle(pty.output)
			pty.output = 0
		}
		terminatePinnedClient(pty)
	} else {
		result.ExpectedWait = "timeout"
		result.ObservedWait = fmt.Sprintf("wait-%d", event)
		terminatePinnedClient(pty)
	}
	if order == "pipes-first" {
		pty.closePipes()
		pty.close()
	} else {
		pty.close()
		pty.closePipes()
	}
	result.ChildExited, _ = processExited(childPID)
	result.HostExited, _ = processExited(hostPID)
	result.HandlesClosed = pty.signal == 0 && pty.ptyReference == 0 && pty.input == 0 && pty.output == 0 && pty.childProcess == 0 && pty.hostProcess == 0
	return result, nil
}
