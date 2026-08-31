package main

import "strings"

const (
	probeBeginMarker = "__PINNED_CONPTY_PROBE_BEGIN__"
	probeEndMarker   = "__PINNED_CONPTY_PROBE_END__"
)

// probeWorkload exercises host operations any terminal must handle. Logical
// records are delimited by the explicit CRLF bytes authored here; no display
// row is used as a line boundary.
func probeWorkload() string { return probeWorkloadForWidth(80) }

func probeWorkloadForWidth(width int) string {
	if width < 1 {
		width = 80
	}
	var b strings.Builder
	b.WriteString("\x1b[2J\x1b[H")
	b.WriteString(probeBeginMarker)
	b.WriteString("\r\n")
	b.WriteString("ascii: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\r\n")
	for _, item := range []struct {
		name  string
		count int
		value byte
	}{
		{"exact-n-minus-1", width - 1, 'N'},
		{"exact-n", width, 'N'},
		{"exact-n-plus-1", width + 1, 'N'},
		{"exact-2n-plus-1", 2*width + 1, 'N'},
	} {
		b.WriteString(item.name)
		b.WriteString(": ")
		prefix := len(item.name) + 2
		b.WriteString(strings.Repeat(string(item.value), maxInt(0, item.count-prefix)))
		b.WriteString("\r\n")
	}
	b.WriteString("width-edge: ")
	b.WriteString(strings.Repeat("B", width))
	b.WriteString("\r\n")
	b.WriteString("repeat-char: ")
	b.WriteString(strings.Repeat("R", 97))
	b.WriteString("\r\n")
	b.WriteString("alternating: ")
	for i := 0; i < 129; i++ {
		if i%2 == 0 {
			b.WriteByte('0')
		} else {
			b.WriteByte('1')
		}
	}
	b.WriteString("\r\n")
	b.WriteString("spaces:       \r\n")
	b.WriteString("empty:\r\n")
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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
