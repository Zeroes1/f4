package main

import "strings"

const (
	probeBeginMarker     = "__PINNED_CONPTY_PROBE_BEGIN__"
	probeEndMarker       = "__PINNED_CONPTY_PROBE_END__"
	controlBeginMarker   = "__PINNED_CONPTY_PROBE_CONTROL_BEGIN__"
	controlEndMarker     = "__PINNED_CONPTY_PROBE_CONTROL_END__"
	alternateBeginMarker = "__PINNED_CONPTY_PROBE_ALT_BEGIN__"
	alternateEndMarker   = "__PINNED_CONPTY_PROBE_ALT_END__"
)

// probeWorkload exercises host operations any terminal must handle. Logical
// records are delimited by the explicit CRLF bytes authored here; no display
// row is used as a line boundary.
func probeWorkload() string { return probeWorkloadForWidth(80) }

func partialProbeWorkload(width int) []byte {
	if width < 1 {
		width = 80
	}
	return []byte("partial: " + strings.Repeat("P", width+40) + "\r\n")
}

func probeWorkloadForWidth(width int) string {
	if width < 1 {
		width = 80
	}
	var b strings.Builder
	b.WriteString(probeBeginMarker)
	b.WriteString("\r\n")
	// Keep the principal long-line proof at the top of the buffer, before
	// cursor/rewrite and alternate-screen operations can trigger repaint.
	b.WriteString("long: ")
	b.WriteString(strings.Repeat("C", 257))
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
	// Repeated identical records catch accidental line coalescing, loss, or
	// deduplication in the terminal history path.
	b.WriteString("repeat: SAME\r\n")
	b.WriteString("repeat: SAME\r\n")
	b.WriteString("repeat: SAME\r\n")
	b.WriteString(probeEndMarker)
	b.WriteString("\r\n")
	return b.String()
}

func controlProbeWorkload() string {
	var b strings.Builder
	b.WriteString(controlBeginMarker)
	b.WriteString("\r\n")
	b.WriteString("\x1b[31mred\x1b[0m ")
	b.WriteString("\x1b[2K\x1b[1Grewritten\r\n")
	b.WriteString("cursor: one\x1b[1Gtwo\r\n")
	b.WriteString("tabs:\tX\tY\r\n")
	b.WriteString("\x1b]0;pinned-conpty-probe\x07\r\n")
	b.WriteString(controlEndMarker)
	b.WriteString("\r\n")
	// Keep the initial screen erase outside the marked control payload: it can
	// itself trigger an absolute repaint before the marker-bearing phase ends.
	b.WriteString("\x1b[2J\x1b[H")
	return b.String()
}

// alternateProbeWorkload is a separate phase because leaving the alternate
// buffer causes the host to repaint the primary buffer.  Keeping that repaint
// out of the primary payload makes the exact-line assertion about history,
// rather than about a frame that the source explicitly describes as redraw.
func alternateProbeWorkload(width int) string {
	if width < 1 {
		width = 80
	}
	var b strings.Builder
	b.WriteString("\x1b[?1049halt-screen\r\n")
	b.WriteString("alternate-end\r\n\x1b[?1049l\r\n")
	// Emit the stable handoff markers after the alternate buffer is restored;
	// the repaint caused by leaving it must not swallow new history markers.
	b.WriteString(alternateBeginMarker)
	b.WriteString("\r\n")
	b.WriteString(alternateEndMarker)
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

func controlExpectedMarkers() []string {
	return []string{controlBeginMarker, controlEndMarker}
}

func alternateExpectedMarkers() []string {
	return []string{alternateBeginMarker, alternateEndMarker}
}

func probeOutputContainsMarker(output []byte, marker string) bool {
	return strings.Contains(string(output), marker)
}
