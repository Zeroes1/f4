package app

import (
	"testing"

	"github.com/unxed/vtui"
)

// screenshotHotkeyRows are the widest cells of the shortcut list from issue
// #1239: a long command name, the longest chord, the area and a long
// "when" condition.
func screenshotHotkeyRows() []hotkeyRow {
	return []hotkeyRow{
		{
			Label:     "Appearance and language settings dialog",
			Key:       "Ctrl+Alt+Shift+F12",
			Area:      "Terminal",
			Condition: "FrameworkNoTerminalCtrlNWorkspace",
			Desc:      "Open the settings on the page that holds this group of options",
		},
		{Label: "Close workspace", Key: "Ctrl+W", Area: "Common", Desc: "Close the current workspace"},
	}
}

func fixedColumnsWidth(columns []vtui.TableColumn) int {
	total := 0
	for _, c := range columns[:len(columns)-1] {
		total += c.Width
	}
	return total
}

// TestIssue1239KeyAndAreaKeepTheirWidthInAPane: when the list does not fit,
// the free-text columns (command, condition) have to give way. The key and the
// area are short, and a chord cut to "Ctrl+Alt+Sh" or an area cut to "Com" says
// nothing.
func TestIssue1239KeyAndAreaKeepTheirWidthInAPane(t *testing.T) {
	rows := screenshotHotkeyRows()
	const pane = 80
	columns := hotkeyTableColumns(rows, pane)

	for _, col := range []int{1, 2} {
		want := vtui.StringWidth(rows[0].GetCellText(col))
		if got := columns[col].Width; got < want {
			t.Errorf("column %q is %d wide, want all %d cells of its longest value", columns[col].Title, got, want)
		}
	}
}

// TestIssue1239DescriptionKeepsARoom: the description is the only column that
// is elastic, and it used to be left with just its header's width.
func TestIssue1239DescriptionKeepsARoom(t *testing.T) {
	rows := screenshotHotkeyRows()
	const pane = 80
	columns := hotkeyTableColumns(rows, pane)

	// The dialog spends 4 cells on padding and one on each column separator.
	room := pane - 4 - (len(columns) - 1) - fixedColumnsWidth(columns)
	if room < 16 {
		t.Errorf("description gets %d cells of the %d-wide pane, want at least 16", room, pane)
	}
}

// TestIssue1239NarrowPaneStillFits: a pane too narrow for everything must not
// be overflowed, and the chord must still be readable.
func TestIssue1239NarrowPaneStillFits(t *testing.T) {
	rows := screenshotHotkeyRows()
	const pane = 60
	columns := hotkeyTableColumns(rows, pane)

	room := pane - 4 - (len(columns) - 1) - fixedColumnsWidth(columns)
	if room < vtui.StringWidth(columns[len(columns)-1].Title) {
		t.Errorf("columns overflow the %d-wide pane: description has %d cells left", pane, room)
	}
	if got := columns[1].Width; got < 12 {
		t.Errorf("key column is %d wide, want at least 12", got)
	}
}
