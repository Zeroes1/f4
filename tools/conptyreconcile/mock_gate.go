package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type mockGateFailure struct {
	Seed  uint64 `json:"seed"`
	Stage string `json:"stage"`
	Error string `json:"error"`
}

type mockGateReport struct {
	Mode              string            `json:"mode"`
	SourceCommit      string            `json:"source_commit"`
	StartedAt         time.Time         `json:"started_at"`
	FinishedAt        time.Time         `json:"finished_at"`
	RecordedSeedCount int               `json:"recorded_seed_count"`
	DelayedRuns       int               `json:"delayed_runs"`
	Failures          []mockGateFailure `json:"failures,omitempty"`
	ArtifactDirectory string            `json:"artifact_directory"`
}

// runMockGate is the automatic mock stage.  It does not use an old fixture or
// a second terminal model: each scenario is generated from its recorded seed,
// fed through the pinned-source transcription, and saved with its raw input
// and serialized delayed event log for replay.
func runMockGate(reportPath string) error {
	report := mockGateReport{
		Mode:              "mock",
		SourceCommit:      pinnedOpenConsoleCommit,
		StartedAt:         time.Now().UTC(),
		RecordedSeedCount: len(recordedSeeds),
	}
	artifactDirectory := reportPath + ".runs"
	report.ArtifactDirectory = artifactDirectory
	if err := os.MkdirAll(artifactDirectory, 0o755); err != nil {
		return fmt.Errorf("create mock artifact directory: %w", err)
	}
	if err := validateRecordedSeeds(); err != nil {
		return err
	}

	// The fixed matrix is intentionally additional to the exact 300-seed gate.
	// It makes the required edge widths and Unicode/control families visible in
	// the report even if a future seed-list review changes the random values.
	for _, width := range edgeScenarioWidths() {
		scenarioCase := edgeScenario(width)
		mockCapture, err := runMockScenarioWithCapture(scenarioCase)
		base := filepath.Join(artifactDirectory, "matrix-"+strconv.Itoa(width))
		artifact := struct {
			Scenario scenario `json:"scenario"`
			Capture  capture  `json:"capture"`
			Error    string   `json:"error,omitempty"`
		}{Scenario: scenarioCase, Capture: mockCapture}
		if err != nil {
			artifact.Error = err.Error()
			report.Failures = append(report.Failures, mockGateFailure{Seed: uint64(width), Stage: "matrix", Error: err.Error()})
		}
		if writeErr := os.WriteFile(base+".input", scenarioCase.Input, 0o644); writeErr != nil && artifact.Error == "" {
			artifact.Error = writeErr.Error()
			report.Failures = append(report.Failures, mockGateFailure{Seed: uint64(width), Stage: "matrix-artifact", Error: writeErr.Error()})
		}
		if writeErr := writeJSON(base+".json", artifact); writeErr != nil && artifact.Error == "" {
			artifact.Error = writeErr.Error()
			report.Failures = append(report.Failures, mockGateFailure{Seed: uint64(width), Stage: "matrix-artifact", Error: writeErr.Error()})
		}
	}

	for _, seed := range recordedSeeds {
		scenarioCase := scenarioForSeed(int64(seed))
		artifact := struct {
			Scenario scenario `json:"scenario"`
			Mock     capture  `json:"mock_capture"`
			Capture  capture  `json:"delayed_capture"`
			Error    string   `json:"error,omitempty"`
		}{Scenario: scenarioCase}
		mockCapture, err := runMockScenarioWithCapture(scenarioCase)
		artifact.Mock = mockCapture
		if err != nil {
			artifact.Error = err.Error()
			report.Failures = append(report.Failures, mockGateFailure{Seed: seed, Stage: "mock", Error: err.Error()})
		}
		capture, err := runDelayedMockScenarioWithCapture(scenarioCase, int64(seed^0x9e3779b97f4a7c15))
		report.DelayedRuns++
		artifact.Capture = capture
		if err != nil {
			artifact.Error = err.Error()
			report.Failures = append(report.Failures, mockGateFailure{Seed: seed, Stage: "random-delay", Error: err.Error()})
		}
		base := filepath.Join(artifactDirectory, "seed-"+strconv.FormatUint(seed, 10))
		if writeErr := os.WriteFile(base+".input", scenarioCase.Input, 0o644); writeErr != nil && artifact.Error == "" {
			artifact.Error = writeErr.Error()
			report.Failures = append(report.Failures, mockGateFailure{Seed: seed, Stage: "artifact", Error: writeErr.Error()})
		}
		if writeErr := writeJSON(base+".json", artifact); writeErr != nil && artifact.Error == "" {
			artifact.Error = writeErr.Error()
			report.Failures = append(report.Failures, mockGateFailure{Seed: seed, Stage: "artifact", Error: writeErr.Error()})
		}
	}
	report.FinishedAt = time.Now().UTC()
	if err := writeJSON(reportPath, report); err != nil {
		return err
	}
	if len(report.Failures) != 0 {
		return fmt.Errorf("mock gate failed: %d failures; see %s", len(report.Failures), reportPath)
	}
	return nil
}

func validateRecordedSeeds() error {
	if len(recordedSeeds) != 300 {
		return fmt.Errorf("recorded seed list has %d entries, want exactly 300", len(recordedSeeds))
	}
	seen := make(map[uint64]struct{}, len(recordedSeeds))
	for _, seed := range recordedSeeds {
		if _, exists := seen[seed]; exists {
			return fmt.Errorf("recorded seed list contains duplicate %d", seed)
		}
		seen[seed] = struct{}{}
	}
	return nil
}
