package panel

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/unxed/f4/internal/sysinfo"
	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestPanelsFrameDeterministicHelpers(t *testing.T) {
	if got := getMenuText(ViewModeBrief, ViewModeBrief, "Brief"); got != "√Brief" {
		t.Fatalf("getMenuText selected = %q", got)
	}
	if got := getMenuText(ViewModeBrief, ViewModeWide, "Wide"); got != " Wide" {
		t.Fatalf("getMenuText unselected = %q", got)
	}
	if got := getSortMenuText(SortName, SortName, "Name"); got != "√Name" {
		t.Fatalf("getSortMenuText selected = %q", got)
	}
	if got := getSortMenuText(SortName, SortSize, "Size"); got != " Size" {
		t.Fatalf("getSortMenuText unselected = %q", got)
	}
	if getToggleMenuText(true, "On") != "√On" || getToggleMenuText(false, "Off") != " Off" {
		t.Fatal("getToggleMenuText result is incorrect")
	}

	if ShellSingleQuote("a'b") != "'a'\\''b'" {
		t.Fatalf("ShellSingleQuote escaped result is incorrect")
	}
	for input, want := range map[string]string{
		"":                      "Terminal",
		"python3 -u script.py":  "Python",
		"/usr/bin/f4 --help":    "f4",
		"\"python3\" script.py": "Python",
	} {
		if got := workspaceCommandName(input); got != want {
			t.Fatalf("workspaceCommandName(%q) = %q, want %q", input, got, want)
		}
	}

	if !isCommandFocusToggleKey(&vtinput.InputEvent{VirtualKeyCode: vtinput.VK_OEM_3}) {
		t.Fatal("OEM grave key was not recognized")
	}
	if !isCommandFocusToggleKey(&vtinput.InputEvent{Char: '`'}) || !isCommandFocusToggleKey(&vtinput.InputEvent{Char: 'ё'}) {
		t.Fatal("text-only grave keys were not recognized")
	}
	if isCommandFocusToggleKey(&vtinput.InputEvent{Char: '~'}) {
		t.Fatal("unrelated text key was recognized as a focus toggle")
	}

	for _, tc := range []struct {
		mode  int
		event *vtinput.InputEvent
		want  bool
	}{
		{0, &vtinput.InputEvent{}, false},
		{1000, &vtinput.InputEvent{}, true},
		{1000, &vtinput.InputEvent{MouseEventFlags: vtinput.MouseMoved}, false},
		{1002, &vtinput.InputEvent{MouseEventFlags: vtinput.MouseMoved}, false},
		{1002, &vtinput.InputEvent{MouseEventFlags: vtinput.MouseMoved, ButtonState: 1}, true},
		{1003, &vtinput.InputEvent{MouseEventFlags: vtinput.MouseMoved}, true},
	} {
		if got := terminalWantsMouseEvent(tc.mode, tc.event); got != tc.want {
			t.Fatalf("terminalWantsMouseEvent(%d, %#v) = %v, want %v", tc.mode, tc.event, got, tc.want)
		}
	}
	if terminalWantsMouseEvent(1000, nil) {
		t.Fatal("nil mouse event was accepted")
	}

	for _, tc := range []struct {
		input string
		path  string
		ok    bool
	}{
		{"cd /tmp/work", "/tmp/work", true},
		{"CHDIR 'two words'", "two words", true},
		{"cd \"two words\"", "two words", true},
		{"cd..", "..", true},
		{"cd ..", "..", true},
		{"cd/", string(os.PathSeparator), true},
		{"printf hi", "", false},
	} {
		path, ok := parseDirChangeCommand(tc.input)
		if path != tc.path || ok != tc.ok {
			t.Fatalf("parseDirChangeCommand(%q) = %q, %v", tc.input, path, ok)
		}
	}
	for _, tc := range []struct {
		input string
		path  string
		ok    bool
	}{
		{"edit:file.txt", "file.txt", true},
		{" EDIT: two words ", "two words", true},
		{"edit:<<capture", "", false},
		{"edit:", "", false},
		{"open:file.txt", "", false},
	} {
		path, ok := parsePlainEditCommand(tc.input)
		if path != tc.path || ok != tc.ok {
			t.Fatalf("parsePlainEditCommand(%q) = %q, %v", tc.input, path, ok)
		}
	}

	root := t.TempDir()
	clean := filepath.Join(root, "folder")
	if !SameFolderHistoryPath(root, filepath.Join(root, ".")) || SameFolderHistoryPath(root, clean) || SameFolderHistoryPath("", root) {
		t.Fatal("SameFolderHistoryPath result is incorrect")
	}
	history := []string{"/new", "/middle", "/old"}
	if pos, path, ok := FolderHistoryStep(history, "/middle", -1, -1); !ok || pos != 2 || path != "/old" {
		t.Fatalf("history back = %d, %q, %v", pos, path, ok)
	}
	if pos, path, ok := FolderHistoryStep(history, "/middle", -1, 1); !ok || pos != 0 || path != "/new" {
		t.Fatalf("history forward = %d, %q, %v", pos, path, ok)
	}
	if _, _, ok := FolderHistoryStep(history, "/old", 2, -1); ok {
		t.Fatal("history moved past the oldest entry")
	}
	if _, _, ok := FolderHistoryStep(nil, "/old", -1, -1); ok {
		t.Fatal("empty history produced a step")
	}

	t.Setenv("LUNOBOT_FRAME_COVERAGE", "value")
	if got := ExpandPathEnv("$LUNOBOT_FRAME_COVERAGE/${LUNOBOT_FRAME_COVERAGE}/%LUNOBOT_FRAME_COVERAGE%"); got != "value/value/value" {
		t.Fatalf("ExpandPathEnv variables = %q", got)
	}
	if got := ExpandPathEnv("$LUNOBOT_FRAME_MISSING/${LUNOBOT_FRAME_MISSING}/%LUNOBOT_FRAME_MISSING%"); got != "$LUNOBOT_FRAME_MISSING/${LUNOBOT_FRAME_MISSING}/%LUNOBOT_FRAME_MISSING%" {
		t.Fatalf("ExpandPathEnv missing variables = %q", got)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got := ExpandPathEnv("~"); got != home || !strings.HasPrefix(ExpandPathEnv("~/child"), home) {
		t.Fatalf("ExpandPathEnv home expansion is incorrect: %q", got)
	}
	for _, c := range []byte{'a', 'Z', '0', '_'} {
		if !isEnvironmentVariableChar(c) {
			t.Fatalf("environment variable character %q rejected", c)
		}
	}
	if isEnvironmentVariableChar('-') {
		t.Fatal("dash accepted in an environment variable name")
	}

	if driveMatchesPath(sysinfo.DriveEntry{Name: "C:"}, filepath.Join(root, "x")) {
		if runtime.GOOS != "windows" {
			t.Fatal("POSIX path matched a Windows drive")
		}
	}
	if IsAIPanel(nil) || IsAIPanel(&FileSystemPanel{}) {
		t.Fatal("non-AI panel was recognized as AI")
	}
	aiVFS := &mockTitleVFS{OSVFS: *vfs.NewOSVFS(root), title: "ai"}
	if !IsAIPanel(&FileSystemPanel{Vfs: aiVFS}) {
		t.Fatal("AI panel was not recognized")
	}

	left := &FileSystemPanel{}
	right := &FileSystemPanel{}
	left.SetPosition(50, 0, 79, 10)
	right.SetPosition(0, 0, 29, 10)
	pf := &PanelsFrame{Panels: [2]Panel{left, right}}
	if pf.VisualLeftFSP() != right || pf.VisualRightFSP() != left {
		t.Fatal("visual panel ordering is incorrect")
	}
	if (&PanelsFrame{}).VisualLeftFSP() != nil || (&PanelsFrame{}).VisualRightFSP() != nil {
		t.Fatal("nil visual panels were not handled")
	}

	pf.SetExitCode(17)
	if !pf.Done || pf.ExitCode != 17 {
		t.Fatal("SetExitCode did not mark the frame done")
	}
	if localPTYFailureMessage(os.ErrClosed) == "" {
		t.Fatal("localPTYFailureMessage returned an empty message")
	}
	if (&remotePTYInterruptTarget{}).Matches(nil) || sameRemotePTYBackend(nil, nil) {
		t.Fatal("nil remote PTY targets matched")
	}
	if !progressBlockedByModal(coverageFrameStack{top: vtui.NewVMenu("modal")}) || progressBlockedByModal(coverageFrameStack{}) {
		t.Fatal("progress modal detection is incorrect")
	}
}

type coverageFrameStack struct {
	top vtui.Frame
}

func (s coverageFrameStack) GetTopFrame() vtui.Frame { return s.top }
