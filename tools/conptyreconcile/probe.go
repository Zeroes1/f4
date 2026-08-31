package main

import "strings"

const (
	probeBeginMarker = "__F4_NATIVE_PROBE_BEGIN__"
	probeEndMarker   = "__F4_NATIVE_PROBE_END__"
)

// probeWorkload exercises host operations that affect the future f4 adapter,
// without trying to predict the host's repaint strategy.  The logical payload
// between markers is the contract the eventual f4 integration must recover.
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
	b.WriteString("\x1b]0;f4-native-openconsole-probe\x07")
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

// probeOutputContainsMarker accepts both a direct byte match and a match in a
// printable compaction. ConPTY is a terminal renderer: resize/reflow may put
// cursor/erase controls and line breaks between adjacent bytes of a logical
// marker (especially at the 1-column edge case), while the marker itself is
// still present in the rendered stream.
func probeOutputContainsMarker(output []byte, marker string) bool {
	if strings.Contains(string(output), marker) {
		return true
	}
	var compact strings.Builder
	compact.Grow(len(output))
	for _, byteValue := range output {
		if byteValue >= 0x20 || byteValue == '\t' {
			compact.WriteByte(byteValue)
		}
	}
	return strings.Contains(compact.String(), marker)
}
