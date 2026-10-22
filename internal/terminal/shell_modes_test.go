package terminal

import "testing"

// The modes a command switches on belong to the command: when the shell gets
// its prompt back (OSC 133;D) bracketed paste, the mouse modes and the
// win32-input mode go back to what the shell had when it started the command
// (OSC 133;C) -- docs/FAR2L_COMPAT.md, T-07; unxed/f4#2160.

type modeSnapshot struct {
	paste, sgr, win32 bool
	mouse             int
}

func modesOf(tv *TerminalView) modeSnapshot {
	return modeSnapshot{tv.BracketedPasteMode, tv.MouseSGRMode, tv.Win32InputMode, tv.MouseTrackingMode}
}

func TestShellModes_CommandModesEndWithTheCommand(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, &mockPty{})

	p.Process([]byte("\x1b]133;C\x07"))
	p.Process([]byte("\x1b[?2004h\x1b[?9001h\x1b[?1000h\x1b[?1006h"))
	if got := modesOf(tv); !got.paste || !got.win32 || !got.sgr || got.mouse != 1000 {
		t.Fatalf("the command's modes were not switched on: %+v", got)
	}
	p.Process([]byte("\x1b]133;D;0\x07"))
	if got := modesOf(tv); got != (modeSnapshot{}) {
		t.Fatalf("modes of the finished command survive: %+v", got)
	}
}

func TestShellModes_ShellOwnModesComeBack(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, &mockPty{})

	p.Process([]byte("\x1b[?2004h")) // the shell's own, for its prompt
	p.Process([]byte("\x1b]133;C\x07"))
	p.Process([]byte("\x1b[?2004l\x1b[?1002h")) // the command turns the paste off and the mouse on
	p.Process([]byte("\x1b]133;D\x07"))
	if got := modesOf(tv); !got.paste || got.mouse != 0 {
		t.Fatalf("after the command: %+v, want the shell's own paste mode on and the mouse off", got)
	}
}

func TestShellModes_NoCommandNoChange(t *testing.T) {
	tv := NewTerminalView(80, 24)
	defer tv.Close()
	p := NewAnsiParser(tv, &mockPty{})

	// the shell sets its modes at the prompt, with no command running: they stay
	p.Process([]byte("\x1b[?2004h\x1b]133;D\x07"))
	if !tv.BracketedPasteMode {
		t.Fatal("a D mark without a C mark switched the shell's paste mode off")
	}
}
