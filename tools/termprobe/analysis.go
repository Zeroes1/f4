package main

import (
	"bytes"
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// A small VT model, enough to answer "how many rows does this text occupy".
//
// Adapted from tools/conptycprobe: a cursor model rather than a CRLF split,
// because ConPTY emits both shapes depending on the build (P6 vs P11/P12) and
// only a cursor model reads them alike.
// ---------------------------------------------------------------------------

type wideGrid struct {
	width   int
	rows    [][]byte
	x, y    int
	pending bool
	savedX  int
	savedY  int

	// observations made while feeding, used for frame shape questions.
	sawCursorHide bool
	firstCUPHome  bool
	sawAnyCUP     bool
	sizeReport    [2]int // rows, cols; zero when absent
	sawAltEnter   bool
	sawAltLeave   bool
	maxRow        int
}

func newWideGrid(width int) *wideGrid { return &wideGrid{width: width} }

func (g *wideGrid) blankRow() []byte { return bytes.Repeat([]byte{' '}, g.width) }

func (g *wideGrid) ensureRow(y int) {
	for len(g.rows) <= y {
		g.rows = append(g.rows, g.blankRow())
	}
	if y > g.maxRow {
		g.maxRow = y
	}
}

func (g *wideGrid) clearAll() {
	for i := range g.rows {
		g.rows[i] = g.blankRow()
	}
}

func (g *wideGrid) clearToEnd() {
	if g.y < 0 || g.y >= len(g.rows) || g.x >= g.width {
		return
	}
	for i := g.x; i < g.width; i++ {
		g.rows[g.y][i] = ' '
	}
}

func (g *wideGrid) nextRow() {
	g.y++
	g.ensureRow(g.y)
}

func (g *wideGrid) put(c byte) {
	if g.width <= 0 {
		return
	}
	if g.pending {
		g.nextRow()
		g.x = 0
		g.pending = false
	}
	g.ensureRow(g.y)
	if g.x < 0 {
		g.x = 0
	}
	if g.x >= g.width {
		g.x = g.width - 1
	}
	g.rows[g.y][g.x] = c
	if g.x == g.width-1 {
		g.pending = true
	} else {
		g.x++
	}
}

func csiParams(raw string) []int {
	raw = strings.TrimLeft(raw, "?>")
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ";")
	params := make([]int, len(parts))
	for i, part := range parts {
		if part == "" {
			continue
		}
		value := 0
		for _, c := range part {
			if c < '0' || c > '9' {
				value = 0
				break
			}
			value = value*10 + int(c-'0')
		}
		params[i] = value
	}
	return params
}

func csiDefault(params []int, index, fallback int) int {
	if index >= len(params) || params[index] == 0 {
		return fallback
	}
	return params[index]
}

func (g *wideGrid) csi(raw string, final byte) {
	params := csiParams(raw)
	private := strings.HasPrefix(raw, "?")

	switch final {
	case 'H', 'f':
		row := csiDefault(params, 0, 1) - 1
		col := csiDefault(params, 1, 1) - 1
		if !g.sawAnyCUP {
			g.sawAnyCUP = true
			g.firstCUPHome = row == 0 && col == 0
		}
		if row < 0 {
			row = 0
		}
		if col < 0 {
			col = 0
		}
		g.y = row
		g.x = col
		g.pending = false
		g.ensureRow(g.y)
	case 'A':
		g.y -= csiDefault(params, 0, 1)
		if g.y < 0 {
			g.y = 0
		}
		g.pending = false
	case 'B':
		g.y += csiDefault(params, 0, 1)
		g.ensureRow(g.y)
		g.pending = false
	case 'C':
		g.x += csiDefault(params, 0, 1)
		if g.x >= g.width {
			g.x = g.width - 1
		}
		g.pending = false
	case 'D':
		g.x -= csiDefault(params, 0, 1)
		if g.x < 0 {
			g.x = 0
		}
		g.pending = false
	case 'G':
		g.x = csiDefault(params, 0, 1) - 1
		g.pending = false
	case 'd':
		g.y = csiDefault(params, 0, 1) - 1
		g.ensureRow(g.y)
		g.pending = false
	case 'J':
		switch csiDefault(params, 0, 0) {
		case 2, 3:
			g.clearAll()
		default:
			g.clearToEnd()
		}
	case 'K':
		g.clearToEnd()
	case 't':
		// XTWINOPS. ESC[8;rows;cols t is the size report ConPTY sends on
		// some builds (P14).
		if len(params) >= 3 && params[0] == 8 {
			g.sizeReport = [2]int{params[1], params[2]}
		}
	case 'l':
		if private {
			for _, p := range params {
				switch p {
				case 25:
					g.sawCursorHide = true
				case 1049:
					g.sawAltLeave = true
				}
			}
		}
	case 'h':
		if private {
			for _, p := range params {
				if p == 1049 {
					g.sawAltEnter = true
				}
			}
		}
	}
}

func (g *wideGrid) escape(raw []byte) int {
	if len(raw) < 2 {
		return len(raw)
	}
	switch raw[1] {
	case '[':
		for i := 2; i < len(raw); i++ {
			c := raw[i]
			if c >= 0x40 && c <= 0x7e {
				g.csi(string(raw[2:i]), c)
				return i + 1
			}
		}
		return len(raw)
	case ']':
		// OSC, terminated by BEL or ST.
		for i := 2; i < len(raw); i++ {
			if raw[i] == 0x07 {
				return i + 1
			}
			if raw[i] == 0x1b && i+1 < len(raw) && raw[i+1] == '\\' {
				return i + 2
			}
		}
		return len(raw)
	case '7':
		g.savedX, g.savedY = g.x, g.y
		return 2
	case '8':
		g.x, g.y = g.savedX, g.savedY
		return 2
	default:
		return 2
	}
}

func (g *wideGrid) feed(raw []byte) {
	for i := 0; i < len(raw); i++ {
		switch raw[i] {
		case 0x1b:
			step := g.escape(raw[i:])
			if step < 1 {
				step = 1
			}
			i += step - 1
		case '\r':
			g.x = 0
			g.pending = false
		case '\n':
			g.nextRow()
			g.pending = false
		case '\b':
			if g.x > 0 {
				g.x--
			}
			g.pending = false
		case '\t':
			next := ((g.x / 8) + 1) * 8
			if next >= g.width {
				g.pending = true
			} else {
				g.x = next
			}
		default:
			if raw[i] >= 0x20 && raw[i] != 0x7f {
				g.put(raw[i])
			}
		}
	}
}

func (g *wideGrid) rowsContaining(text string) int {
	count := 0
	for _, row := range g.rows {
		if bytes.Contains(row, []byte(text)) {
			count++
		}
	}
	return count
}

// lineRows lays the stream out at the given width and reports how many rows
// carry the text. One row means the emitter sent the line whole.
func lineRows(raw []byte, width int, text string) int {
	g := newWideGrid(width)
	g.feed(raw)
	return g.rowsContaining(text)
}

// ---------------------------------------------------------------------------
// Markers
//
// ASCII only and delimited by '~': the OEM code page mangled non-ASCII in an
// earlier probe's transcript, and a marker that cannot survive the transport
// is not a marker.
// ---------------------------------------------------------------------------

func fillMarker(n int) string { return fmt.Sprintf("~F%06d~", n) }

const (
	markerLongStart = "~L1~"
	markerLongEnd   = "~E1~"
	markerColor     = "~RGB~"
	markerDone      = "~DONE~"
	markerReady     = "~READY~"
	markerAltDone   = "~ALTDONE~"
	markerBye       = "~BYE~"
)

// countFillMarkers reports how many distinct ~Fnnnnnn~ markers the stream
// carries, and the lowest and highest index seen. The spread matters as much
// as the count: it says whether a repaint carried the whole history or only
// its tail.
func countFillMarkers(raw []byte) (count, lowest, highest int) {
	lowest = -1
	highest = -1
	seen := map[int]bool{}
	s := raw
	for {
		i := bytes.Index(s, []byte("~F"))
		if i < 0 {
			break
		}
		s = s[i:]
		if len(s) < 9 || s[8] != '~' {
			s = s[2:]
			continue
		}
		n := 0
		ok := true
		for _, c := range s[2:8] {
			if c < '0' || c > '9' {
				ok = false
				break
			}
			n = n*10 + int(c-'0')
		}
		if ok && !seen[n] {
			seen[n] = true
			if lowest < 0 || n < lowest {
				lowest = n
			}
			if n > highest {
				highest = n
			}
		}
		s = s[9:]
	}
	return len(seen), lowest, highest
}

// ---------------------------------------------------------------------------
// Frame shape (the §1 step-9 questions, re-asked per height)
// ---------------------------------------------------------------------------

type frameShape struct {
	bytes         int
	hidesCursor   bool
	startsAtHome  bool
	sizeReport    [2]int
	sawAltEnter   bool
	sawAltLeave   bool
	distinctRows  int
	fillMarkers   int
	lowestMarker  int
	highestMarker int
}

func analyseFrame(raw []byte, width int) frameShape {
	g := newWideGrid(width)
	g.feed(raw)
	count, lo, hi := countFillMarkers(raw)
	return frameShape{
		bytes:         len(raw),
		hidesCursor:   g.sawCursorHide || bytes.Contains(raw, []byte("\x1b[?25l")),
		startsAtHome:  g.firstCUPHome,
		sizeReport:    g.sizeReport,
		sawAltEnter:   g.sawAltEnter,
		sawAltLeave:   g.sawAltLeave,
		distinctRows:  g.maxRow + 1,
		fillMarkers:   count,
		lowestMarker:  lo,
		highestMarker: hi,
	}
}

// ---------------------------------------------------------------------------
// Ladder results
// ---------------------------------------------------------------------------

type rungResult struct {
	Height int

	// Precise marks a measurement taken alone rather than alongside other
	// sessions; phase 1 timings are upper bounds, phase 2 timings are not.
	Precise bool

	CreateOK bool
	CreateMs int64
	CreateNo string

	HostRSSAfterCreateKB int64
	HostRSSAfterFillKB   int64
	HostName             string

	LinesAsked  int
	LinesSeen   int
	LowestSeen  int
	HighestSeen int
	FillMs      int64
	FillBytes   int

	// The child's own view of the console, which is what width- and
	// height-aware programs make decisions on.
	ChildCols, ChildRows   int
	ChildBufW, ChildBufH   int
	ChildSetBufTallerOK    bool
	ChildSetBufTallerDetal string

	// A width change: does conhost re-wrap and re-transmit the whole history?
	ReflowMs           int64
	ReflowBytes        int
	ReflowMarkers      int
	ReflowLowest       int
	ReflowHighest      int
	ReflowStartsAtHome bool
	ReflowHidesCursor  bool
	ReflowSizeReport   [2]int

	// The F4 trick: one wide frame, every logical line rejoined.
	WideMs        int64
	WideBytes     int
	WideLongRows  int
	RestoreOK     bool
	AfterRestoreM int

	// Alternate screen: the protocol event full-screen programs announce.
	AltEnterSeen bool
	AltLeaveSeen bool
	AltChildRows int
	AltChildCols int

	Notes []string
}

// durFill is the fill phase's duration, used when reporting a silent session.
func (r rungResult) durFill() string {
	return fmt.Sprintf("%dms", r.FillMs)
}

func (r rungResult) line() string {
	if !r.CreateOK {
		return fmt.Sprintf("%-7d create FAILED: %s", r.Height, r.CreateNo)
	}
	tag := " "
	if r.Precise {
		tag = "*"
	}
	rejoined := "no"
	if r.WideLongRows == 1 {
		rejoined = "yes"
	} else if r.WideLongRows == 0 {
		rejoined = "n/a"
	}
	return fmt.Sprintf(
		"%-7d%s create %5dms  host %6dKB->%6dKB  fill %d/%d in %5dms  "+
			"reflow %7dB %5dms carrying %d [%d..%d]  wide4000 %6dB %5dms rejoined=%s  alt h/l=%v/%v",
		r.Height, tag, r.CreateMs,
		r.HostRSSAfterCreateKB, r.HostRSSAfterFillKB,
		r.LinesSeen, r.LinesAsked, r.FillMs,
		r.ReflowBytes, r.ReflowMs, r.ReflowMarkers, r.ReflowLowest, r.ReflowHighest,
		r.WideBytes, r.WideMs, rejoined,
		r.AltEnterSeen, r.AltLeaveSeen)
}

// ladderVerdict turns the rungs into the two sentences a reader needs: the
// tallest height that worked at all, and the tallest whose reflow still
// carried the whole history in a time worth paying.
func ladderVerdict(rungs []rungResult, budgetMs int64) string {
	tallestCreated := 0
	tallestCarrying := 0
	tallestInBudget := 0
	for _, r := range rungs {
		if !r.CreateOK {
			continue
		}
		if r.Height > tallestCreated {
			tallestCreated = r.Height
		}
		carriedAll := r.LinesSeen > 0 && r.ReflowMarkers >= r.LinesSeen
		if carriedAll && r.Height > tallestCarrying {
			tallestCarrying = r.Height
		}
		if carriedAll && r.ReflowMs <= budgetMs && r.Height > tallestInBudget {
			tallestInBudget = r.Height
		}
	}
	if tallestCreated == 0 {
		return "no height could be created at all -- direction F is closed on this build"
	}
	return fmt.Sprintf(
		"tallest ConPTY created: %d rows; tallest whose width-change repaint carried the whole history: %d rows; "+
			"tallest doing so within %dms: %d rows",
		tallestCreated, tallestCarrying, budgetMs, tallestInBudget)
}

func defaultLadder() []int {
	return []int{125, 250, 500, 1000, 2000, 4000, 8000, 16000, 32000}
}
