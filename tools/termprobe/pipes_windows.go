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
	// DETACHED_PROCESS, not CREATE_NO_WINDOW. The two are easy to confuse and
	// the difference is the entire measurement: CREATE_NO_WINDOW gives the
	// child a *hidden console*, so a run using it had `mode con` succeed and
	// PowerShell 5.1 report pipe-ok -- which contradicted the recorded finding
	// that PS5 needs a console, because the child had one. DETACHED_PROCESS
	// gives no console at all, which is what direction A is about.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: detachedProcess}
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

// measurePipes runs every candidate with a short timeout and announces each
// one before it starts: a tester watching a silent probe cannot tell a slow
// wsl.exe from a hung one.
func measurePipes() []pipeResult {
	wide := []string{"COLUMNS=200", "LINES=50", "TERM=xterm-256color", "COLORTERM=truecolor"}
	specs := []struct {
		name    string
		timeout time.Duration
		env     []string
		argv    []string
	}{
		{"cmd /c echo", 8 * time.Second, nil, []string{"cmd", "/c", "echo", "hello"}},
		// A program that genuinely needs a console: the control that proves
		// the split A relies on is measurable at all.
		{"cmd /c mode con", 8 * time.Second, nil, []string{"cmd", "/c", "mode", "con"}},
		{"powershell 5.1", 15 * time.Second, nil, []string{"powershell", "-NoProfile", "-NonInteractive", "-Command", "Write-Output pipe-ok"}},
		{"pwsh 7+", 15 * time.Second, nil, []string{"pwsh", "-NoProfile", "-NonInteractive", "-Command", "Write-Output pipe-ok"}},
		// Does anything honour COLUMNS with no console to ask?
		{"pwsh width", 15 * time.Second, wide, []string{"pwsh", "-NoProfile", "-NonInteractive", "-Command", "$Host.UI.RawUI.WindowSize"}},
		{"wsl echo", 12 * time.Second, nil, []string{"wsl.exe", "-e", "echo", "pipe-ok"}},
		{"wsl stty size", 12 * time.Second, wide, []string{"wsl.exe", "-e", "sh", "-c", "stty size 2>&1; echo COLUMNS=$COLUMNS"}},
		{"wsl colour", 12 * time.Second, wide, []string{"wsl.exe", "-e", "sh", "-c", "printf '\\033[38;2;255;96;0mRGB\\033[0m\\n'"}},
		{"ssh -V", 8 * time.Second, nil, []string{"ssh", "-V"}},
		{"git branch --column", 12 * time.Second, wide, []string{"git", "--no-pager", "branch", "--column=always"}},
	}

	out := make([]pipeResult, 0, len(specs))
	for _, sp := range specs {
		out = append(out, runPipe(sp.name, sp.timeout, sp.env, sp.argv...))
	}
	return out
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
