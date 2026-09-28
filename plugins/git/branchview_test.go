package git

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestNewBranchViewLoadsBranchesSynchronously(t *testing.T) {
	withFakeGit(t, "* main\n  feature/foo\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatalf("newBranchView returned an error: %v", err)
	}
	if len(bv.table.Rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(bv.table.Rows))
	}
}

func TestNewBranchViewSurfacesGitFailure(t *testing.T) {
	withFakeGit(t, "fatal: not a git repository\n", errors.New("exit status 128"))

	_, err := newBranchView("/repo", nil)
	if err == nil {
		t.Fatal("newBranchView should fail when git branch fails")
	}
	if !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("error = %q, want it to surface git's own stderr", err.Error())
	}
}

func TestBranchViewF5Refreshes(t *testing.T) {
	withFakeGit(t, "* main\n", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}

	execGit = func(context.Context, string, []string) ([]byte, error) {
		return []byte("* main\n  feature\n"), nil
	}

	if !bv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_F5}) {
		t.Fatal("plain F5 was not claimed")
	}
	if len(bv.table.Rows) != 2 {
		t.Fatalf("rows after F5 = %d, want 2", len(bv.table.Rows))
	}
}

func TestBranchViewEscapeAndF10Close(t *testing.T) {
	for _, key := range []uint16{vtinput.VK_ESCAPE, vtinput.VK_F10} {
		withFakeGit(t, "* main\n", nil)

		bv, err := newBranchView("/repo", nil)
		if err != nil {
			t.Fatal(err)
		}
		if !bv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: key}) {
			t.Fatalf("key %d was not claimed", key)
		}
		if !bv.IsDone() {
			t.Fatalf("key %d did not close the view", key)
		}
	}
}

func TestBranchViewEnterClaimsKeyWithNoRows(t *testing.T) {
	withFakeGit(t, "", nil) // an empty `git branch --list`: reload succeeds with zero rows.

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(bv.table.Rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(bv.table.Rows))
	}
	if !bv.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_RETURN}) {
		t.Fatal("plain Enter was not claimed")
	}
}

func TestSwitchBranchRunsGitSwitchAndReloadsBoth(t *testing.T) {
	withFakeGit(t, "* main\n  feature\n", nil)

	statusController, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = statusController.Close() }()
	status := statusController.(*statusPanel)

	bv, err := newBranchView("/repo", status)
	if err != nil {
		t.Fatal(err)
	}
	// Move the cursor onto "feature" (display position 1: QuickSearch-less
	// sort keeps git's own order, main first).
	bv.table.SelectPos = 1

	var switchArgs []string
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if len(args) > 2 && args[2] == "switch" {
			switchArgs = args
			return []byte("Switched to branch 'feature'\n"), nil
		}
		// Both bv.reload and status.reload run after a successful switch.
		if len(args) > 2 && args[2] == "branch" {
			return []byte("  main\n* feature\n"), nil
		}
		return []byte("# branch.head feature\n"), nil
	}

	bv.switchBranch()

	want := []string{"-c", "core.quotepath=false", "switch", "feature"}
	if !reflect.DeepEqual(switchArgs, want) {
		t.Fatalf("switch args = %v, want %v", switchArgs, want)
	}
	if len(bv.table.Rows) != 2 {
		t.Fatalf("branch rows after switch = %d, want 2 (reloaded)", len(bv.table.Rows))
	}
	if status.branch != "feature" {
		t.Fatalf("status.branch after switch = %q, want %q (underlying panel reloaded)", status.branch, "feature")
	}
}

func TestSwitchBranchFailureLeavesBothPanelsUntouched(t *testing.T) {
	withFakeGit(t, "* main\n  feature\n", nil)

	statusController, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = statusController.Close() }()
	status := statusController.(*statusPanel)

	bv, err := newBranchView("/repo", status)
	if err != nil {
		t.Fatal(err)
	}
	bv.table.SelectPos = 1

	var reloadCalls int
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if len(args) > 2 && args[2] == "switch" {
			return []byte("error: Your local changes to the following files would be overwritten by checkout:\nfoo.go\n"),
				errors.New("exit status 1")
		}
		reloadCalls++
		return []byte("* main\n  feature\n"), nil
	}

	// Not the switch command itself -- this deliberately never attempts to
	// stash or merge on the user's behalf (branchview.go's own doc comment
	// on switchBranch): a failed switch must leave both panels exactly as
	// they were, only surfacing git's own message.
	bv.switchBranch()

	if reloadCalls != 0 {
		t.Fatalf("reload invocations after a failed switch = %d, want 0 (no reload on failure)", reloadCalls)
	}
	if status.branch != "main" {
		t.Fatalf("status.branch after a failed switch = %q, want %q (unchanged)", status.branch, "main")
	}
}

func TestSwitchBranchWithNoRowsIsANoOp(t *testing.T) {
	withFakeGit(t, "", nil)

	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return nil, nil
	}

	bv.switchBranch()

	if calls != 0 {
		t.Fatalf("git invocations from switchBranch with nothing selected = %d, want 0", calls)
	}
}

func TestBranchRowGetCellText(t *testing.T) {
	row := branchRow{entry: branchEntry{Name: "main", Current: true}}
	if got := row.GetCellText(colBranchCurrent); got != "*" {
		t.Errorf("GetCellText(colBranchCurrent) = %q, want %q", got, "*")
	}
	if got := row.GetCellText(colBranchName); got != "main" {
		t.Errorf("GetCellText(colBranchName) = %q, want %q", got, "main")
	}
	if got := row.GetCellText(99); got != "" {
		t.Errorf("GetCellText(99) = %q, want empty", got)
	}

	other := branchRow{entry: branchEntry{Name: "feature", Current: false}}
	if got := other.GetCellText(colBranchCurrent); got != "" {
		t.Errorf("GetCellText(colBranchCurrent) for a non-current branch = %q, want empty", got)
	}
}

func TestBranchViewGetTypeIsUniqueFromDiffViewAndLogView(t *testing.T) {
	withFakeGit(t, "", nil)
	bv, err := newBranchView("/repo", nil)
	if err != nil {
		t.Fatal(err)
	}
	if bv.GetType() == vtui.TypeUser+9 {
		t.Fatalf("BranchView.GetType() collides with DiffView's vtui.TypeUser+9")
	}
	if bv.GetType() == vtui.TypeUser+10 {
		t.Fatalf("BranchView.GetType() collides with LogView's vtui.TypeUser+10")
	}
}

func TestStatusPanelCtrlSOpensBranches(t *testing.T) {
	withFakeGit(t, "# branch.head main\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_S,
		ControlKeyState: vtinput.LeftCtrlPressed}) {
		t.Fatal("Ctrl+S was not claimed")
	}
}

func TestPlainSFallsThroughToTheTable(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return []byte("# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n"), nil
	}

	// Plain 'S' (no Ctrl) must be left for QuickSearch, not treated as the
	// branch gesture -- the same reasoning commit.go's own
	// TestPlainKFallsThroughToTheTable documents for Ctrl+K.
	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_S})

	if calls != 0 {
		t.Fatalf("git invocations after plain 'S' = %d, want 0 (not the branch gesture)", calls)
	}
}
