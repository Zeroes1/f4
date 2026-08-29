package main

import (
	"strings"
	"testing"

	"github.com/unxed/vtui"
)

// TestIssue863OwnTerminalKeepsPTYHeightWhileBusy guards the resize that makes
// bash redraw a prompt over output which did not end in a newline.
func TestIssue863OwnTerminalKeepsPTYHeightWhileBusy(t *testing.T) {
	vtui.SetDefaultPalette()
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.shellMode = ShellModeOwn
	pf.showPanels = false
	pf.showKeyBar = true
	pf.pty = &mockPty{}

	pf.ResizeConsole(80, 25)
	idleHeight := pf.termView.Height
	if idleHeight <= 0 {
		t.Fatalf("idle own terminal height = %d, want a positive size", idleHeight)
	}

	pf.executing = true
	pf.ResizeConsole(80, 25)
	if pf.termView.Height != idleHeight {
		t.Fatalf("own terminal changed PTY height while busy: got %d, want %d", pf.termView.Height, idleHeight)
	}
	if pf.termView.Y2 >= pf.cmdLine.Y1 {
		t.Fatalf("own terminal overlaps f4 command line: terminal Y2=%d, command line Y1=%d", pf.termView.Y2, pf.cmdLine.Y1)
	}
}

// TestIssue863OwnTerminalDrawsLastOutputRow reproduces a Unix shell prompt
// printed on the same row as command output that has no trailing LF. The f4
// command line must not paint over that row.
func TestIssue863OwnTerminalDrawsLastOutputRow(t *testing.T) {
	vtui.SetDefaultPalette()
	t.Cleanup(swapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())

	pf := NewPanelsFrame()
	defer pf.Close()
	pf.shellMode = ShellModeOwn
	pf.showPanels = false
	pf.showKeyBar = true
	pf.pty = &mockPty{}

	// Start with the shell prompt already in the viewport, then use the same
	// clean-command path as f4's own command line. The shell echoes are muted
	// by production while it runs the command, but its output and next prompt
	// still reach the terminal view.
	pf.ResizeConsole(80, 25)
	pf.parser.Process([]byte("shell$ "))
	pf.termView.PrintCleanCommand("cat cat_tst")
	pf.termView.SetMuted(false)
	pf.parser.Process([]byte("#!/bin/bash\r\necho \"test text\"shell$ "))
	scr := vtui.NewSilentScreenBuf()
	scr.AllocBuf(80, 25)
	pf.Show(scr)

	for y := 0; y < scr.Height(); y++ {
		var row strings.Builder
		for x := 0; x < scr.Width(); x++ {
			row.WriteRune(rune(scr.GetCell(x, y).Char))
		}
		if strings.Contains(row.String(), `echo "test text"shell$`) {
			return
		}
	}
	t.Fatal("last output row was painted over by the f4 command line")
}
