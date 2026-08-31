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
		probe       = flag.Bool("probe", false, "run the pinned-host probe with live resize")
		probeStatic = flag.Bool("probe-static", false, "run the pinned-host probe without live resize")
		gate        = flag.Bool("gate", false, "run the complete standalone native gate")
		probeHost   = flag.String("probe-host", "", "verified pinned OpenConsole.exe")
		reportPath  = flag.String("report", "", "report path")
		emitProbe   = flag.Bool("emit-probe", false, "internal child mode for the pinned-host probe")
		emitWidth   = flag.Int("emit-probe-width", 0, "internal child workload width")
	)
	flag.Parse()
	if *emitProbe {
		if err := emitProbeWorkloadWidth(*emitWidth); err != nil {
			fail(err)
		}
		return
	}
	if *gate {
		if err := runNativeGate(*probeHost, *reportPath); err != nil {
			fail(err)
		}
		return
	}
	if *probe || *probeStatic {
		if err := runNativeProbe(*probeHost, *reportPath, !*probeStatic); err != nil {
			fail(err)
		}
		return
	}
	fail(fmt.Errorf("select -gate, -probe, or -probe-static"))
}

func writeJSON(path string, value any) error {
	if path == "" {
		return fmt.Errorf("report path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "pinned-conpty-probe:", err)
	os.Exit(1)
}
