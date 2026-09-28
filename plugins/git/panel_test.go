package git

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// withFakeGit substitutes execGit for the duration of one test, so no test
// in this file ever runs a real git binary or touches a real repository --
// the same seam-substitution style plugins/multiarc's tests use for
// runToolIn.
func withFakeGit(t *testing.T, output string, err error) {
	t.Helper()
	original := execGit
	t.Cleanup(func() { execGit = original })
	execGit = func(context.Context, string, []string) ([]byte, error) {
		return []byte(output), err
	}
}

func TestNewStatusPanelLoadsStatusSynchronously(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb foo.go\n? bar.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatalf("newStatusPanel returned an error: %v", err)
	}
	defer func() { _ = controller.Close() }()

	panel, ok := controller.(*statusPanel)
	if !ok {
		t.Fatalf("controller is %T, want *statusPanel", controller)
	}
	if len(panel.table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(panel.table.Rows))
	}
	if panel.branch != "main" {
		t.Fatalf("branch = %q, want %q", panel.branch, "main")
	}

	x1, y1, x2, y2 := controller.GetPosition()
	if x1 != 0 || y1 != 0 || x2 != 39 || y2 != 19 {
		t.Fatalf("GetPosition = %d,%d,%d,%d, want 0,0,39,19", x1, y1, x2, y2)
	}
}

func TestNewStatusPanelRejectsEmptyPath(t *testing.T) {
	if _, err := newStatusPanel(vfs.PanelContext{}); err == nil {
		t.Fatal("newStatusPanel with an empty Current.Path should return an error")
	}
}

func TestNewStatusPanelSurfacesGitFailure(t *testing.T) {
	withFakeGit(t, "fatal: not a git repository (or any of the parent directories): .git\n", errors.New("exit status 128"))

	_, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/not/a/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err == nil {
		t.Fatal("newStatusPanel should fail when git status fails")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("error = %q, want it to surface git's own stderr", err.Error())
	}
}

func TestStatusPanelF5Refreshes(t *testing.T) {
	withFakeGit(t, "# branch.head main\n? one.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	// Second load reports two files: a real refresh must replace, not
	// append to, the first load's rows.
	execGit = func(context.Context, string, []string) ([]byte, error) {
		return []byte("# branch.head main\n? one.txt\n? two.txt\n"), nil
	}

	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}) {
		t.Fatal("plain F5 was not claimed")
	}
	panel := controller.(*statusPanel)
	if len(panel.table.Rows) != 2 {
		t.Fatalf("rows after F5 = %d, want 2", len(panel.table.Rows))
	}

	// Ctrl+F5 (or any other modified F5) is not this panel's refresh
	// gesture and must fall through to the table like any other unclaimed
	// combination.
	if controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5,
		ControlKeyState: vtinput.LeftCtrlPressed}) {
		t.Fatal("Ctrl+F5 should not be claimed as the refresh gesture")
	}
}

func TestStatusPanelFocusAndSelection(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb foo.go\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	controller.SetFocus(true)
	if !controller.IsFocused() {
		t.Fatal("SetFocus(true) did not focus the panel")
	}
	controller.SetFocus(false)
	if controller.IsFocused() {
		t.Fatal("SetFocus(false) left the panel focused")
	}

	controller.Show(vtui.NewSilentScreenBuf())
	if name := controller.GetSelectedName(); name != "foo.go" {
		t.Fatalf("GetSelectedName() = %q, want %q", name, "foo.go")
	}

	controller.SetContext(vfs.PanelContext{Side: 1})
	controller.ProcessMouse(&vtinput.InputEvent{Type: vtinput.MouseEventType})

	if err := controller.Close(); err != nil {
		t.Fatalf("Close returned an error: %v", err)
	}
}

func TestStatusRowGetCellText(t *testing.T) {
	plain := statusRow{entry: statusEntry{XY: "M.", Path: "foo.go"}}
	if got := plain.GetCellText(colStatus); got != "M." {
		t.Errorf("GetCellText(colStatus) = %q, want %q", got, "M.")
	}
	if got := plain.GetCellText(colPath); got != "foo.go" {
		t.Errorf("GetCellText(colPath) = %q, want %q", got, "foo.go")
	}
	if got := plain.GetCellText(99); got != "" {
		t.Errorf("GetCellText(99) = %q, want empty", got)
	}

	renamed := statusRow{entry: statusEntry{XY: "R.", Path: "new.go", OrigPath: "old.go"}}
	if got := renamed.GetCellText(colPath); got != "old.go -> new.go" {
		t.Errorf("GetCellText(colPath) for a rename = %q, want %q", got, "old.go -> new.go")
	}
}

func TestStatusPanelTitleShowsDetachedHead(t *testing.T) {
	withFakeGit(t, "# branch.head (detached)\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)
	if !strings.Contains(panel.title(), "0") {
		t.Fatalf("title() = %q, want it to include the change count", panel.title())
	}
}
