package panel

import (
	"testing"

	"github.com/unxed/f4/internal/i18n"
	"github.com/unxed/f4/internal/testutil"
	"github.com/unxed/vtui"
)

// f4#1148 (mitek1977-coder): the Ins/F4 dialog of the drive menu's Links has
// Name: right under the top border, one blank row between the path and the
// hot key and none elsewhere, and its buttons right under a rule and right
// above the bottom border (12.10 list, point 7). f4#2171 puts the global hot
// key on the row of the menu hot key, at the right, with its example under it
// ending where the field ends, as in the ticket's picture.
func TestDriveLinkDialogRows(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	vtui.FrameManager.Init(scr)
	d := NewDriveBookmarkEditDialog(DriveBookmark{}, "/tmp", nil)
	var sep *vtui.Separator
	var buttons []*vtui.Button
	var texts []*vtui.Text
	for _, child := range d.GetChildren() {
		switch c := child.(type) {
		case *vtui.Separator:
			sep = c
		case *vtui.Checkbox:
			t.Errorf("the dialog has a check box (%q): the global hot key needs none", c.GetText())
		case *vtui.Button:
			buttons = append(buttons, c)
		case *vtui.Text:
			texts = append(texts, c)
		}
	}
	if sep == nil || len(buttons) != 2 {
		t.Fatalf("separator %v, %d buttons", sep != nil, len(buttons))
	}
	var hint, globalHint *vtui.Text
	for _, text := range texts {
		switch text.GetText() {
		case i18n.Msg("DriveLink.HotkeyHintShort"):
			hint = text
		case i18n.Msg("DriveLink.GlobalHotkeyHint"):
			globalHint = text
		}
	}
	if hint == nil || globalHint == nil {
		t.Fatalf("hint %v, global hint %v", hint != nil, globalHint != nil)
	}
	_, nameY, _, _ := d.nameEdit.GetPosition()
	_, pathY, _, _ := d.pathEdit.GetPosition()
	hotkeyX, hotkeyY, hotkeyX2, _ := d.HotkeyEdit.GetPosition()
	globalX, globalY, globalX2, _ := d.globalEdit.GetPosition()
	hintX, hintY, hintX2, _ := hint.GetPosition()
	_, globalHintY, globalHintX2, _ := globalHint.GetPosition()
	_, sepY, _, _ := sep.GetPosition()
	_, buttonY, _, _ := buttons[0].GetPosition()
	if pathY != nameY+2 {
		t.Errorf("Path field at row %d, Name field at %d: want only the Path label between them", pathY, nameY)
	}
	if nameY != d.Y1+2 {
		t.Errorf("Name field at row %d, top border at %d: want only the Name label between them", nameY, d.Y1)
	}
	if hotkeyY != pathY+2 {
		t.Errorf("Hot key at row %d, Path field at %d: want one blank row between", hotkeyY, pathY)
	}
	if hotkeyX != d.X1+2+vtui.StringWidth(i18n.Msg("DriveLink.Hotkey"))+1 {
		t.Errorf("hotkey field x = %d: want one blank cell after its label", hotkeyX)
	}
	if globalY != hotkeyY {
		t.Errorf("global hot key on row %d, hot key on %d: want one row", globalY, hotkeyY)
	}
	if globalX2 != d.X2-2 {
		t.Errorf("global hot key field ends at %d, the dialog's inner edge is %d", globalX2, d.X2-2)
	}
	if width := globalX2 - globalX + 1; width < driveLinkGlobalFieldWidth {
		t.Errorf("global hot key field is %d wide, want at least %d", width, driveLinkGlobalFieldWidth)
	}
	labelX := globalX - 1 - vtui.StringWidth(i18n.Msg("DriveLink.GlobalHotkey"))
	if labelX <= hotkeyX2+1 || labelX < hintX2+driveLinkColumnGap {
		t.Errorf("global hot key label at %d crowds the hot key (ends %d) or its hint (ends %d)", labelX, hotkeyX2, hintX2)
	}
	if hintY != hotkeyY+1 || globalHintY != hintY {
		t.Errorf("hints on rows %d and %d, hot keys on %d: want both right under", hintY, globalHintY, hotkeyY)
	}
	if hintX != d.X1+2 {
		t.Errorf("hint at x = %d, want it under the label at %d", hintX, d.X1+2)
	}
	if globalHintX2 != globalX2 {
		t.Errorf("global hint ends at %d, the field at %d: want them to end together", globalHintX2, globalX2)
	}
	if sepY != hintY+1 {
		t.Errorf("rule at row %d, hints at %d: want right under them", sepY, hintY)
	}
	if buttonY != sepY+1 {
		t.Errorf("buttons at row %d, rule at %d: want them right under it", buttonY, sepY)
	}
	if buttonY != d.Y2-1 {
		t.Errorf("buttons at row %d, bottom border at %d: want them right above it", buttonY, d.Y2)
	}
	if width := d.X2 - d.X1 + 1; width > 80 {
		t.Errorf("dialog is %d wide on an 80-column screen", width)
	}
}

// On a wide screen the dialog keeps half the window and the global hot key
// field takes the room up to its edge, starting half way.
func TestDriveLinkDialogGlobalFieldGrowsWithTheDialog(t *testing.T) {
	t.Cleanup(testutil.SwapFrameManager(t))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(200, 50)
	vtui.FrameManager.Init(scr)
	d := NewDriveBookmarkEditDialog(DriveBookmark{Name: "Logs", Path: "/var/log", GlobalHotkey: "CtrlAltShiftF11"}, "", nil)
	if width := d.X2 - d.X1 + 1; width != 100 {
		t.Errorf("dialog is %d wide on a 200-column screen, want 100", width)
	}
	globalX, _, globalX2, _ := d.globalEdit.GetPosition()
	inner := d.X2 - d.X1 + 1 - 4
	labelX := globalX - 1 - vtui.StringWidth(i18n.Msg("DriveLink.GlobalHotkey"))
	if labelX != d.X1+2+inner/2 {
		t.Errorf("global hot key label at %d, want half way at %d", labelX, d.X1+2+inner/2)
	}
	if globalX2 != d.X2-2 {
		t.Errorf("field ends at %d, want the inner edge %d", globalX2, d.X2-2)
	}
	if got := d.globalEdit.GetText(); got != "Ctrl+Alt+Shift+F11" {
		t.Errorf("field shows %q, want Ctrl+Alt+Shift+F11", got)
	}
}
