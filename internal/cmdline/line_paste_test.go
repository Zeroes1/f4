package cmdline

import (
	"testing"

	"github.com/unxed/vtui"
)

// The clipboard of f4 pasted into the command line takes the path of a
// terminal paste: a line break is one space, so nothing runs as a script
// (docs/FAR2L_COMPAT.md, B-03, B-08; unxed/f4#2168).
func TestCommandLine_PasteTextLineBreaksAreSpaces(t *testing.T) {
	vtui.SetDefaultPalette()
	for _, c := range []struct{ name, in, want string }{
		{"LF", "a\nb", "a b"},
		{"CR", "a\rb", "a b"},
		{"CRLF", "a\r\nb", "a b"},
		{"mixed", "a\r\nb\nc\rd", "a b c d"},
		{"trailing break", "a\n", "a "},
		{"no break", "plain text", "plain text"},
	} {
		t.Run(c.name, func(t *testing.T) {
			cl := NewCommandLine("> ")
			cl.SetPosition(0, 0, 40, 0)
			cl.PasteText(c.in)
			if got := cl.Edit.GetText(); got != c.want {
				t.Fatalf("PasteText(%q): the line is %q, want %q", c.in, got, c.want)
			}
		})
	}
}
