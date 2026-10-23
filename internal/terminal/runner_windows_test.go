//go:build windows

package terminal

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
)

func TestLocalCommandRunnerWindowsStreamsMergedLinesAndExitStatus(t *testing.T) {
	t.Setenv("COMSPEC", "cmd.exe")
	dir := t.TempDir()

	// Minimal probe first: if plain exit-code propagation is broken, the
	// compound assertions below only obscure it.
	probeCode, probeErr := NewLocalCommandRunner().RunCommand(
		context.Background(), dir, `exit 5`, func(string) {})
	if probeErr != nil || probeCode != 5 {
		t.Fatalf("probe: exit code = %d, err = %v, want 5, nil", probeCode, probeErr)
	}

	var got []string
	command := `cd & (set /p F4_TEST_INPUT= || echo stdin-eof) & 1>&2 echo stderr-line& <nul set /p "=partial" & exit 7`
	if !WindowsShellSyntax() {
		command = `pwd; if IFS= read -r line; then exit 91; else printf 'stdin-eof\n'; fi; printf 'stderr-line\n' >&2; printf partial; exit 7`
	}
	code, err := NewLocalCommandRunner().RunCommand(
		context.Background(),
		dir,
		command,
		func(line string) { got = append(got, line) },
	)
	if err != nil {
		t.Fatalf("RunCommand: %v", err)
	}
	if code != 7 {
		t.Fatalf("exit code = %d, want 7", code)
	}
	if len(got) != 4 || !strings.EqualFold(got[0], dir) || got[1] != "stdin-eof" || got[2] != "stderr-line" || got[3] != "partial" {
		t.Fatalf("lines = %#v", got)
	}

	info := NewLocalCommandRunner().CommandRunnerInfo()
	wantDialect := vfs.CommandDialectCmd
	if !WindowsShellSyntax() {
		wantDialect = vfs.CommandDialectPOSIX
	}
	if info.Dialect != wantDialect || info.MaxParallel != 0 {
		t.Fatalf("runner info = %+v", info)
	}
}

func TestLocalCommandRunnerWindowsCancellationKillsProcessTree(t *testing.T) {
	t.Setenv("COMSPEC", "cmd.exe")
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	var once sync.Once
	done := make(chan error, 1)
	go func() {
		command := `ping.exe -n 30 127.0.0.1`
		if !WindowsShellSyntax() {
			command = `sleep 30`
		}
		_, err := NewLocalCommandRunner().RunCommand(ctx, "", command, func(string) {
			once.Do(func() { close(started) })
		})
		done <- err
	}()

	// Starting cmd.exe and ping.exe and reading ping's first line is not what
	// this test measures, and on a busy runner under the race detector, at
	// GOMAXPROCS 1, it ran past the 3 seconds this wait used to allow: the
	// race-windows job failed on "ping child did not start" (unxed/f4#2193's
	// first run). The wait ends at ping's first line, so a passing run is no
	// slower. The wait below is the measurement and stays short: a cancel
	// that left the tree running would take ping's whole half minute.
	select {
	case <-started:
	case <-time.After(30 * time.Second):
		cancel()
		<-done
		t.Fatal("ping child did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunCommand error = %v, want context.Canceled", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("RunCommand did not return after process-tree cancellation")
	}
}
