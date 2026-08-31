//go:build windows

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// TestNativeConPTYReplay is a diagnostic handoff check. It feeds an artifact
// produced by the standalone native probe through PanelsFrame's normal output
// routing and records the resulting f4 log. It is intentionally opt-in until
// the native adapter can deliver the live stream into this package without a
// replay boundary.
func TestNativeConPTYReplay(t *testing.T) {
	path := os.Getenv("F4_NATIVE_CONPTY_REPLAY")
	if path == "" {
		t.Skip("set F4_NATIVE_CONPTY_REPLAY to a native probe report")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report struct {
		Sessions []struct {
			RawOutput     []byte `json:"raw_output"`
			ExpectedInput []byte `json:"expected_input"`
		} `json:"sessions"`
	}
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Sessions) == 0 {
		t.Fatal("native report has no sessions")
	}
	pty := &mockPty{}
	tv := NewTerminalView(80, 25)
	pf := &PanelsFrame{
		pty:            pty,
		termView:       tv,
		parser:         NewAnsiParser(tv, pty),
		terminalRedraw: newTerminalRedrawScheduler(nil),
		shellMode:      ShellModeOwn,
	}
	pf.consumeLocalOutput(pty, report.Sessions[0].RawOutput)
	got := tv.GetAllLogBytes()
	t.Logf("native replay: raw=%d f4_log=%d history_rows=%d piece_table=%d cursor=%d,%d", len(report.Sessions[0].RawOutput), len(got), len(tv.GridHistory), tv.pt.Size(), tv.CursorX, tv.CursorY)
	begin := []byte("__PINNED_CONPTY_PROBE_BEGIN__")
	end := []byte("__PINNED_CONPTY_PROBE_END__")
	gotBegin, gotEnd := bytes.Count(got, begin), bytes.Count(got, end)
	t.Logf("native replay markers: begin=%d end=%d", gotBegin, gotEnd)
	if gotBegin != 1 || gotEnd != 1 {
		t.Fatalf("f4 log marker counts begin=%d end=%d", gotBegin, gotEnd)
	}
	start := bytes.Index(got, begin)
	finish := bytes.Index(got[start+len(begin):], end)
	if finish < 0 {
		t.Fatal("f4 end marker precedes begin marker")
	}
	finish += start + len(begin)
	expectedStart := bytes.Index(report.Sessions[0].ExpectedInput, begin)
	expectedFinish := bytes.Index(report.Sessions[0].ExpectedInput[expectedStart+len(begin):], end)
	if expectedStart < 0 || expectedFinish < 0 {
		t.Fatal("expected input does not contain complete markers")
	}
	expectedFinish += expectedStart + len(begin)
	want := bytes.ReplaceAll(report.Sessions[0].ExpectedInput[expectedStart+len(begin):expectedFinish], []byte("\r\n"), []byte("\n"))
	actual := got[start+len(begin) : finish]
	if !bytes.Equal(actual, want) {
		at := 0
		for at < len(actual) && at < len(want) && actual[at] == want[at] {
			at++
		}
		lo := at - 32
		if lo < 0 {
			lo = 0
		}
		hi := at + 64
		if hi > len(actual) {
			hi = len(actual)
		}
		t.Logf("f4 actual payload=%q", actual)
		t.Fatalf("f4 payload mismatch: got=%d want=%d first_diff=%d got_context=%q want_context=%q", len(actual), len(want), at, actual[lo:hi], want[maxIntNative(0, at-32):minIntNative(len(want), at+64)])
	}
}

func maxIntNative(a, b int) int {
	if a > b {
		return a
	}
	return b
}
func minIntNative(a, b int) int {
	if a < b {
		return a
	}
	return b
}
