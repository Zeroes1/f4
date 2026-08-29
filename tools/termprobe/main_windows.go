//go:build windows

package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// termprobe answers, in one run, every question the surviving directions in
// docs/CONPTY_RESEARCH.md depend on -- not just the most likely one.
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
// Nothing here is destructive: the probe owns its pseudoconsoles and its
// children, writes only to its own log, and changes no setting.

func main() {
	var (
		emit       = flag.String("emit", "", "internal: run as the child inside a pseudoconsole")
		fillLines  = flag.Int("fill", 400, "history lines the child prints per rung")
		longWidth  = flag.Int("long", 600, "length of the long line used for the rejoin test")
		width      = flag.Int("width", 120, "pseudoconsole width for the ladder")
		wideWidth  = flag.Int("wide", 4000, "width to widen to for the rejoin frame")
		heightsCSV = flag.String("heights", "", "comma separated ladder (default 125,250,...,32000)")
		budgetMs   = flag.Int64("budget-ms", 250, "repaint budget used to pick the recommended height")
		quick      = flag.Bool("quick", false, "only the 125..2000 rungs")
		skipPipes  = flag.Bool("skip-pipes", false, "skip the direction A measurements")
		useBundled = flag.Bool("bundled", false, "use conpty.dll next to the probe if present")
		logPath    = flag.String("log", "", "write the report here (default termprobe-<pid>.log next to the exe)")
	)
	flag.Parse()

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

	r := &reporter{out: lf, echo: os.Stdout}
	r.printf("termprobe -- one run, every surviving direction of docs/CONPTY_RESEARCH.md")
	r.printf("started %s", time.Now().Format(time.RFC3339))
	r.printf("")

	host := findBundledHost()
	reportEnvironment(r, host, *useBundled)

	self, err := os.Executable()
	if err != nil {
		r.printf("cannot find own path: %v", err)
		return
	}

	r.section("F -- the height ladder")
	r.printf("width %d, %d history lines per rung, long line %d chars, wide frame %d columns",
		*width, *fillLines, *longWidth, *wideWidth)
	r.printf("")

	rungs := make([]rungResult, 0, len(heights))
	for _, h := range heights {
		res := measureRung(r, self, host, *useBundled, *width, h, *fillLines, *longWidth, *wideWidth)
		rungs = append(rungs, res)
		r.printf("%s", res.line())
		for _, n := range res.Notes {
			r.printf("        note: %s", n)
		}
	}
	r.printf("")
	r.printf("VERDICT(F): %s", ladderVerdict(rungs, *budgetMs))
	r.printf("")

	r.section("F -- what the child believes, per rung")
	r.printf("%-8s %-24s %-24s %s", "height", "at start (win/buf)", "inside alt (win/buf)", "SetConsoleScreenBufferSize taller")
	for _, res := range rungs {
		if !res.CreateOK {
			continue
		}
		r.printf("%-8d %-24s %-24s %s",
			res.Height,
			fmt.Sprintf("%dx%d / %dx%d", res.ChildCols, res.ChildRows, res.ChildBufW, res.ChildBufH),
			fmt.Sprintf("%dx%d", res.AltChildCols, res.AltChildRows),
			res.ChildSetBufTallerDetal)
	}
	r.printf("")
	r.printf("Reading: if 'at start' reports the full ladder height, height-aware programs")
	r.printf("(dir /p, more, Write-Progress) will lay out for it -- risk 4 of section 10.")
	r.printf("")

	r.section("F -- a program drawing at the top of a tall window")
	measureTopDraw(r, self, host, *useBundled, *width, tallestCreated(rungs))
	r.printf("")

	r.section("C -- what a width-aware program prints at 4000 columns")
	measureWidthAware(r, host, *useBundled, *wideWidth)
	r.printf("")

	r.section("baseline -- this build's emission shape")
	measureEmissionShape(r, self, host, *useBundled, *width)
	r.printf("")

	if !*skipPipes {
		r.section("A -- candidates over pipes, with no console at all")
		for _, p := range measurePipes() {
			r.printf("%s", p.line())
		}
		r.printf("")
		r.printf("Reading: 'NOTHING' means the program needs a console and cannot be routed")
		r.printf("to pipes; anything that works is a candidate for f4 to host directly.")
		r.printf("")
	}

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

	r.printf("report written to %s", path)
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

func tallestCreated(rungs []rungResult) int {
	best := 0
	for _, r := range rungs {
		if r.CreateOK && r.Height > best {
			best = r.Height
		}
	}
	if best == 0 {
		best = 500
	}
	return best
}

// ---------------------------------------------------------------------------
// reporting
// ---------------------------------------------------------------------------

type reporter struct {
	out  *os.File
	echo *os.File
}

func (r *reporter) printf(format string, a ...any) {
	line := fmt.Sprintf(format, a...)
	fmt.Fprintln(r.out, line)
	fmt.Fprintln(r.echo, line)
	r.out.Sync()
}

func (r *reporter) section(title string) {
	r.printf("=== %s %s", title, strings.Repeat("=", max(0, 70-len(title))))
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

	args := []string{self, "-emit", "main", "-fill", strconv.Itoa(fillLines), "-long", strconv.Itoa(longWidth)}

	t0 := time.Now()
	s, err := newSession(width, height, args, host, useBundled)
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
	off := s.col.waitForMarker(0, markerDone, 90*time.Second)
	res.FillMs = time.Since(fillStart).Milliseconds()
	if off < 0 {
		res.Notes = append(res.Notes, "the child never reported DONE; everything below is unreliable")
		off = s.col.mark()
	}
	s.col.waitQuiet(250*time.Millisecond, 20*time.Second)
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
	s.col.waitForMarker(off, markerReady, 15*time.Second)
	s.col.waitQuiet(200*time.Millisecond, 10*time.Second)

	mark := s.col.mark()
	t1 := time.Now()
	if err := s.resize(width-1, height, host); err != nil {
		res.Notes = append(res.Notes, "resize to width-1 failed: "+err.Error())
	}
	s.col.waitQuiet(400*time.Millisecond, 60*time.Second)
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
	if err := s.resize(wideWidth, height, host); err != nil {
		res.Notes = append(res.Notes, "widen failed: "+err.Error())
	} else {
		s.col.waitQuiet(400*time.Millisecond, 60*time.Second)
		res.WideMs = time.Since(t2).Milliseconds()
		wideFrame := s.col.since(mark)
		res.WideBytes = len(wideFrame)
		res.WideLongRows = lineRows(wideFrame, wideWidth, markerLongStart)
		if res.WideLongRows == 1 && lineRows(wideFrame, wideWidth, markerLongEnd) != 1 {
			res.Notes = append(res.Notes, "the long line's start and end landed on different rows at the wide width")
		}
	}

	mark = s.col.mark()
	if err := s.resize(width, height, host); err != nil {
		res.Notes = append(res.Notes, "restore failed: "+err.Error())
	} else {
		s.col.waitQuiet(400*time.Millisecond, 60*time.Second)
		res.RestoreOK = true
		after := analyseFrame(s.col.since(mark), width)
		res.AfterRestoreM = after.fillMarkers
	}

	// --- the alternate screen ---------------------------------------------
	s.writeInput("\r\n")
	altOff := s.col.waitForMarker(mark, markerAltDone, 30*time.Second)
	if altOff < 0 {
		res.Notes = append(res.Notes, "the alt-screen phase did not complete")
	}
	s.col.waitQuiet(300*time.Millisecond, 10*time.Second)
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
// the other directions' questions
// ---------------------------------------------------------------------------

func measureTopDraw(r *reporter, self string, host bundledHost, useBundled bool, width, height int) {
	args := []string{self, "-emit", "topdraw"}
	s, err := newSession(width, height, args, host, useBundled)
	if err != nil {
		r.printf("could not create a %dx%d session: %v", width, height, err)
		return
	}
	defer s.Close(host)
	s.col.waitForMarker(0, markerDone, 30*time.Second)
	s.col.waitQuiet(300*time.Millisecond, 10*time.Second)
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
	r.printf("a %d-row session: text drawn at ESC[1;1H landed on stream row %d; the visible", height, rowOfTop)
	r.printf("slice would be the last rows, so anything drawn 'at the top of the window'")
	r.printf("is %d rows above it. This is the Write-Progress class of problem.", rowOfTop)
}

func measureWidthAware(r *reporter, host bundledHost, useBundled bool, wide int) {
	type probeCmd struct {
		name string
		argv []string
	}
	cmds := []probeCmd{
		{"cmd dir /w", []string{"cmd.exe", "/d", "/c", "dir", "/w", "/-p", `%SystemRoot%\System32\drivers\etc`}},
		{"powershell Format-Wide", []string{"powershell", "-NoProfile", "-NonInteractive", "-Command",
			"Get-ChildItem $env:SystemRoot\\System32\\drivers\\etc | Format-Wide -AutoSize | Out-String"}},
	}
	for _, c := range cmds {
		for _, w := range []int{120, wide} {
			s, err := newSession(w, 200, c.argv, host, useBundled)
			if err != nil {
				r.printf("%-24s w=%-5d session failed: %v", c.name, w, err)
				continue
			}
			s.col.waitQuiet(700*time.Millisecond, 30*time.Second)
			raw := s.col.since(0)
			g := newWideGrid(w)
			g.feed(raw)
			longest := 0
			nonEmpty := 0
			for _, row := range g.rows {
				t := strings.TrimRight(string(row), " ")
				if t != "" {
					nonEmpty++
					if len(t) > longest {
						longest = len(t)
					}
				}
			}
			r.printf("%-24s w=%-5d rows=%-4d longest row=%-5d bytes=%d",
				c.name, w, nonEmpty, longest, len(raw))
			s.Close(host)
		}
	}
	r.printf("Reading: a big jump in 'longest row' between 120 and %d is a program that", wide)
	r.printf("formats for the width it is told -- the reason direction C was demoted.")
}

func measureEmissionShape(r *reporter, self string, host bundledHost, useBundled bool, width int) {
	args := []string{self, "-emit", "main", "-fill", "40", "-long", strconv.Itoa(width + width/2)}
	s, err := newSession(width, 200, args, host, useBundled)
	if err != nil {
		r.printf("session failed: %v", err)
		return
	}
	defer s.Close(host)
	s.col.waitForMarker(0, markerDone, 30*time.Second)
	s.col.waitQuiet(300*time.Millisecond, 10*time.Second)
	live := s.col.since(0)

	rowsAtWidth := lineRows(live, width, markerLongStart)
	hardCRLF := strings.Contains(string(live), markerLongStart) &&
		crlfWithinLongLine(string(live))

	mark := s.col.mark()
	s.resize(width-1, 200, host)
	s.col.waitQuiet(400*time.Millisecond, 30*time.Second)
	frame := analyseFrame(s.col.since(mark), width-1)

	r.printf("live stream: the long line's start marker occupies %d row(s) at width %d", rowsAtWidth, width)
	r.printf("live stream: a hard CRLF inside the wrapped line (the 19045/P6 shape): %v", hardCRLF)
	r.printf("repaint: hides cursor=%v  starts at home=%v  size report=%v",
		frame.hidesCursor, frame.startsAtHome, frame.sizeReport)
	r.printf("Reading: these are the P6/P11/P12/P14 findings re-asked on this build; they")
	r.printf("belong in the conptyBehaviour table of the mocks.")
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
