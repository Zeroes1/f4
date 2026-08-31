package main

import "strings"

const (
	probeBeginMarker = "__PINNED_CONPTY_PROBE_BEGIN__"
	probeEndMarker   = "__PINNED_CONPTY_PROBE_END__"
)

// probeWorkload exercises host operations any terminal must handle,
// without trying to predict the host's repaint strategy.  The logical payload
// between markers is the contract any consumer of this stream must recover.
func probeWorkload() string {
	var b strings.Builder
	b.WriteString("\x1b[2J\x1b[H")
	b.WriteString(probeBeginMarker)
	b.WriteString("\r\n")
	b.WriteString("ascii: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\r\n")
	b.WriteString("width-edge: ")
	b.WriteString(strings.Repeat("B", 80))
	b.WriteString("\r\n")
	b.WriteString("unicode: 漢字 e\u0301 ☕️ 😀 👩‍💻 אבג العربية\r\n")
	b.WriteString("\x1b[31mred\x1b[0m ")
	b.WriteString("\x1b[2K\x1b[1Grewritten\r\n")
	b.WriteString("cursor: one\x1b[1Gtwo\r\n")
	b.WriteString("tabs:\tX\tY\r\n")
	// Repeated identical records catch accidental line coalescing, loss, or
	// deduplication in the terminal history path.
	b.WriteString("repeat: SAME\r\n")
	b.WriteString("repeat: SAME\r\n")
	b.WriteString("repeat: SAME\r\n")
	b.WriteString("\x1b]0;pinned-conpty-probe\x07")
	b.WriteString("alternate-begin\r\n")
	b.WriteString("\x1b[?1049halt-screen\r\n")
	b.WriteString("alternate-end\x1b[?1049l\r\n")
	b.WriteString("long: ")
	b.WriteString(strings.Repeat("C", 257))
	b.WriteString("\r\n")
	b.WriteString(probeEndMarker)
	b.WriteString("\r\n")
	return b.String()
}

func probeExpectedMarkers() []string {
	// Alternate-screen contents are intentionally not required here: ConPTY
	// restores the primary screen and may legitimately omit text written while
	// the alternate buffer was active. The outer markers survive repaint and
	// are the stable handoff contract for this probe.
	return []string{probeBeginMarker, probeEndMarker}
}

func probeOutputContainsMarker(output []byte, marker string) bool {
	return strings.Contains(string(output), marker)
}
