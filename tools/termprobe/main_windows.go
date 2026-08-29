//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// termprobe answers, in one bounded run, every question the surviving
// directions in docs/CONPTY_RESEARCH.md depend on.
//
//	F  (tall viewport)     the height ladder: what can be created, what a
//	                       width change costs, whether the repaint carries the
//	                       whole history, whether the wide frame rejoins
//	                       lines, what the child believes its geometry is,
//	                       and what the alternate screen does.
//	D3 (bundled host)      the same, re-runnable against a bundled conpty.dll
//	A  (pipes)             which shells produce anything with no console
//	C  (permanently wide)  what a width-aware program prints at 4000 columns
//	--- (baseline)         P6/P11/P12/P14 re-asked on this build
//
// The structure exists because of how the earlier versions failed. Every step
// is supervised (scheduler.go): a step that does not return is abandoned and
// reported as hung rather than stopping the run. Independent measurements run
// in parallel, so one stalled height costs only itself. Results are printed
// the moment they exist. And the whole run has a hard deadline after which a
// summary is printed and the process exits, without waiting politely for
// anything.

var (
	results struct {
		sync.Mutex
		rungs []rungResult
		steps []stepResult
	}
	reportFile *reporter
)

func recordRung(r rungResult) {
	results.Lock()
	results.rungs = append(results.rungs, r)
	results.Unlock()
}

func recordStep(s stepResult) {
	results.Lock()
	results.steps = append(results.steps, s)
	results.Unlock()
}

func main() {
	var (
		emit       = flag.String("emit", "", "internal: run as the child inside a pseudoconsole")
		dryRun     = flag.Bool("dryrun", false, "exercise the scheduler with fake steps and exit (no console work)")
		fillLines  = flag.Int("fill", 150, "history lines the child prints per rung")
		childWin   = flag.Int("window", 4000, "internal: ms the child leaves the parent for its resizes")
		longWidth  = flag.Int("long", 600, "length of the long line used for the rejoin test")
		width      = flag.Int("width", 120, "pseudoconsole width for the ladder")
		wideWidth  = flag.Int("wide", 4000, "width to widen to for the rejoin frame")
		heightsCSV = flag.String("heights", "", "comma separated ladder (default 125,250,...,32000)")
		budgetMs   = flag.Int64("budget-ms", 250, "repaint budget used to pick the recommended height")
		totalMin   = flag.Float64("total-min", 5, "hard limit for the whole run, in minutes")
		workers    = flag.Int("workers", 3, "how many pseudoconsoles to measure at once")
		skipPipes  = flag.Bool("skip-pipes", false, "skip the direction A measurements")
		useBundled = flag.Bool("bundled", false, "use conpty.dll next to the probe if present")
		logPath    = flag.String("log", "", "write the report here (default termprobe-<pid>.log)")
	)
	flag.Parse()

	switch *emit {
	case "":
	case "main":
		runChild(*fillLines, *longWidth, *childWin)
		return
	case "topdraw":
		runChildTopDraw()
		return
	default:
		fmt.Fprintln(os.Stderr, "unknown -emit scenario")
		os.Exit(2)
	}

	path := *logPath
	if path == "" {
		path = fmt.Sprintf("termprobe-%d.log", os.Getpid())
	}
	lf, err := os.Create(path)
	if err != nil {
		fmt.Println("cannot create the log:", err)
		os.Exit(2)
	}
	defer lf.Close()

	started := time.Now()
	total := time.Duration(*totalMin * float64(time.Minute))
	plan := planBudget(total)

	r := &reporter{out: lf, echo: os.Stdout}
	reportFile = r

	// The last line of defence. If anything at all still manages to block --
	// a Windows call with no timeout of its own, a goroutine we abandoned --
	// the summary is printed and the process leaves. It does not ask.
	time.AfterFunc(total, func() {
		r.printf("")
		r.printf("HARD DEADLINE (%s) reached. Summarising what was measured and exiting.", total)
		printSummary(r, *budgetMs, started, path, "hard deadline")
		os.Exit(3)
	})

	r.printf("termprobe -- one bounded run, every surviving direction of docs/CONPTY_RESEARCH.md")
	r.printf("started %s", started.Format(time.RFC3339))
	r.printf("budget %s: smoke %s, feasibility %s, timing %s, reserve %s; %d measurements at once",
		total, plan.Smoke.Round(time.Second), plan.Feasibility.Round(time.Second),
		plan.Timing.Round(time.Second), plan.Reserve.Round(time.Second), *workers)
	r.printf("every step is supervised: one that does not return is abandoned and marked HUNG.")
	r.printf("")

	if *dryRun {
		dryRunScheduler(r)
		return
	}

	host := findBundledHost()
	reportEnvironment(r, host, *useBundled)

	self, err := os.Executable()
	if err != nil {
		r.printf("cannot find own path: %v", err)
		return
	}

	// --- smoke test ---------------------------------------------------
	if !smokeTest(r, self, host, *useBundled, started.Add(plan.Smoke)) {
		r.printf("")
		r.printf("No spawn strategy delivered output. The rounds would only time out, so")
		r.printf("they are not run. The table above names, per strategy, whether the child")
		r.printf("started, how it exited, whether a new console host appeared and what the")
		r.printf("reader did -- that is the diagnosis, and it needs no further run.")
		printSummary(r, *budgetMs, started, path, "smoke test failed")
		return
	}

	heights := defaultLadder()
	if *heightsCSV != "" {
		heights = parseHeights(*heightsCSV)
	}

	// --- phase 1: feasibility, all heights at once ---------------------
	feasDeadline := time.Now().Add(plan.Feasibility)
	r.section("phase 1 -- feasibility, all heights in parallel")
	r.printf("Timings here are upper bounds: several pseudoconsoles are alive at once.")
	r.printf("Phase 2 re-measures the survivors one at a time for numbers to quote.")
	r.printf("")

	tasks := make([]task, 0, len(heights)+1)
	for _, h := range heights {
		h := h
		tasks = append(tasks, task{
			Name:   fmt.Sprintf("height %d", h),
			Budget: 45 * time.Second,
			Run: func() (string, error) {
				res := measureRung(r, self, host, *useBundled, *width, h,
					*fillLines, *longWidth, *wideWidth, false)
				recordRung(res)
				return res.line(), nil
			},
		})
	}
	if !*skipPipes {
		tasks = append(tasks, task{
			Name:   "A: pipes",
			Budget: 60 * time.Second,
			Run: func() (string, error) {
				lines := []string{}
				for _, p := range measurePipes() {
					lines = append(lines, p.line())
				}
				return "\n      " + strings.Join(lines, "\n      "), nil
			},
		})
	}

	for _, s := range runTasks(tasks, *workers, feasDeadline, func(s stepResult) {
		r.printf("%s", s.line())
	}) {
		recordStep(s)
	}
	r.printf("")

	// --- phase 2: precise timings, serial, survivors only --------------
	timingDeadline := time.Now().Add(plan.Timing)
	r.section("phase 2 -- precise timings, one at a time, ascending")
	survivors := survivingHeights()
	if len(survivors) == 0 {
		r.printf("no height survived phase 1; nothing to re-measure.")
	}
	for _, h := range survivors {
		if time.Now().Add(20 * time.Second).After(timingDeadline) {
			r.printf("stopping phase 2 at %d rows: not enough budget left for a clean measurement.", h)
			break
		}
		h := h
		s := withTimeout(fmt.Sprintf("timing %d", h), 40*time.Second, func() (string, error) {
			res := measureRung(r, self, host, *useBundled, *width, h,
				*fillLines, *longWidth, *wideWidth, true)
			replaceRung(res)
			return res.line(), nil
		})
		recordStep(s)
		r.printf("%s", s.line())
	}
	r.printf("")

	// --- the remaining directions, whatever budget is left -------------
	if time.Now().Before(timingDeadline) {
		r.section("C -- what a width-aware program prints")
		s := withTimeout("C: dir /w", 30*time.Second, func() (string, error) {
			return measureWidthAware(host, *useBundled, *width, *wideWidth), nil
		})
		recordStep(s)
		r.printf("%s", s.line())
		r.printf("")
	}

	if time.Now().Before(timingDeadline) {
		r.section("baseline -- this build's emission shape")
		s := withTimeout("baseline", 30*time.Second, func() (string, error) {
			return measureEmissionShape(self, host, *useBundled, *width), nil
		})
		recordStep(s)
		r.printf("%s", s.line())
		r.printf("")
	}

	printSummary(r, *budgetMs, started, path, "completed")
}

// dryRunScheduler proves the supervision machinery on a machine with no
// ConPTY at all -- including under Wine, where this probe is smoke-tested
// before being handed to a tester. It deliberately includes a step that never
// returns and one that panics.
func dryRunScheduler(r *reporter) {
	r.section("dry run -- the scheduler only, no console work")
	tasks := []task{
		{Name: "fast", Budget: 2 * time.Second, Run: func() (string, error) { return "returned at once", nil }},
		{Name: "slow but fine", Budget: 3 * time.Second, Run: func() (string, error) {
			time.Sleep(700 * time.Millisecond)
			return "took 700ms", nil
		}},
		{Name: "hangs forever", Budget: time.Second, Run: func() (string, error) { select {} }},
		{Name: "panics", Budget: 2 * time.Second, Run: func() (string, error) {
			var p *int
			_ = *p
			return "", nil
		}},
		{Name: "fails", Budget: 2 * time.Second, Run: func() (string, error) {
			return "", fmt.Errorf("E_INVALIDARG")
		}},
	}
	out := runTasks(tasks, 3, time.Now().Add(20*time.Second), func(s stepResult) { r.printf("%s", s.line()) })
	ok, hung, failed := 0, 0, 0
	for _, s := range out {
		switch s.Outcome {
		case stepOK:
			ok++
		case stepHung:
			hung++
		case stepFailed:
			failed++
		}
	}
	r.printf("")
	r.printf("dry run: %d ok, %d hung (abandoned, not waited for), %d failed. The run", ok, hung, failed)
	r.printf("survived a step that never returns and a step that panics, which is the point.")
}

func survivingHeights() []int {
	results.Lock()
	defer results.Unlock()
	out := []int{}
	for _, r := range results.rungs {
		if r.CreateOK && r.LinesSeen > 0 {
			out = append(out, r.Height)
		}
	}
	sort.Ints(out)
	return out
}

func replaceRung(res rungResult) {
	results.Lock()
	defer results.Unlock()
	for i := range results.rungs {
		if results.rungs[i].Height == res.Height {
			results.rungs[i] = res
			return
		}
	}
	results.rungs = append(results.rungs, res)
}

func printSummary(r *reporter, budgetMs int64, started time.Time, path, why string) {
	results.Lock()
	rungs := make([]rungResult, len(results.rungs))
	copy(rungs, results.rungs)
	steps := make([]stepResult, len(results.steps))
	copy(steps, results.steps)
	results.Unlock()

	sort.Slice(rungs, func(i, j int) bool { return rungs[i].Height < rungs[j].Height })

	r.section("summary (" + why + ")")
	if len(rungs) == 0 {
		r.printf("no height was measured.")
	}
	for _, res := range rungs {
		r.printf("%s", res.line())
		if res.CreateOK {
			r.printf("      child at reflow/wide/restore: %s | %s | %s",
				res.ChildAtReflow, res.ChildAtWide, res.ChildAtRestore)
		}
		for _, n := range res.Notes {
			r.printf("      note: %s", n)
		}
	}
	r.printf("")
	r.printf("VERDICT(F): %s", ladderVerdict(rungs, budgetMs))
	r.printf("")

	if len(rungs) > 0 {
		r.printf("%-8s %-24s %-14s %s", "height", "child sees (win/buf)", "inside alt", "taller buffer accepted")
		for _, res := range rungs {
			if !res.CreateOK {
				continue
			}
			r.printf("%-8d %-24s %-14s %s", res.Height,
				fmt.Sprintf("%dx%d / %dx%d", res.ChildCols, res.ChildRows, res.ChildBufW, res.ChildBufH),
				fmt.Sprintf("%dx%d", res.AltChildCols, res.AltChildRows),
				res.ChildSetBufTallerDetal)
		}
		r.printf("")
	}

	hung := []string{}
	for _, s := range steps {
		if s.Outcome == stepHung {
			hung = append(hung, s.Name)
		}
	}
	if len(hung) > 0 {
		r.printf("HUNG steps (abandoned, their numbers are missing rather than wrong): %s",
			strings.Join(hung, ", "))
		r.printf("")
	}

	r.printf("total %s; report written to %s", time.Since(started).Round(time.Second), path)
}

func parseHeights(csv string) []int {
	out := []int{}
	for _, p := range strings.Split(csv, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			out = append(out, n)
		}
	}
	if len(out) == 0 {
		return defaultLadder()
	}
	return out
}

// ---------------------------------------------------------------------------
// reporting
// ---------------------------------------------------------------------------

type reporter struct {
	mu   sync.Mutex
	out  *os.File
	echo *os.File
}

func (r *reporter) printf(format string, a ...any) {
	line := fmt.Sprintf(format, a...)
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintln(r.out, line)
	fmt.Fprintln(r.echo, line)
	r.out.Sync()
}

func (r *reporter) section(title string) {
	r.printf("=== %s %s", title, strings.Repeat("=", max(0, 60-len(title))))
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func reportEnvironment(r *reporter, host bundledHost, useBundled bool) {
	r.section("environment")
	r.printf("windows build        %s", windowsBuild())
	r.printf("conhost.exe          %s", fileVersion(systemFile("conhost.exe")))
	r.printf("condrv.sys           %s", fileVersion(systemFile(`drivers\condrv.sys`)))
	if host.found {
		r.printf("bundled conpty.dll   %s (%s)  in use: %v", host.version, host.path, useBundled)
	} else {
		r.printf("bundled conpty.dll   absent")
	}
	r.printf("")
}

// ---------------------------------------------------------------------------
// smoke test: stop at the first strategy that works
// ---------------------------------------------------------------------------

func smokeTest(r *reporter, self string, host bundledHost, useBundled bool, deadline time.Time) bool {
	r.section("smoke test -- does a pseudoconsole deliver anything at all")
	r.printf("%-26s %8s %7s  %s", "strategy", "bytes", "marker", "child / host / reader")

	const want = "probe-alive"
	argv := []string{"cmd.exe", "/d", "/c", "echo", want}

	for _, st := range defaultStrategies() {
		if time.Now().After(deadline) {
			r.printf("(smoke budget spent)")
			break
		}
		st := st
		res := withTimeout("smoke: "+st.Name, 12*time.Second, func() (string, error) {
			s, err := newSessionWith(st, 80, 25, argv, host, useBundled)
			if err != nil {
				return "", err
			}
			defer s.Close(host)
			until := time.Now().Add(4 * time.Second)
			got := false
			for time.Now().Before(until) {
				if strings.Contains(string(s.col.since(0)), want) {
					got = true
					break
				}
				time.Sleep(40 * time.Millisecond)
			}
			return fmt.Sprintf("%-26s %8d %7v  %s / %s / %s",
				st.Name, s.col.size(), got, s.childStatus(), s.hostDescription(), s.readerStatus()), nil
		})
		recordStep(res)

		if res.Outcome == stepHung {
			r.printf("%-26s %8s %7s  HUNG after %s, abandoned", st.Name, "-", "-", res.Dur.Round(time.Millisecond))
			continue
		}
		if res.Outcome == stepFailed {
			r.printf("%-26s %8s %7s  %s", st.Name, "-", "-", res.Err)
			continue
		}
		r.printf("%s", res.Summary)

		// The moment one strategy delivers bytes, the question is answered and
		// the remaining rows would only cost budget. An earlier version kept
		// walking the matrix after the answer was in hand and hung on the next
		// row, which is how a solved problem turned into another trip.
		if strings.Contains(res.Summary, " true  ") || !strings.Contains(res.Summary, "        0 ") {
			chosenStrategy = st
			r.printf("")
			r.printf("using strategy %q; the remaining rows are not needed.", st.Name)
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// one rung
// ---------------------------------------------------------------------------

var sizeLineRE = regexp.MustCompile(`~SZ:([a-z-]+);wincols=(\d+);winrows=(\d+);bufw=(\d+);bufh=(\d+)~`)

func parseSizeLines(raw []byte) map[string][4]int {
	out := map[string][4]int{}
	for _, m := range sizeLineRE.FindAllStringSubmatch(string(raw), -1) {
		n := [4]int{}
		for i := 0; i < 4; i++ {
			n[i], _ = strconv.Atoi(m[i+2])
		}
		out[m[1]] = n
	}
	return out
}

var setBufRE = regexp.MustCompile(`~SETBUF:[^~]*~`)

// measureRung runs one height. Every wait inside it is bounded, and the whole
// call is itself run under a watchdog by the caller, so it has two independent
// reasons it cannot block the run.
func measureRung(r *reporter, self string, host bundledHost, useBundled bool,
	width, height, fillLines, longWidth, wideWidth int, precise bool) rungResult {

	res := rungResult{Height: height, LinesAsked: fillLines, Precise: precise}
	settle := settleFor(height)
	limit := waitFor(height)

	// The child holds the session open for this long after it has printed its
	// history, which is the parent's whole window for the three resizes.
	window := 3*settle + 3*time.Second
	if window < 4*time.Second {
		window = 4 * time.Second
	}
	args := []string{self, "-emit", "main",
		"-fill", strconv.Itoa(fillLines),
		"-long", strconv.Itoa(longWidth),
		"-window", strconv.Itoa(int(window / time.Millisecond))}

	t0 := time.Now()
	s, err := newSession(width, height, args, host, useBundled)
	res.CreateMs = time.Since(t0).Milliseconds()
	if err != nil {
		res.CreateNo = err.Error()
		return res
	}
	defer func() {
		if cs := s.Close(host); cs.Outcome == stepHung {
			res.Notes = append(res.Notes, "ClosePseudoConsole did not return; session abandoned")
		}
	}()
	res.CreateOK = true
	res.HostName = s.hostName
	res.HostRSSAfterCreateKB = workingSetKB(s.hostPID)

	fillStart := time.Now()
	off := s.col.waitForMarker(0, markerDone, limit)
	res.FillMs = time.Since(fillStart).Milliseconds()
	if off < 0 {
		if s.col.silent() {
			res.Notes = append(res.Notes, fmt.Sprintf(
				"SILENT: zero bytes in %dms. child: %s; host: %s; %s. Remaining phases skipped.",
				res.FillMs, s.childStatus(), s.hostDescription(), s.readerStatus()))
			return res
		}
		res.Notes = append(res.Notes, "no DONE marker; the numbers below are unreliable")
		off = s.col.mark()
	}
	s.col.waitQuiet(settle, limit)
	full := s.col.since(0)
	res.FillBytes = len(full)
	res.LinesSeen, res.LowestSeen, res.HighestSeen = countFillMarkers(full)
	res.HostRSSAfterFillKB = workingSetKB(s.hostPID)
	if sizes := parseSizeLines(full); len(sizes) > 0 {
		if v, ok := sizes["start"]; ok {
			res.ChildCols, res.ChildRows, res.ChildBufW, res.ChildBufH = v[0], v[1], v[2], v[3]
		}
	}

	s.col.waitForMarker(off, markerReady, 10*time.Second)
	s.col.waitQuiet(settle, limit)

	// Liveness is recorded next to the resize, so "zero bytes came back" can
	// never again be confused with "there was nobody left to answer".
	res.ChildAtReflow = s.childStatus()
	mark := s.col.mark()
	t1 := time.Now()
	if err := s.resize(width-1, height, host); err != nil {
		res.Notes = append(res.Notes, "resize to width-1 failed: "+err.Error())
	}
	s.col.waitQuiet(settle, limit)
	res.ReflowMs = time.Since(t1).Milliseconds()
	shape := analyseFrame(s.col.since(mark), width-1)
	res.ReflowBytes = shape.bytes
	res.ReflowMarkers = shape.fillMarkers
	res.ReflowLowest = shape.lowestMarker
	res.ReflowHighest = shape.highestMarker
	res.ReflowStartsAtHome = shape.startsAtHome
	res.ReflowHidesCursor = shape.hidesCursor
	res.ReflowSizeReport = shape.sizeReport

	res.ChildAtWide = s.childStatus()
	mark = s.col.mark()
	t2 := time.Now()
	if err := s.resize(wideWidth, height, host); err != nil {
		res.Notes = append(res.Notes, "widen failed: "+err.Error())
	} else {
		s.col.waitQuiet(settle, limit)
		res.WideMs = time.Since(t2).Milliseconds()
		wide := s.col.since(mark)
		res.WideBytes = len(wide)
		res.WideLongRows = lineRows(wide, wideWidth, markerLongStart)
	}

	res.ChildAtRestore = s.childStatus()
	mark = s.col.mark()
	if err := s.resize(width, height, host); err != nil {
		res.Notes = append(res.Notes, "restore failed: "+err.Error())
	} else {
		s.col.waitQuiet(settle, limit)
		res.RestoreOK = true
		res.AfterRestoreM = analyseFrame(s.col.since(mark), width).fillMarkers
	}

	if s.col.waitForMarker(mark, markerAltDone, 20*time.Second) < 0 {
		res.Notes = append(res.Notes, "the alt-screen phase did not complete")
	}
	s.col.waitQuiet(settle, 10*time.Second)
	tail := s.col.since(mark)
	tailShape := analyseFrame(tail, width)
	res.AltEnterSeen = tailShape.sawAltEnter
	res.AltLeaveSeen = tailShape.sawAltLeave
	if sizes := parseSizeLines(tail); len(sizes) > 0 {
		if v, ok := sizes["in-alt"]; ok {
			res.AltChildCols, res.AltChildRows = v[0], v[1]
		}
	}
	if m := setBufRE.Find(tail); m != nil {
		res.ChildSetBufTallerDetal = string(m)
		res.ChildSetBufTallerOK = strings.Contains(res.ChildSetBufTallerDetal, "ok=true")
	} else {
		res.ChildSetBufTallerDetal = "(not reported)"
	}

	if res.ReflowBytes == 0 && !strings.Contains(res.ChildAtReflow, "still running") {
		res.Notes = append(res.Notes, "the child had already exited before the resizes: "+
			res.ChildAtReflow+" -- the reflow numbers are missing, not zero")
	}
	return res
}

func settleFor(height int) time.Duration {
	d := 250*time.Millisecond + time.Duration(height/40)*time.Millisecond
	if d > 1500*time.Millisecond {
		d = 1500 * time.Millisecond
	}
	return d
}

func waitFor(height int) time.Duration {
	d := 8*time.Second + time.Duration(height/10)*time.Millisecond
	if d > 25*time.Second {
		d = 25 * time.Second
	}
	return d
}

// ---------------------------------------------------------------------------
// the other directions
// ---------------------------------------------------------------------------

func measureWidthAware(host bundledHost, useBundled bool, narrow, wide int) string {
	argv := []string{"cmd.exe", "/d", "/c", "dir", "/w", "/-p", `%SystemRoot%\System32\drivers\etc`}
	lines := []string{}
	for _, w := range []int{narrow, wide} {
		s, err := newSession(w, 200, argv, host, useBundled)
		if err != nil {
			lines = append(lines, fmt.Sprintf("width %-5d session failed: %v", w, err))
			continue
		}
		s.col.waitQuiet(600*time.Millisecond, 12*time.Second)
		raw := s.col.since(0)
		g := newWideGrid(w)
		g.feed(raw)
		longest, nonEmpty := 0, 0
		for _, row := range g.rows {
			if t := strings.TrimRight(string(row), " "); t != "" {
				nonEmpty++
				if len(t) > longest {
					longest = len(t)
				}
			}
		}
		lines = append(lines, fmt.Sprintf("width %-5d rows=%-4d longest=%-5d bytes=%d", w, nonEmpty, longest, len(raw)))
		s.Close(host)
	}
	return "\n      " + strings.Join(lines, "\n      ")
}

func measureEmissionShape(self string, host bundledHost, useBundled bool, width int) string {
	args := []string{self, "-emit", "main", "-fill", "20", "-long", strconv.Itoa(width + width/2)}
	s, err := newSession(width, 200, args, host, useBundled)
	if err != nil {
		return "session failed: " + err.Error()
	}
	defer s.Close(host)
	s.col.waitForMarker(0, markerDone, 15*time.Second)
	s.col.waitQuiet(300*time.Millisecond, 10*time.Second)
	live := s.col.since(0)
	rows := lineRows(live, width, markerLongStart)
	hard := strings.Contains(string(live), markerLongStart) && crlfWithinLongLine(string(live))

	mark := s.col.mark()
	s.resize(width-1, 200, host)
	s.col.waitQuiet(300*time.Millisecond, 12*time.Second)
	frame := analyseFrame(s.col.since(mark), width-1)

	return fmt.Sprintf("\n      long line start marker on %d row(s) at width %d; hard CRLF inside the wrap (P6): %v"+
		"\n      repaint: hides cursor=%v starts at home=%v size report=%v",
		rows, width, hard, frame.hidesCursor, frame.startsAtHome, frame.sizeReport)
}

func crlfWithinLongLine(s string) bool {
	i := strings.Index(s, markerLongStart)
	j := strings.Index(s, markerLongEnd)
	if i < 0 || j < 0 || j < i {
		return false
	}
	return strings.Contains(s[i:j], "\r\n")
}
