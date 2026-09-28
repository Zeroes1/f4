package git

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
)

func TestHasStagedChangesTrueWhenAnyEntryIsStaged(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n? untracked.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	if !panel.hasStagedChanges() {
		t.Fatal("hasStagedChanges() = false, want true: staged.go has a staged modification")
	}
}

func TestHasStagedChangesFalseWithOnlyUnstagedAndUntracked(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 .M N... 100644 100644 100644 aaaaaaa bbbbbbb modified.go\n? untracked.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	if panel.hasStagedChanges() {
		t.Fatal("hasStagedChanges() = true, want false: nothing in this fixture is staged")
	}
}

func TestHasStagedChangesFalseWithOnlyAnUnresolvedConflict(t *testing.T) {
	// A conflict is never counted as "staged" here (commit.go's own doc
	// comment): stageActionFor always resolves it to "add" (resolve), the
	// same as an untracked file, not "restore" (already staged).
	withFakeGit(t, "# branch.head main\nu UU N... 100644 100644 100644 100644 aaaaaaa bbbbbbb ccccccc conflicted.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	if panel.hasStagedChanges() {
		t.Fatal("hasStagedChanges() = true, want false: an unresolved conflict is not commit-ready")
	}
}

func TestHasStagedChangesFalseWithNoEntriesAtAll(t *testing.T) {
	withFakeGit(t, "# branch.head main\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	if panel.hasStagedChanges() {
		t.Fatal("hasStagedChanges() = true, want false: a clean repository has nothing staged")
	}
}

func TestCtrlKWithNothingStagedNeverRunsGitCommit(t *testing.T) {
	withFakeGit(t, "# branch.head main\n? untracked.txt\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return []byte("# branch.head main\n? untracked.txt\n"), nil
	}

	if !controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_K,
		ControlKeyState: vtinput.LeftCtrlPressed}) {
		t.Fatal("Ctrl+K was not claimed")
	}
	if calls != 0 {
		t.Fatalf("git invocations after Ctrl+K with nothing staged = %d, want 0", calls)
	}
}

func TestPlainKFallsThroughToTheTable(t *testing.T) {
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

	// Plain 'K' (no Ctrl) must be left for QuickSearch, not treated as the
	// commit gesture -- the same reasoning stage.go's own
	// TestCtrlInsertFallsThroughToTheTable documents for the opposite
	// modifier combination.
	controller.ProcessKey(&vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, VirtualKeyCode: vtinput.VK_K})

	if calls != 0 {
		t.Fatalf("git invocations after plain 'K' = %d, want 0 (not the commit gesture)", calls)
	}
}

func TestOnCommitMessageEnteredRejectsBlankMessage(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	var calls int
	execGit = func(context.Context, string, []string) ([]byte, error) {
		calls++
		return nil, nil
	}

	for _, blank := range []string{"", "   ", "\t\n"} {
		panel.onCommitMessageEntered(blank)
	}
	if calls != 0 {
		t.Fatalf("git invocations for blank commit messages = %d, want 0", calls)
	}
}

func TestOnCommitMessageEnteredTrimsAndCommits(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	var commitArgs []string
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if args[2] == "commit" {
			commitArgs = args
			return []byte("[main abc1234] a message\n 1 file changed, 1 insertion(+)\n"), nil
		}
		return []byte("# branch.head main\n"), nil
	}

	panel.onCommitMessageEntered("  a message  ")

	want := []string{"-c", "core.quotepath=false", "commit", "-m", "a message"}
	if !reflect.DeepEqual(commitArgs, want) {
		t.Fatalf("commit args = %v, want %v", commitArgs, want)
	}
	if len(panel.table.Rows) != 0 {
		t.Fatalf("rows after commit = %d, want 0 (reload picked up the clean status)", len(panel.table.Rows))
	}
}

func TestRunCommitFailureLeavesThePanelUntouched(t *testing.T) {
	withFakeGit(t, "# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n", nil)

	controller, err := newStatusPanel(vfs.PanelContext{Current: vfs.PanelState{Path: "/repo"}, Bounds: [4]int{0, 0, 39, 19}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = controller.Close() }()
	panel := controller.(*statusPanel)

	var statusCalls int
	execGit = func(_ context.Context, _ string, args []string) ([]byte, error) {
		if args[2] == "commit" {
			return []byte("nothing to commit\n"), errors.New("exit status 1")
		}
		statusCalls++
		return []byte("# branch.head main\n1 M. N... 100644 100644 100644 aaaaaaa bbbbbbb staged.go\n"), nil
	}

	panel.runCommit("a message")

	if statusCalls != 0 {
		t.Fatalf("reload ran %d times after a failed git commit, want 0 (no reload on failure)", statusCalls)
	}
	if len(panel.table.Rows) != 1 {
		t.Fatalf("rows after a failed commit = %d, want 1 (unchanged)", len(panel.table.Rows))
	}
}

func TestFirstOutputLine(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"one line, no newline", "one line, no newline"},
		{"first\nsecond\nthird\n", "first"},
	}
	for _, tt := range tests {
		if got := firstOutputLine(tt.in); got != tt.want {
			t.Errorf("firstOutputLine(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
