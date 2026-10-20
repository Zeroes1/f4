//go:build linux

package sysinfo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"syscall"
	"testing"
	"time"
)

// fakeWindowsInterop makes dir the whole PATH of the test and returns a
// function that installs a script there under a Windows interop name, the way
// WSL exposes wmic.exe and powershell.exe.
func fakeWindowsInterop(t *testing.T) func(name, script string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	return func(name, script string) {
		t.Helper()
		path := filepath.Join(dir, name)
		// #nosec G306 G703 -- the script must be executable and lives in the test's temporary directory.
		if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		// Executing a file moments after writing it can fail with "text file
		// busy" when another thread forked while it was still open for
		// writing. Run it once here, retrying that one error, so the call
		// under test does not meet it.
		for attempt := 0; attempt < 100; attempt++ {
			err := exec.Command(path).Run() // #nosec G204 -- a script this test just wrote
			if err == nil || !errors.Is(err, syscall.ETXTBSY) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("%s stayed busy", path)
	}
}

func TestQueryHostGPUsFromWSLPrefersWmic(t *testing.T) {
	install := fakeWindowsInterop(t)
	install("wmic.exe", `printf 'Name=GPU One\r\n\r\nName=GPU Two\r\n\r\n'`)
	install("powershell.exe", `printf 'never asked\n'`)

	got := queryHostGPUsFromWSL()
	if want := []string{"GPU One", "GPU Two"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("queryHostGPUsFromWSL() = %#v, want %#v from wmic.exe alone", got, want)
	}
}

func TestQueryHostGPUsFromWSLFallsBackToPowerShell(t *testing.T) {
	install := fakeWindowsInterop(t)
	// wmic.exe exists but is no use: once because it fails, once because it
	// prints nothing parseable. Both must lead on to PowerShell.
	install("powershell.exe", `printf 'Shell GPU\r\n\r\n  Second Shell GPU  \r\n'`)

	for name, script := range map[string]string{
		"wmic fails":       `exit 3`,
		"wmic prints junk": `printf 'Caption=something else\r\n'`,
	} {
		install("wmic.exe", script)
		got := queryHostGPUsFromWSL()
		if want := []string{"Shell GPU", "Second Shell GPU"}; !reflect.DeepEqual(got, want) {
			t.Errorf("%s: queryHostGPUsFromWSL() = %#v, want %#v", name, got, want)
		}
	}
}

func TestQueryHostGPUsFromWSLPowerShellOnly(t *testing.T) {
	install := fakeWindowsInterop(t)
	install("powershell.exe", `printf 'Only GPU\n'`)

	if got, want := queryHostGPUsFromWSL(), []string{"Only GPU"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("queryHostGPUsFromWSL() = %#v, want %#v", got, want)
	}
}

func TestQueryHostGPUsFromWSLGivesUpWhenNothingAnswers(t *testing.T) {
	install := fakeWindowsInterop(t)
	// Neither interop binary is on PATH.
	if got := queryHostGPUsFromWSL(); got != nil {
		t.Fatalf("with no interop binaries queryHostGPUsFromWSL() = %#v, want nil", got)
	}

	// wmic.exe prints nothing useful and PowerShell is missing.
	install("wmic.exe", `printf 'Caption=x\r\n'`)
	if got := queryHostGPUsFromWSL(); got != nil {
		t.Fatalf("with an unhelpful wmic.exe queryHostGPUsFromWSL() = %#v, want nil", got)
	}

	// PowerShell is present but fails.
	install("powershell.exe", `exit 1`)
	if got := queryHostGPUsFromWSL(); got != nil {
		t.Fatalf("with a failing powershell.exe queryHostGPUsFromWSL() = %#v, want nil", got)
	}
}
