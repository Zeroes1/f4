//go:build windows

package main

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
)

// Direction A: programs that never touch the Windows console API can be
// started on plain pipes, in which case f4 is their terminal by construction
// and the whole wrap question does not arise for them. The doc records A as
// "not disproved, not demonstrated": PowerShell 5 turned out to need a
// console, WSL was not installed on the test machine, and pwsh was untested.
// These are the same questions, asked again wherever the probe runs.

type pipeResult struct {
	Name    string
	Command string
	Bytes   int
	Err     string
	First   string
	UTF16   bool
}

func runPipe(name string, timeout time.Duration, env []string, argv ...string) pipeResult {
	res := pipeResult{Name: name, Command: strings.Join(argv, " ")}
	cmd := exec.Command(argv[0], argv[1:]...)
	// No console at all: this is the whole point of the measurement.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}

	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(timeout):
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		<-done
		res.Err = "timeout"
	}

	res.Bytes = len(out)
	if err != nil && res.Err == "" {
		res.Err = err.Error()
	}
	res.UTF16 = looksUTF16LE(out)
	if res.UTF16 {
		out = []byte(decodeUTF16LE(out))
	}
	res.First = firstLine(out)
	return res
}

func looksUTF16LE(b []byte) bool {
	if len(b) < 4 {
		return false
	}
	zeros := 0
	limit := min(len(b), 64)
	for i := 1; i < limit; i += 2 {
		if b[i] == 0 {
			zeros++
		}
	}
	return zeros*4 >= limit
}

func decodeUTF16LE(b []byte) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		u = append(u, uint16(b[i])|uint16(b[i+1])<<8)
	}
	return string(utf16.Decode(u))
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 110 {
		s = s[:110] + "..."
	}
	return s
}

func measurePipes() []pipeResult {
	wide := []string{"COLUMNS=200", "LINES=50", "TERM=xterm-256color", "COLORTERM=truecolor"}
	return []pipeResult{
		runPipe("cmd /c echo", 10*time.Second, nil, "cmd", "/c", "echo", "hello"),
		// A program that genuinely needs a console: the control that proves
		// the split A relies on is measurable at all.
		runPipe("cmd /c mode con", 10*time.Second, nil, "cmd", "/c", "mode", "con"),
		runPipe("powershell 5.1", 25*time.Second, nil, "powershell", "-NoProfile", "-NonInteractive", "-Command", "Write-Output pipe-ok"),
		runPipe("pwsh 7+", 25*time.Second, nil, "pwsh", "-NoProfile", "-NonInteractive", "-Command", "Write-Output pipe-ok"),
		// Does anything honour COLUMNS with no console to ask?
		runPipe("pwsh width", 25*time.Second, wide, "pwsh", "-NoProfile", "-NonInteractive", "-Command", "$Host.UI.RawUI.WindowSize"),
		runPipe("wsl echo", 20*time.Second, nil, "wsl.exe", "-e", "echo", "pipe-ok"),
		runPipe("wsl stty size", 20*time.Second, wide, "wsl.exe", "-e", "sh", "-c", "stty size 2>&1; echo COLUMNS=$COLUMNS"),
		runPipe("wsl colour", 20*time.Second, wide, "wsl.exe", "-e", "sh", "-c", "printf '\\033[38;2;255;96;0mRGB\\033[0m\\n'"),
		runPipe("ssh -V", 10*time.Second, nil, "ssh", "-V"),
		runPipe("git branch --column", 15*time.Second, wide, "git", "--no-pager", "branch", "--column=always"),
	}
}

func (p pipeResult) line() string {
	verdict := "works"
	switch {
	case p.Err != "" && p.Bytes == 0:
		verdict = "NOTHING"
	case p.Err != "":
		verdict = "partial"
	}
	enc := ""
	if p.UTF16 {
		enc = " [UTF-16LE!]"
	}
	e := ""
	if p.Err != "" {
		e = " err=" + p.Err
	}
	return fmt.Sprintf("%-20s %-8s %6dB%s %s%s", p.Name, verdict, p.Bytes, enc, p.First, e)
}
