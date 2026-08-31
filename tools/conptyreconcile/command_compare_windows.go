//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type commandLineMismatch struct {
	Index            int    `json:"index"`
	Expected         string `json:"expected"`
	Observed         string `json:"observed"`
	ObservedCrossRow bool   `json:"observed_cross_row,omitempty"`
}

type commandCompareReport struct {
	Mode                    string                `json:"mode"`
	Host                    pinnedHostIdentity    `json:"host"`
	Command                 string                `json:"command"`
	RedirectCommand         string                `json:"redirect_command"`
	RedirectedBytes         int                   `json:"redirected_bytes"`
	RedirectedSHA256        string                `json:"redirected_sha256"`
	HostRawBytes            int                   `json:"host_raw_bytes"`
	HostRawSHA256           string                `json:"host_raw_sha256"`
	ExpectedLines           int                   `json:"expected_lines"`
	ObservedLines           int                   `json:"observed_lines"`
	MismatchCount           int                   `json:"mismatch_count"`
	NormalizedMismatchCount int                   `json:"normalized_mismatch_count"`
	ContentMismatchCount    int                   `json:"content_mismatch_count"`
	TrailingPaddingOnly     int                   `json:"trailing_padding_only"`
	CrossRowMismatch        int                   `json:"cross_row_mismatch"`
	CUPBeforeCRLF           int                   `json:"cup_before_crlf"`
	Mismatches              []commandLineMismatch `json:"mismatches,omitempty"`
	ChildExitCode           uint32                `json:"child_exit_code"`
	ChildExited             bool                  `json:"child_exited"`
	HostExited              bool                  `json:"host_exited"`
	HandlesClosed           bool                  `json:"handles_closed"`
	CompletedAt             time.Time             `json:"completed_at"`
}

// runNativeCommandCompare executes the same recursive dir command through a
// pinned ConPTY and directly to a file. The file is an independent byte-level
// ground truth for child line boundaries; host rendering is compared only
// after stripping the renderer's out-of-band controls via RenderedLines.
func runNativeCommandCompare(hostPath, reportPath string) error {
	if reportPath == "" {
		reportPath = filepath.Join("artifacts", "pinned-conpty-command-compare.json")
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
	root := `C:\Windows\System32`
	begin := "__PINNED_CONPTY_PROBE_DIR_COMPARE_BEGIN__"
	end := "__PINNED_CONPTY_PROBE_DIR_COMPARE_END__"
	command := fmt.Sprintf(`cmd.exe /d /q /c "echo %s & set DIRCMD= & dir /s /b %s & echo %s & exit /b 0"`, begin, root, end)
	redirectCommand := fmt.Sprintf(`cmd.exe /d /q /c "set DIRCMD= & dir /s /b %s"`, root)
	redirectPath := reportPath + ".redirected.raw"
	redirected, err := runRedirectedDir(root, redirectPath)
	if err != nil {
		return err
	}
	session, runErr := runNativeProbeSessionWithWorkload(resolved, executable, 80, 1000, false, nil, command, []string{begin, end})
	if runErr != nil {
		return fmt.Errorf("pinned command comparison session: %w", runErr)
	}
	expected := splitCommandLines(redirected)
	rendered := parseRenderedHistoryAtWidth(session.RawOutput, 80).Lines()
	segment, ok := renderedMarkerSegment(rendered, begin, end)
	if !ok || len(segment) < 2 {
		return fmt.Errorf("pinned command comparison markers did not delimit rendered output")
	}
	observed := segment[1 : len(segment)-1]
	report := commandCompareReport{
		Mode: "pinned-conpty-command-compare", Host: identity, Command: command,
		RedirectCommand: redirectCommand, RedirectedBytes: len(redirected),
		HostRawBytes: len(session.RawOutput), HostRawSHA256: session.RawSHA256,
		ExpectedLines: len(expected), ObservedLines: len(observed),
		CUPBeforeCRLF: len(cupPattern.FindAll(session.RawOutput, -1)),
		ChildExitCode: session.ExitCode, ChildExited: session.ChildExited,
		HostExited: session.HostExited, HandlesClosed: session.HandlesClosed,
		CompletedAt: time.Now().UTC(),
	}
	redirectHash := sha256.Sum256(redirected)
	report.RedirectedSHA256 = fmt.Sprintf("%x", redirectHash[:])
	for index := 0; index < len(expected) || index < len(observed); index++ {
		want, got := "", ""
		var cross bool
		if index < len(expected) {
			want = string(expected[index])
		}
		if index < len(observed) {
			got = string(observed[index].Bytes)
			cross = observed[index].CrossRow
		}
		if want != got {
			report.MismatchCount++
			if cross {
				report.CrossRowMismatch++
			}
			if strings.TrimRight(got, " ") == strings.TrimRight(want, " ") {
				report.TrailingPaddingOnly++
			} else {
				report.ContentMismatchCount++
			}
			if len(report.Mismatches) < 20 {
				report.Mismatches = append(report.Mismatches, commandLineMismatch{Index: index, Expected: want, Observed: got, ObservedCrossRow: cross})
			}
		}
		if want != strings.TrimRight(got, " ") {
			report.NormalizedMismatchCount++
		}
	}
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	artifactDir := reportPath + ".sessions"
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return err
	}
	if err := writeAndVerifyRawArtifact(filepath.Join(artifactDir, "80x1000.raw"), session.RawOutput, session.RawSHA256); err != nil {
		return err
	}
	if report.MismatchCount != 0 {
		return fmt.Errorf("native command comparison found %d line mismatches (report %s)", report.MismatchCount, reportPath)
	}
	fmt.Printf("native command comparison complete: %s mismatches=%d expected_lines=%d observed_lines=%d cup_before_crlf=%d\n", reportPath, report.MismatchCount, report.ExpectedLines, report.ObservedLines, report.CUPBeforeCRLF)
	return nil
}

func runRedirectedDir(root, path string) ([]byte, error) {
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("cmd.exe", "/d", "/q", "/c", fmt.Sprintf("set DIRCMD= & dir /s /b %s", root))
	cmd.Stdout = file
	cmd.Stderr = file
	cmd.Env = append(os.Environ(), "DIRCMD=", "PAGER=", "GIT_PAGER=", "GIT_TERMINAL_PROMPT=0")
	err = cmd.Run()
	closeErr := file.Close()
	if err != nil {
		return nil, fmt.Errorf("redirected command failed: %w", err)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func splitCommandLines(data []byte) [][]byte {
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
	data = bytes.TrimSuffix(data, []byte("\n"))
	if len(data) == 0 {
		return nil
	}
	parts := bytes.Split(data, []byte("\n"))
	result := make([][]byte, len(parts))
	for index, part := range parts {
		result[index] = append([]byte(nil), part...)
	}
	return result
}
