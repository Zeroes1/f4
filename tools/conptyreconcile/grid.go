package main

// Grid is the reconcile tool's reading of a byte stream, kept under its old
// name so the call sites did not have to change. It is now an adapter over
// the 1:1 conhost port (msrow.go, mstextbuffer.go, msdispatch.go,
// msreflow.go, mscwd.go, msengine.go); the hand-written cursor model that
// used to live in this file violated THE RULE at the head of
// docs/CONPTY_RESEARCH.md -- it claimed Microsoft's semantics while being a
// reimplementation from observed behaviour -- and is gone.

type Grid struct {
	t *msTerminal
}

func NewGrid(width int) *Grid { return NewGridSized(width, 0) }

// NewGridSized takes the console height. Zero means "the stream will
// announce its geometry, or nothing scrolls": see msDefaultUnsizedHeight.
func NewGridSized(width, height int) *Grid {
	return &Grid{t: newMsTerminal(width, height)}
}

// Feed applies the stream through the ported dispatch.
func (g *Grid) Feed(b []byte) { g.t.Feed(b) }

// LogicalLinesWithWidth returns the logical lines the buffer holds, read
// the way TextBuffer::Reflow reads rows: joined on WasWrapForced, text up
// to MeasureRight, tagged with the width in force.
func (g *Grid) LogicalLinesWithWidth() []liveLine {
	return g.t.logicalLines()
}

// LogicalLines is LogicalLinesWithWidth without the widths.
func (g *Grid) LogicalLines() []string {
	lines := g.t.logicalLines()
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = l.Text
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
