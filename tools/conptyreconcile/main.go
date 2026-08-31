package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	var (
		stage      = flag.String("stage", "all", "audit, mock, host, or all")
		sourceRoot = flag.String("source-root", "", "extracted pinned Microsoft Terminal source tree")
		hostPath   = flag.String("host", "", "verified adjacent pinned OpenConsole.exe")
		reportPath = flag.String("report", "", "audit or run report path")
		probe      = flag.Bool("probe", false, "run the standalone native OpenConsole probe")
		probeReport = flag.String("probe-report", "", "native probe report path")
		probeHost  = flag.String("probe-host", "", "optional verified pinned OpenConsole.exe; otherwise download the pinned package")
		emitSeed   = flag.String("emit-seed", "", "internal pinned-host client mode: emit one recorded seed")
		emitWidth  = flag.String("emit-width", "", "internal pinned-host client mode: emit one edge width")
		emitProbe  = flag.Bool("emit-probe", false, "internal native-probe client mode")
	)
	flag.Parse()
	if *emitSeed != "" || *emitWidth != "" || *emitProbe {
		if (*emitSeed != "" && *emitWidth != "") || (*emitProbe && (*emitSeed != "" || *emitWidth != "")) {
			fail(fmt.Errorf("-emit-probe, -emit-seed, and -emit-width are mutually exclusive"))
		}
		if *emitProbe {
			if err := emitProbeWorkload(); err != nil {
				fail(err)
			}
			return
		}
		if err := emitScenario(*emitSeed, *emitWidth); err != nil {
			fail(err)
		}
		return
	}
	if *probe {
		if err := runNativeProbe(*probeHost, *probeReport); err != nil {
			fail(err)
		}
		return
	}

	executable, err := os.Executable()
	if err != nil {
		fail(err)
	}
	toolRoot := filepath.Dir(executable)
	if *hostPath == "" {
		*hostPath = filepath.Join(toolRoot, "OpenConsole.exe")
	}
	if *reportPath == "" {
		*reportPath = filepath.Join(toolRoot, "conptyreconcile-report.json")
	}

	// Every executable stage is a verification stage.  The source-fidelity
	// audit is therefore mandatory before mock, host, or the combined gate; a
	// caller cannot bypass the three-pass prerequisite by selecting a stage.
	if *stage != "audit" && *stage != "mock" && *stage != "host" && *stage != "all" {
		fail(fmt.Errorf("unknown -stage %q", *stage))
	}
	if *sourceRoot == "" {
		fail(fmt.Errorf("-source-root is required before any verification stage"))
	}
	{
		if *sourceRoot == "" {
			fail(fmt.Errorf("-source-root is required for the three-pass audit"))
		}
		report := runThreeAudits(*sourceRoot, toolRoot)
		if err := writeJSON(*reportPath, report); err != nil {
			fail(err)
		}
		if !report.passed() {
			fail(fmt.Errorf("three-pass source-fidelity audit failed; see %s", *reportPath))
		}
		if *stage == "audit" {
			return
		}
	}

	if *stage == "mock" {
		if err := runMockGate(*reportPath); err != nil {
			fail(err)
		}
		return
	}
	if *stage == "host" {
		if err := runPinnedHost(*hostPath, *reportPath); err != nil {
			fail(err)
		}
		return
	}
	if *stage == "all" {
		if err := runMockGate(*reportPath); err != nil {
			fail(err)
		}
		if err := runPinnedHost(*hostPath, *reportPath); err != nil {
			fail(err)
		}
		return
	}
	fail(fmt.Errorf("unknown -stage %q", *stage))
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "conptyreconcile:", err)
	os.Exit(1)
}
