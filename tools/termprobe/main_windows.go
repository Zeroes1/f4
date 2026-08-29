//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// termprobe answers, in one run, every question the surviving directions in
// docs/CONPTY_RESEARCH.md depend on -- not only the most likely one.
//
//	F  (tall viewport)     the height ladder: what can be created, what a
//	                       width change costs there, whether the repaint
//	                       carries the whole history, whether the wide frame
//	                       rejoins lines, what the child believes its
//	                       geometry is, and what the alternate screen does.
//	D3 (bundled host)      the same ladder against a conpty.dll placed next
//	                       to the probe, plus every binary's version.
//	A  (pipes)             which shells and tools produce anything with no
//	                       console at all, and whether COLUMNS is honoured.
//	C  (permanently wide)  what a width-aware program prints at 4000.
//	--- (baseline)         the §1 emission questions re-asked on this build:
//	                       wrap shape, size report, repaint starts at home.
//
// It runs in ROUNDS, smallest scale first, touching every direction in each
// round before moving to a bigger one. A run that dies at 16000 rows -- or is
// killed by an impatient tester -- therefore still leaves a complete answer
// for every direction at every smaller scale, and a partial verdict is
// printed after each round. Finishing one direction at every scale before
// starting the next would lose everything about the other directions if the
// tall rungs misbehave, and the tall rungs are the ones expected to
// misbehave.
//
// Nothing here is destructive: the probe owns its pseudoconsoles and its
// children, writes only to its own log, and changes no setting.

var (
	runDeadline time.Time
	verbose     bool
)

func timeLeft() time.Duration {
	if runDeadline.IsZero() {
		return time.Hour
	}
	d := time.Until(runDeadline)
	if d < 0 {
		return 0
	}
	return d
}

func outOfTime() bool { return timeLeft() <= 0 }

// capped keeps every individual wait inside whatever is left of the run, so a
// stuck phase cannot eat a deadline meant for the whole probe.
func capped(d time.Duration) time.Duration {
	if l := timeLeft(); d > l {
		return l
	}
	return d
}

// settleFor and waitFor scale with the height: a 32000-row repaint is
// legitimately slower than a 125-row one, and one fixed timeout either cuts
// the big frames short or makes the small rounds crawl.
func settleFor(height int) time.Duration {
	d := 250*time.Millisecond + time.Duration(height/40)*time.Millisecond
	if d > 2*time.Second {
		d = 2 * time.Second
	}
	return d
}

func waitFor(height int) time.Duration {
	d := 12*time.Second + time.Duration(height/6)*time.Millisecond
	if d > 90*time.Second {
		d = 90 * time.Second
	}
	return d
}

func main() {
	var (
		emit        = flag.String("emit", "", "internal: run as the child inside a pseudoconsole")
		fillLines   = flag.Int("fill", 200, "history lines the child prints per round")
		longWidth   = flag.Int("long", 600, "length of the long line used for the rejoin test")
		width       = flag.Int("width", 120, "pseudoconsole width for the ladder")
		wideWidth   = flag.Int("wide", 4000, "width to widen to for the rejoin frame")
		heightsCSV  = flag.String("heights", "", "comma separated ladder (default 125,250,...,32000)")
		budgetMs    = flag.Int64("budget-ms", 250, "repaint budget used to pick the recommended height")
		quick       = flag.Bool("quick", false, "only the 125..2000 rounds")
		skipPipes   = flag.Bool("skip-pipes", false, "skip the direction A measurements")
		useBundled  = flag.Bool("bundled", false, "use conpty.dll next to the probe if present")
		deadlineMin = flag.Int("deadline-min", 20, "give up and summarise after this many minutes (0 = no limit)")
		selfTest    = flag.Bool("selftest", false, "run only the smoke test and exit")
		force       = flag.Bool("force", false, "run the rounds even if the smoke test finds a silent session")
		verboseF    = flag.Bool("v", false, "print a line for every phase, not only every round")
		logPath     = flag.String("log", "", "write the report here (default termprobe-<pid>.log)")
	)
	flag.Parse()
	verbose = *verboseF

	switch *emit {
	case "":
	case "main":
		runChild(*fillLines, *longWidth)
		return
	case "topdraw":
		runChildTopDraw()
		return
	default:
		fmt.Fprintln(os.Stderr, "unknown -emit scenario")
		os.Exit(2)
	}

	heights := defaultLadder()
	if *quick {
		heights = []int{125, 250, 500, 1000, 2000}
	}
	if *heightsCSV != "" {
		heights = parseHeights(*heightsCSV)
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
	if *deadlineMin > 0 {
		runDeadline = started.Add(time.Duration(*deadlineMin) * time.Minute)
	}

	r := &reporter{out: lf, echo: os.Stdout, started: started}
	r.printf("termprobe -- one run, every surviving direction of docs/CONPTY_RESEARCH.md")
	r.printf("started %s", started.Format(time.RFC3339))
	if *deadlineMin > 0 {
		r.printf("deadline %d minutes; rounds run smallest first, so an early stop still leaves a usable log",
			*deadlineMin)
	}
	r.printf("")

	host := findBundledHost()
	reportEnvironment(r, host, *useBundled)

	self, err := os.Executable()
	if err != nil {
		r.printf("cannot find own path: %v", err)
		return
	}

	if !smokeTest(r, self, host, *useBundled) {
		r.printf("")
		r.printf("The smoke test found a SILENT pseudoconsole: created without error, but not")
		r.printf("one byte came back. Running the ladder now would only time out nine times,")
		r.printf("which is exactly what the first field run of this probe did.")
		r.printf("")
		r.printf("Things worth trying, cheapest first:")
		r.printf("  * copy the exe to a local disk and run it from there (a removable drive")
		r.printf("    with Mark-of-the-Web can have the child launch blocked silently)")
		r.printf("  * run it from a normal console window rather than from Explorer")
		r.printf("  * check that no security product is blocking a second copy of this exe")
		r.printf("  * send this log: the child status and byte counts above name the failure")
		if !*force {
			r.printf("")
			r.printf("Stopping here. Pass -force to run the rounds anyway.")
			return
		}
		r.printf("")
		r.printf("-force given: continuing, expect timeouts.")
	}
	if *selfTest {
		r.printf("")
		r.printf("-selftest given: stopping after the smoke test.")
		return
	}

	r.printf("plan: %d rounds at heights %v; each round touches F, the top-draw risk,", len(heights), heights)
	r.printf("the emission shape and the width-aware question at that scale.")
	r.printf("")

	rungs := make([]rungResult, 0, len(heights))
	stopped := ""

	for i, h := range heights {
		if outOfTime() {
			stopped = fmt.Sprintf("deadline reached before the %d-row round", h)
			break
		}
		roundStart := time.Now()
		r.section(fmt.Sprintf("round %d/%d -- height %d", i+1, len(heights), h))

		// --- F: the rung itself -------------------------------------------
		res := measureRung(r, self, host, *useBundled, *width, h, *fillLines, *longWidth, *wideWidth)
		rungs = append(rungs, res)
		r.printf("F   %s", res.line())
		for _, n := range res.Notes {
			r.printf("      note: %s", n)
		}
		if res.CreateOK {
			r.printf("F   child sees window %dx%d, buffer %dx%d; inside alt %dx%d; taller buffer: %s",
				res.ChildCols, res.ChildRows, res.ChildBufW, res.ChildBufH,
				res.AltChildCols, res.AltChildRows, res.ChildSetBufTallerDetal)
		}

		// --- F risk: a program drawing at the top of a tall window ---------
		if res.CreateOK && !outOfTime() {
			measureTopDraw(r, self, host, *useBundled, *width, h)
		}

		// --- baseline: this build's emission shape, re-asked at this scale -
		if res.CreateOK && !outOfTime() {
			measureEmissionShape(r, self, host, *useBundled, *width, h)
		}

		// --- C: what a width-aware program prints -------------------------
		if res.CreateOK && !outOfTime() {
			measureWidthAware(r, host, *useBundled, *width, *wideWidth, h)
		}

		// --- A: pipes, once, in the first round ---------------------------
		if !*skipPipes && i == 0 && !outOfTime() {
			r.printf("A   candidates over pipes, with no console at all:")
			for _, p := range measurePipes(r) {
				r.printf("A     %s", p.line())
			}
			r.printf("A   'NOTHING' means the program needs a console and cannot be routed to")
			r.printf("A   pipes; anything that works is a candidate for f4 to host directly.")
		}

		r.printf("--- round %d done in %s; partial verdict: %s",
			i+1, time.Since(roundStart).Round(time.Millisecond), ladderVerdict(rungs, *budgetMs))
		r.printf("")
	}

	r.section("summary")
	if stopped != "" {
		r.printf("STOPPED EARLY: %s", stopped)
		r.printf("everything above is complete for the scales it covers.")
	}
	r.printf("VERDICT(F): %s", ladderVerdict(rungs, *budgetMs))
	r.printf("")
	r.printf("%-8s %-24s %-14s %s", "height", "at start (win/buf)", "inside alt", "SetConsoleScreenBufferSize taller")
	for _, res := range rungs {
		if !res.CreateOK {
			continue
		}
		r.printf("%-8d %-24s %-14s %s",
			res.Height,
			fmt.Sprintf("%dx%d / %dx%d", res.ChildCols, res.ChildRows, res.ChildBufW, res.ChildBufH),
			fmt.Sprintf("%dx%d", res.AltChildCols, res.AltChildRows),
			res.ChildSetBufTallerDetal)
	}
	r.printf("")
	r.printf("Reading: if 'at start' reports the full round height, height-aware programs")
	r.printf("(dir /p, more, Write-Progress) lay out for it -- risk 4 of section 10.")
	r.printf("")

	r.section("D3 -- bundled host")
	if host.found {
		r.printf("conpty.dll found next to the probe: %s (version %s)", host.path, host.version)
		r.printf("re-run with -bundled to put the whole ladder through it and diff the two logs.")
	} else {
		r.printf("no conpty.dll next to the probe. To measure D3, drop the signed pair")
		r.printf("(OpenConsole.exe + conpty.dll) from the Microsoft.Windows.Console.ConPTY")
		r.printf("NuGet package beside this exe and re-run with -bundled.")
	}
	r.printf("")
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
// reporting, with a heartbeat
//
// A probe that prints nothing for three minutes is indistinguishable from a
// hung one, and the tester is right to kill it. Every wait that can be long
// says what it is waiting for and how long it has been waiting.
// ---------------------------------------------------------------------------

type reporter struct {
	mu      sync.Mutex
	out     *os.File
	echo    *os.File
	started time.Time
}

func (r *reporter) printf(format string, a ...any) {
	line := fmt.Sprintf(format, a...)
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintln(r.out, line)
	fmt.Fprintln(r.echo, line)
	r.out.Sync()
}

// tick prints only to the console: the log stays readable, the tester sees
// the probe is alive.
func (r *reporter) tick(format string, a ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fmt.Fprintf(r.echo, "      ... %s\n", fmt.Sprintf(format, a...))
}

func (r *reporter) section(title string) {
	r.printf("=== %s %s", title, strings.Repeat("=", max(0, 66-len(title))))
}

func (r *reporter) phase(format string, a ...any) {
	if verbose {
		r.printf("      %s", fmt.Sprintf(format, a...))
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

type heartbeat struct {
	stop chan struct{}
	done chan struct{}
}

func startHeartbeat(r *reporter, label string) *heartbeat {
	h := &heartbeat{stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(h.done)
		start := time.Now()
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-h.stop:
				return
			case <-t.C:
				r.tick("%s (%s elapsed, %s of the run's budget left)",
					label, time.Since(start).Round(time.Second), timeLeft().Round(time.Second))
			}
		}
	}()
	return h
}

func (h *heartbeat) stopIt() {
	close(h.stop)
	<-h.done
}

func waitMarker(r *reporter, c *collector, off int, marker, label string, timeout time.Duration) int {
	hb := startHeartbeat(r, "waiting for "+label)
	defer hb.stopIt()
	return c.waitForMarker(off, marker, capped(timeout))
}

func waitSettled(r *reporter, c *collector, quiet, timeout time.Duration, label string) {
	hb := startHeartbeat(r, "draining "+label)
	defer hb.stopIt()
	c.waitQuiet(quiet, capped(timeout))
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
// smoke test
//
// The cheapest possible question, asked before anything expensive: does a
// pseudoconsole on this machine deliver bytes at all? The first field run of
// this probe spent five minutes timing out on a session that never produced
// one, and the log could not say so because nothing ever asked.
// ---------------------------------------------------------------------------

func smokeTest(r *reporter, self string, host bundledHost, useBundled bool) bool {
	r.section("smoke test")
	ok := false

	cases := []struct {
		name string
		argv []string
		want string
	}{
		{"cmd /c echo", []string{"cmd.exe", "/d", "/c", "echo", "probe-alive"}, "probe-alive"},
		{"our own child", []string{self, "-emit", "topdraw"}, markerDone},
	}

	for _, c := range cases {
		s, err := newSession(80, 25, c.argv, host, useBundled)
		if err != nil {
			r.printf("%-16s CreatePseudoConsole failed: %v", c.name, err)
			continue
		}
		deadline := time.Now().Add(capped(6 * time.Second))
		got := false
		for time.Now().Before(deadline) {
			if strings.Contains(string(s.col.since(0)), c.want) {
				got = true
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		bytes := s.col.size()
		r.printf("%-16s %6d bytes, marker %v, child %s, host %s",
			c.name, bytes, got, s.childStatus(), s.hostDescription())
		if bytes > 0 {
			ok = true
		}
		s.Close(host)
	}
	r.printf("")
	if ok {
		r.printf("the pseudoconsole delivers output; proceeding to the rounds.")
	}
	return ok
}

// ---------------------------------------------------------------------------
// one rung of the ladder
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

func measureRung(r *reporter, self string, host bundledHost, useBundled bool,
	width, height, fillLines, longWidth, wideWidth int) rungResult {

	res := rungResult{Height: height, LinesAsked: fillLines}
	settle := settleFor(height)
	limit := waitFor(height)

	args := []string{self, "-emit", "main", "-fill", strconv.Itoa(fillLines), "-long", strconv.Itoa(longWidth)}

	r.phase("creating a %dx%d pseudoconsole", width, height)
	t0 := time.Now()
	hb := startHeartbeat(r, fmt.Sprintf("CreatePseudoConsole %dx%d", width, height))
	s, err := newSession(width, height, args, host, useBundled)
	hb.stopIt()
	res.CreateMs = time.Since(t0).Milliseconds()
	if err != nil {
		res.CreateNo = err.Error()
		return res
	}
	defer s.Close(host)
	res.CreateOK = true
	res.HostName = s.hostName
	res.HostRSSAfterCreateKB = workingSetKB(s.hostPID)

	// --- history -----------------------------------------------------------
	fillStart := time.Now()
	off := waitMarker(r, s.col, 0, markerDone, fmt.Sprintf("%d history lines", fillLines), limit)
	res.FillMs = time.Since(fillStart).Milliseconds()
	if off < 0 {
		if s.col.silent() {
			// Nothing at all arrived. The remaining seven phases of this rung
			// would each wait out their own timeout and measure the same
			// nothing, so the rung ends here.
			res.Notes = append(res.Notes,
				fmt.Sprintf("SILENT session: zero bytes in %s. child: %s; host: %s. "+
					"Remaining phases skipped.", res.durFill(), s.childStatus(), s.hostDescription()))
			return res
		}
		res.Notes = append(res.Notes, "the child never reported DONE; everything below is unreliable")
		off = s.col.mark()
	}
	waitSettled(r, s.col, settle, limit, "the history")
	full := s.col.since(0)
	res.FillBytes = len(full)
	res.LinesSeen, res.LowestSeen, res.HighestSeen = countFillMarkers(full)
	res.HostRSSAfterFillKB = workingSetKB(s.hostPID)

	if sizes := parseSizeLines(full); len(sizes) > 0 {
		if v, ok := sizes["start"]; ok {
			res.ChildCols, res.ChildRows, res.ChildBufW, res.ChildBufH = v[0], v[1], v[2], v[3]
		}
	}

	// --- a width change: does conhost re-wrap and re-send everything? ------
	waitMarker(r, s.col, off, markerReady, "the child to hand over", 15*time.Second)
	waitSettled(r, s.col, settle, limit, "before the width change")

	mark := s.col.mark()
	t1 := time.Now()
	r.phase("width %d -> %d", width, width-1)
	if err := s.resize(width-1, height, host); err != nil {
		res.Notes = append(res.Notes, "resize to width-1 failed: "+err.Error())
	}
	waitSettled(r, s.col, settle, limit, "the reflow repaint")
	res.ReflowMs = time.Since(t1).Milliseconds()
	frame := s.col.since(mark)
	shape := analyseFrame(frame, width-1)
	res.ReflowBytes = shape.bytes
	res.ReflowMarkers = shape.fillMarkers
	res.ReflowLowest = shape.lowestMarker
	res.ReflowHighest = shape.highestMarker
	res.ReflowStartsAtHome = shape.startsAtHome
	res.ReflowHidesCursor = shape.hidesCursor
	res.ReflowSizeReport = shape.sizeReport

	// --- the F4 trick: one wide frame, every logical line rejoined ---------
	mark = s.col.mark()
	t2 := time.Now()
	r.phase("width %d -> %d (the rejoin frame)", width-1, wideWidth)
	if err := s.resize(wideWidth, height, host); err != nil {
		res.Notes = append(res.Notes, "widen failed: "+err.Error())
	} else {
		waitSettled(r, s.col, settle, limit, "the wide repaint")
		res.WideMs = time.Since(t2).Milliseconds()
		wideFrame := s.col.since(mark)
		res.WideBytes = len(wideFrame)
		res.WideLongRows = lineRows(wideFrame, wideWidth, markerLongStart)
		if res.WideLongRows == 1 && lineRows(wideFrame, wideWidth, markerLongEnd) != 1 {
			res.Notes = append(res.Notes, "the long line's start and end landed on different rows at the wide width")
		}
	}

	mark = s.col.mark()
	r.phase("restoring width %d", width)
	if err := s.resize(width, height, host); err != nil {
		res.Notes = append(res.Notes, "restore failed: "+err.Error())
	} else {
		waitSettled(r, s.col, settle, limit, "the restore repaint")
		res.RestoreOK = true
		after := analyseFrame(s.col.since(mark), width)
		res.AfterRestoreM = after.fillMarkers
	}

	// --- the alternate screen ---------------------------------------------
	s.writeInput("\r\n")
	if waitMarker(r, s.col, mark, markerAltDone, "the alt-screen phase", 30*time.Second) < 0 {
		res.Notes = append(res.Notes, "the alt-screen phase did not complete")
	}
	waitSettled(r, s.col, settle, 15*time.Second, "the alt-screen tail")
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

	s.writeInput("\r\n")
	return res
}

// ---------------------------------------------------------------------------
// the other directions, asked at the round's scale
// ---------------------------------------------------------------------------

func measureTopDraw(r *reporter, self string, host bundledHost, useBundled bool, width, height int) {
	args := []string{self, "-emit", "topdraw"}
	s, err := newSession(width, height, args, host, useBundled)
	if err != nil {
		r.printf("Frisk could not create a %dx%d session: %v", width, height, err)
		return
	}
	defer s.Close(host)
	waitMarker(r, s.col, 0, markerDone, "the top-draw child", waitFor(height))
	waitSettled(r, s.col, settleFor(height), 15*time.Second, "the top-draw frame")
	raw := s.col.since(0)
	g := newWideGrid(width)
	g.feed(raw)
	rowOfTop := -1
	for i, row := range g.rows {
		if strings.Contains(string(row), "~TOPDRAW~") {
			rowOfTop = i
			break
		}
	}
	r.printf("Frisk ESC[1;1H text in a %d-row session landed on stream row %d -- that many rows",
		height, rowOfTop)
	r.printf("Frisk above the visible slice (the Write-Progress class of problem).")
}

func measureWidthAware(r *reporter, host bundledHost, useBundled bool, narrow, wide, height int) {
	// Bounded height: this question is about width, and a 32000-row console
	// would only make the same answer slower.
	h := height
	if h > 200 {
		h = 200
	}
	argv := []string{"cmd.exe", "/d", "/c", "dir", "/w", "/-p", `%SystemRoot%\System32\drivers\etc`}
	for _, w := range []int{narrow, wide} {
		if outOfTime() {
			return
		}
		s, err := newSession(w, h, argv, host, useBundled)
		if err != nil {
			r.printf("C   dir /w at width %-5d session failed: %v", w, err)
			continue
		}
		waitSettled(r, s.col, 600*time.Millisecond, 20*time.Second, fmt.Sprintf("dir /w at width %d", w))
		raw := s.col.since(0)
		g := newWideGrid(w)
		g.feed(raw)
		longest, nonEmpty := 0, 0
		for _, row := range g.rows {
			t := strings.TrimRight(string(row), " ")
			if t != "" {
				nonEmpty++
				if len(t) > longest {
					longest = len(t)
				}
			}
		}
		r.printf("C   dir /w at width %-5d rows=%-4d longest row=%-5d bytes=%d", w, nonEmpty, longest, len(raw))
		s.Close(host)
	}
	r.printf("C   a jump in 'longest row' between %d and %d is a program formatting for the", narrow, wide)
	r.printf("C   width it is told -- the reason direction C was demoted.")
}

func measureEmissionShape(r *reporter, self string, host bundledHost, useBundled bool, width, height int) {
	h := height
	if h > 200 {
		h = 200
	}
	args := []string{self, "-emit", "main", "-fill", "20", "-long", strconv.Itoa(width + width/2)}
	s, err := newSession(width, h, args, host, useBundled)
	if err != nil {
		r.printf("base session failed: %v", err)
		return
	}
	defer s.Close(host)
	waitMarker(r, s.col, 0, markerDone, "the emission-shape child", 20*time.Second)
	waitSettled(r, s.col, 300*time.Millisecond, 15*time.Second, "the live stream")
	live := s.col.since(0)

	rowsAtWidth := lineRows(live, width, markerLongStart)
	hardCRLF := strings.Contains(string(live), markerLongStart) && crlfWithinLongLine(string(live))

	mark := s.col.mark()
	s.resize(width-1, h, host)
	waitSettled(r, s.col, 300*time.Millisecond, 20*time.Second, "the shape repaint")
	frame := analyseFrame(s.col.since(mark), width-1)

	r.printf("base long line: start marker on %d row(s) at width %d; hard CRLF inside the wrap (P6 shape): %v",
		rowsAtWidth, width, hardCRLF)
	r.printf("base repaint: hides cursor=%v  starts at home=%v  size report=%v",
		frame.hidesCursor, frame.startsAtHome, frame.sizeReport)
}

// crlfWithinLongLine reports whether a CRLF appears between the long line's
// markers, which is the 19045-era shape (P6) as opposed to the whole-line
// autowrap shape (P11/P12).
func crlfWithinLongLine(s string) bool {
	i := strings.Index(s, markerLongStart)
	j := strings.Index(s, markerLongEnd)
	if i < 0 || j < 0 || j < i {
		return false
	}
	return strings.Contains(s[i:j], "\r\n")
}
