//go:build linux || darwin

package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/unxed/f4/internal/terminal"
)

// TestTerminalStartOpensNamedFileInViewer starts `f4 sub/notes.txt` in a
// terminal, the way issue #991 asks to use it, and reads what f4 draws there:
// the file's text, which only the viewer shows. It takes the whole road the
// file travels -- the command line, the client, the ATTACH datagram to the
// session daemon, and the viewer opened on attach -- that the unit tests
// check a piece at a time. The harness is the one of
// TestTerminalStartOpensItsDirectoryOverTheSession.
func TestTerminalStartOpensNamedFileInViewer(t *testing.T) {
	if testing.Short() {
		t.Skip("starts f4 and its session daemon in a pty")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skipf("cannot locate the test binary to run as f4: %v", err)
	}

	root := startupTestRoot(t)
	home := filepath.Join(root, "home")
	configHome := filepath.Join(root, "config")
	tmp := filepath.Join(root, "tmp")
	logDir := filepath.Join(root, "log")
	here := filepath.Join(root, "here")
	sub := filepath.Join(here, "sub")
	for _, dir := range []string{home, configHome, tmp, logDir, here, sub} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// The text differs from the file name, which a panel shows too.
	const contentMarker = "viewmark991"
	writeStartupFixture(t, filepath.Join(sub, "notes.txt"), "first line\n"+contentMarker+" in the file\n")

	env := []string{
		runAsF4Env + "=1",
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + configHome,
		"TMPDIR=" + tmp,
		"PWD=" + here,
		"PATH=" + os.Getenv("PATH"),
		"SHELL=/bin/sh",
		"HISTFILE=/dev/null",
		"TERM=xterm-256color",
		"COLORTERM=truecolor",
		"LANG=en_US.UTF-8",
		"VTUI_DEBUG=" + filepath.Join(logDir, "f4.log"),
	}
	for _, name := range []string{"USER", "LOGNAME"} {
		if value, ok := os.LookupEnv(name); ok {
			env = append(env, name+"="+value)
		}
	}

	pty, err := terminal.NewPTY()
	if err != nil {
		t.Skipf("PTY allocation unavailable in this environment: %v", err)
	}
	pty.SetSize(startTermCols, startTermRows)
	screen := newStartupScreen(startTermCols, startTermRows)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 32*1024)
		for {
			n, err := pty.Master.Read(buf)
			if n > 0 {
				screen.Feed(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// A relative path, resolved against the directory the shell is in.
	// #nosec G204 G702 -- exe is this test binary, started as f4 on purpose.
	cmd := exec.Command(exe, filepath.Join("sub", "notes.txt"))
	cmd.Dir = here
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = pty.Slave, pty.Slave, pty.Slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true}
	if err := cmd.Start(); err != nil {
		_ = pty.Close()
		t.Fatalf("start f4 in a pty: %v", err)
	}
	client := &startedProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		client.err = cmd.Wait()
		close(client.done)
	}()
	t.Cleanup(func() { stopStartupTest(t, client, pty, readDone, tmp) })

	shown := func(rows [][]rune) bool {
		for _, row := range rows {
			if strings.Contains(string(row), contentMarker) {
				return true
			}
		}
		return false
	}
	// The very first start with an empty profile builds its configuration and
	// takes seconds.
	deadline := time.Now().Add(60 * time.Second)
	var rows [][]rune
	for time.Now().Before(deadline) {
		select {
		case <-client.done:
			t.Fatalf("f4 exited before the file was shown: %v\nscreen:\n%s\nf4 debug log (tail):\n%s",
				client.err, formatStartupScreen(screen.Settled(time.Second)), startupLogTail(logDir, 120))
		default:
		}
		var complete bool
		rows, complete = screen.Snapshot()
		if complete && shown(rows) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	rows = screen.Settled(5 * time.Second)
	if !shown(rows) {
		t.Fatalf("`f4 sub/notes.txt` in a terminal must show the file in the viewer (issue #991); its text %q is not on the screen.\n"+
			"screen:\n%s\nf4 debug log (tail):\n%s", contentMarker, formatStartupScreen(rows), startupLogTail(logDir, 120))
	}
}
