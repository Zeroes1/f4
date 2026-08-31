package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func main() {
	var (
		probe               = flag.Bool("probe", false, "run the pinned-host probe with live resize")
		probeStatic         = flag.Bool("probe-static", false, "run the pinned-host probe without live resize")
		gate                = flag.Bool("gate", false, "run the complete standalone native gate")
		seeds               = flag.Bool("seeds", false, "run the 300-session native seed stage")
		seed                = flag.Uint64("seed", 0, "run one deterministic native seed")
		partial             = flag.Bool("partial", false, "run resize during an incomplete line")
		commandProbe        = flag.Bool("command-probe", false, "measure recursive dir output on the pinned host")
		commandCompare      = flag.Bool("command-compare", false, "compare recursive dir through pinned host with redirected output")
		commandCompareWidth = flag.Int("command-compare-width", 80, "pinned-host capture width for -command-compare")
		clearProbe          = flag.Bool("clear-probe", false, "verify Clear-Host emits and applies ESC[3J")
		scrollProbe         = flag.Bool("scroll-probe", false, "verify consumer scrollback and piece-table eviction")
		emptyProbe          = flag.Bool("empty-probe", false, "verify an empty child emits no empty frame")
		reflowProbe         = flag.Bool("reflow-probe", false, "verify consumer reflow after a static pinned-host session")
		probeHost           = flag.String("probe-host", "", "verified pinned OpenConsole.exe")
		reportPath          = flag.String("report", "", "report path")
		emitProbe           = flag.Bool("emit-probe", false, "internal child mode for the pinned-host probe")
		emitWidth           = flag.Int("emit-probe-width", 0, "internal child workload width")
		emitSeed            = flag.String("emit-seed", "", "internal child deterministic seed")
		emitPartial         = flag.Bool("emit-partial", false, "internal child incomplete-line workload")
		emitAlternate       = flag.Bool("emit-alternate", false, "internal child alternate-screen workload")
		emitControl         = flag.Bool("emit-control", false, "internal child control-sequence workload")
		emitReflow          = flag.Bool("emit-reflow", false, "internal child static reflow workload")
		emitScroll          = flag.Bool("emit-scroll", false, "internal child static scrollback workload")
	)
	flag.Parse()
	if *seed == 0 {
		if value := os.Getenv("PINNED_CONPTY_SEED"); value != "" {
			parsed, err := strconv.ParseUint(value, 0, 64)
			if err != nil || parsed == 0 {
				fail(fmt.Errorf("invalid PINNED_CONPTY_SEED %q", value))
			}
			*seed = parsed
		}
	}
	if *emitProbe {
		if err := emitProbeWorkloadWidth(*emitWidth); err != nil {
			fail(err)
		}
		return
	}
	if *emitSeed != "" {
		seed, err := strconv.ParseUint(*emitSeed, 16, 64)
		if err != nil {
			fail(fmt.Errorf("invalid seed %q: %w", *emitSeed, err))
		}
		if err := emitSeedWorkload(seed, *emitWidth); err != nil {
			fail(err)
		}
		return
	}
	if *emitPartial {
		if err := emitPartialWorkload(*emitWidth); err != nil {
			fail(err)
		}
		return
	}
	if *emitAlternate {
		if err := emitAlternateWorkload(*emitWidth); err != nil {
			fail(err)
		}
		return
	}
	if *emitControl {
		if err := emitControlWorkload(); err != nil {
			fail(err)
		}
		return
	}
	if *emitReflow {
		if err := emitReflowWorkload(*emitWidth); err != nil {
			fail(err)
		}
		return
	}
	if *emitScroll {
		if err := emitScrollWorkload(); err != nil {
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
	if *seeds {
		if err := runNativeSeedGate(*probeHost, *reportPath); err != nil {
			fail(err)
		}
		return
	}
	if *seed != 0 {
		if err := runNativeSingleSeed(*probeHost, *reportPath, *seed); err != nil {
			fail(err)
		}
		return
	}
	if *partial {
		if err := runNativePartialProbe(*probeHost, *reportPath); err != nil {
			fail(err)
		}
		return
	}
	if *commandProbe {
		if err := runNativeCommandProbe(*probeHost, *reportPath); err != nil {
			fail(err)
		}
		return
	}
	if *commandCompare {
		if err := runNativeCommandCompareAtWidth(*probeHost, *reportPath, *commandCompareWidth); err != nil {
			fail(err)
		}
		return
	}
	if *clearProbe {
		if err := runNativeClearProbe(*probeHost, *reportPath); err != nil {
			fail(err)
		}
		return
	}
	if *scrollProbe {
		if err := runNativeScrollProbe(*probeHost, *reportPath); err != nil {
			fail(err)
		}
		return
	}
	if *emptyProbe {
		if err := runNativeEmptyProbe(*probeHost, *reportPath); err != nil {
			fail(err)
		}
		return
	}
	if *reflowProbe {
		if err := runNativeReflowProbe(*probeHost, *reportPath); err != nil {
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
