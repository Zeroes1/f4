//go:build linux

package proclist

import (
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestFormatRSSKiBPicksAReadableUnit(t *testing.T) {
	cases := []struct {
		kb   uint64
		want string
	}{
		{512, "512 K"},
		{1024, "1.0 M"},
		{2560, "2.5 M"},
		{3 * (1 << 20), "3.0 G"},
	}
	for _, c := range cases {
		if got := formatRSSKiB(c.kb); got != c.want {
			t.Errorf("formatRSSKiB(%d) = %q, want %q", c.kb, got, c.want)
		}
	}
}

func TestFormatCPUPercentClampsNegative(t *testing.T) {
	if got := formatCPUPercent(-5); got != "0.0" {
		t.Fatalf("formatCPUPercent(-5) = %q, want %q", got, "0.0")
	}
	if got := formatCPUPercent(12.34); got != "12.3" {
		t.Fatalf("formatCPUPercent(12.34) = %q, want %q", got, "12.3")
	}
}

func TestCompareSamplesOrdersNumericColumnsNotLexically(t *testing.T) {
	// "9" sorts after "10" as text but must sort before it numerically --
	// exactly what a plain Table falls back to without SortCompare.
	a := sample{pid: 9, rssKiB: 9, cpuPercent: 9}
	b := sample{pid: 10, rssKiB: 10, cpuPercent: 10}
	for _, col := range []int{colPID, colMem, colCPU} {
		if compareSamples(a, b, col) >= 0 {
			t.Errorf("column %d: compareSamples(9, 10) did not order 9 before 10", col)
		}
		if compareSamples(b, a, col) <= 0 {
			t.Errorf("column %d: compareSamples(10, 9) did not order 10 after 9", col)
		}
	}
	if compareSamples(a, a, colCPU) != 0 {
		t.Error("compareSamples of equal samples should be 0")
	}
}

func TestCompareSamplesOrdersNameLexically(t *testing.T) {
	a := sample{name: "bash"}
	b := sample{name: "zsh"}
	if compareSamples(a, b, colName) >= 0 {
		t.Error("compareSamples(bash, zsh, colName) should order bash first")
	}
}

func TestProcListPanelWiring(t *testing.T) {
	controller, err := newProcListPanel(vfs.PanelContext{Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	x1, y1, x2, y2 := controller.GetPosition()
	if x1 != 0 || y1 != 0 || x2 != 39 || y2 != 19 {
		t.Fatalf("GetPosition = %d,%d,%d,%d, want 0,0,39,19", x1, y1, x2, y2)
	}

	controller.SetFocus(true)
	if !controller.IsFocused() {
		t.Fatal("SetFocus(true) did not focus the panel")
	}
	controller.SetFocus(false)
	if controller.IsFocused() {
		t.Fatal("SetFocus(false) left the panel focused")
	}

	// newProcListPanel takes an initial synchronous /proc snapshot before
	// returning, so the real process list -- this test binary among it --
	// is already there without waiting for the refresh ticker.
	controller.Show(vtui.NewSilentScreenBuf())
	if name := controller.GetSelectedName(); name == "" {
		t.Fatal("GetSelectedName is empty with the real process list loaded")
	}

	controller.SetContext(vfs.PanelContext{Side: 1})

	// A plain, unmodified key is not claimed: PluginPanelInstance relies on
	// that to close the panel on Escape (internal/panel/plugins.go).
	if controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}) {
		t.Fatal("plain Escape should not be claimed by the table")
	}

	controller.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})

	if err := controller.Close(); err != nil {
		t.Fatalf("Close returned an error: %v", err)
	}
}
