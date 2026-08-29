package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type auditPassResult struct {
	Name   string
	Status string
	Lines  []string
}

type auditReport struct {
	SourceCommit string
	Runs         []auditRun
	Passes       []auditPassResult
}

type auditRun struct {
	Name   string
	Passes []auditPassResult
}

// runThreeAudits is intentionally separate from the test runner.  A caller
// must obtain three PASS results before it is allowed to enter either mock or
// host execution.
func runThreeAudits(sourceRoot, toolRoot string) auditReport {
	report := auditReport{SourceCommit: pinnedOpenConsoleCommit}
	for i := 1; i <= 3; i++ {
		passes := []auditPassResult{
			runSymbolPathAudit(sourceRoot, toolRoot),
			runTransitionAudit(sourceRoot, toolRoot),
			runNegativeAudit(toolRoot),
		}
		report.Runs = append(report.Runs, auditRun{Name: fmt.Sprintf("self-audit-%d", i), Passes: passes})
		report.Passes = append(report.Passes, passes...)
	}
	return report
}

func (r auditReport) passed() bool {
	if r.SourceCommit != pinnedOpenConsoleCommit || len(r.Runs) != 3 || len(r.Passes) != 9 {
		return false
	}
	for _, run := range r.Runs {
		if len(run.Passes) != 3 {
			return false
		}
		for _, pass := range run.Passes {
			if pass.Status != "PASS" {
				return false
			}
		}
	}
	return true
}

func runSymbolPathAudit(sourceRoot, toolRoot string) auditPassResult {
	pass := auditPassResult{Name: "symbol/path", Status: "PASS"}
	for _, entry := range pinnedSourceManifest {
		sourcePath := filepath.Join(sourceRoot, filepath.FromSlash(entry.Path))
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			pass.Status = "FAIL"
			pass.Lines = append(pass.Lines, fmt.Sprintf("Gap: missing pinned source %s: %v", entry.Path, err))
			continue
		}
		if entry.SHA256 != "" && sha256Hex(data) != entry.SHA256 {
			pass.Status = "FAIL"
			pass.Lines = append(pass.Lines, fmt.Sprintf("Gap: SHA-256 mismatch for %s", entry.Path))
		}
		for _, symbol := range entry.SourceSymbols {
			if !strings.Contains(string(data), symbol) {
				pass.Status = "FAIL"
				pass.Lines = append(pass.Lines, fmt.Sprintf("Gap: source symbol %s absent from %s", symbol, entry.Path))
			}
		}
		mockData, err := os.ReadFile(filepath.Join(toolRoot, entry.MockFile))
		if err != nil {
			pass.Status = "FAIL"
			pass.Lines = append(pass.Lines, fmt.Sprintf("Gap: mock file %s missing for %s", entry.MockFile, entry.Path))
			continue
		}
		for _, symbol := range entry.MockSymbols {
			if !strings.Contains(string(mockData), symbol) {
				pass.Status = "FAIL"
				pass.Lines = append(pass.Lines, fmt.Sprintf("Gap: mock symbol %s absent from %s", symbol, entry.MockFile))
			}
		}
	}
	if len(pass.Lines) == 0 {
		pass.Lines = append(pass.Lines, "All manifest paths, hashes, and symbol anchors resolved.")
	}
	return pass
}

func runTransitionAudit(sourceRoot, toolRoot string) auditPassResult {
	pass := auditPassResult{Name: "transition/control-flow", Status: "FAIL"}
	pass.Lines = append(pass.Lines,
		"Gap: line-by-line branch/default/order review is not yet complete.",
		"Gap: the current parser, stream, reflow, and frame files contain transcriptions that still need direct comparison with the pinned source.",
	)
	_ = sourceRoot
	_ = toolRoot
	return pass
}

func runNegativeAudit(toolRoot string) auditPassResult {
	pass := auditPassResult{Name: "negative/provenance", Status: "PASS"}
	mockFiles := make(map[string]bool)
	for _, entry := range pinnedSourceManifest {
		mockFiles[entry.MockFile] = true
	}
	for fileName := range mockFiles {
		if !strings.HasSuffix(fileName, ".go") {
			continue
		}
		path := filepath.Join(toolRoot, fileName)
		data, err := os.ReadFile(path)
		if err != nil {
			pass.Status = "FAIL"
			pass.Lines = append(pass.Lines, fmt.Sprintf("Gap: cannot read %s: %v", fileName, err))
			continue
		}
		text := strings.ToLower(string(data))
		for _, marker := range []string{"wine", "linux pty", "windows terminal", "old capture", "field dump"} {
			if strings.Contains(text, marker) {
				pass.Status = "FAIL"
				pass.Lines = append(pass.Lines, fmt.Sprintf("Gap: forbidden provenance marker %q in %s", marker, fileName))
			}
		}
	}
	if len(pass.Lines) == 0 {
		pass.Lines = append(pass.Lines, "No alternate-host, stale-capture, or field-dump marker found in mock Go files.")
	}
	return pass
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
