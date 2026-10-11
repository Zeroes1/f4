package macro

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// f4#1870 (DkmS1953): with nothing recorded, the assign dialog says before a
// key is pressed that the key's macro will be removed.
func TestAssignDialogWarnsAboutAnEmptyRecording(t *testing.T) {
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	text := func(f *MacroAssignFrame) string {
		var b strings.Builder
		for _, child := range f.GetChildren() {
			if tx, ok := child.(*vtui.Text); ok {
				b.WriteString(tx.GetText())
				b.WriteString(" ")
			}
		}
		return b.String()
	}
	firstWord := strings.Fields(i18n.Msg("Macro.AssignEmpty"))[0]

	empty := NewMacroAssignFrame(&MacroManager{})
	if !strings.Contains(text(empty), firstWord) {
		t.Errorf("no warning in the dialog for an empty recording: %q", text(empty))
	}
	for _, e := range vtui.ValidateLayout(empty.Window) {
		t.Errorf("layout: %v", e)
	}

	recorded := NewMacroAssignFrame(&MacroManager{Buffer: []*vtinput.InputEvent{{Type: vtinput.KeyEventType, KeyDown: true, Char: 'a'}}})
	if strings.Contains(text(recorded), firstWord) {
		t.Errorf("the warning shows although keys were recorded: %q", text(recorded))
	}
}
