package panel

import (
	"strings"
	"testing"

	"github.com/unxed/f4/internal/config"
	"github.com/unxed/vtui"
)

// The columns of the drive rows are ruled with Far's solid line, and a drive
// whose parameters are hidden (or not known) keeps the rules in the same
// places as the other rows (mitek1977-coder, f4#1148, list of 16.10, points 1
// and 2: a removable drive used to end after its type, with no rules).
func TestDriveMenuRowsKeepTheRulesOfHiddenParameters(t *testing.T) {
	options := config.DriveMenuShowType | config.DriveMenuShowLabel | config.DriveMenuShowFilesystem | config.DriveMenuShowSize
	rows := []DriveMenuPlatformRow{
		{Base: "C:", Kind: "fixed", Label: "Win10", Filesystem: "NTFS", Total: "953.0 GiB", Free: "300.6 GiB"},
		{Base: "G:", Kind: "removable"},
	}
	lines := DriveMenuPlatformRowsText(rows, options)
	if strings.Contains(lines[0], " | ") || !strings.Contains(lines[0], " │ ") {
		t.Fatalf("the columns are not ruled with the solid line: %q", lines[0])
	}
	rules := func(line string) []int {
		var at []int
		col := 0
		for _, r := range line {
			if r == '│' {
				at = append(at, col)
			}
			col += vtui.StringWidth(string(r))
		}
		return at
	}
	full, hidden := rules(lines[0]), rules(lines[1])
	if len(hidden) != len(full) {
		t.Fatalf("the drive with hidden parameters has %d rules, the others %d:\n%q\n%q", len(hidden), len(full), lines[0], lines[1])
	}
	for i := range full {
		if full[i] != hidden[i] {
			t.Fatalf("rule %d at column %d and %d:\n%q\n%q", i, full[i], hidden[i], lines[0], lines[1])
		}
	}
}
