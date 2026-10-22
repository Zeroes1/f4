//go:build linux

package main

import (
	"strings"
	"testing"
	"time"
)

// A shell script plays the manager: it asks the terminal, waits for the reply
// and for the F10 key, and writes the sequences of a well-behaved program.
func TestRecordAnswersAndQuits(t *testing.T) {
	script := `printf '\033[?2004h\033[6n'; IFS= read -r -d R pos; ` +
		`case "$pos" in *"1;1") printf 'GOT-DSR';; esac; ` +
		`read -r -n 1 k; printf '\033[?2004l'`
	r, err := Record([]string{"bash", "-c", script}, 300*time.Millisecond, Options{})
	if err != nil {
		t.Fatal(err)
	}
	rec, mark := r.Data, r.Mark
	text := string(rec)
	if !strings.Contains(text, "GOT-DSR") {
		t.Errorf("the cursor position report was not answered: %q", text)
	}
	if mark <= 0 || mark > len(rec) {
		t.Fatalf("mark %d outside the recording of %d bytes", mark, len(rec))
	}
	if !strings.Contains(text[mark:], "\x1b[?2004l") {
		t.Errorf("no output after the quit keys: %q", text)
	}
	rs := Check(Find(rec, mark))
	if got := resultOf(rs, "P-01"); got != "pass" {
		t.Errorf("P-01 = %q: %v", got, rs)
	}
}

// With --far2l the kit answers the activation request with far2lok before the
// answer to the DSR, as P-05 describes; the script reads both in that order.
func TestRecordFar2lAcknowledgesTheExtensions(t *testing.T) {
	script := `printf '\033_far2l1\033\\\033[5n'; IFS= read -r -d n reply; ` +
		`case "$reply" in *far2lok*) printf 'GOT-OK';; esac; ` +
		`read -r -n 1 k; printf '\033_far2l0\033\\'`
	r, err := Record([]string{"bash", "-c", script}, 300*time.Millisecond, Options{Far2l: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(r.Data), "GOT-OK") {
		t.Errorf("far2lok did not come before the DSR answer: %q", r.Data)
	}
	if r.Killed {
		t.Error("the script was killed")
	}
}

// A manager that waits for something the silent terminal never sends is
// killed after the limit and P-06 fails.
func TestRecordSilentExtensionsFailsAHangingManager(t *testing.T) {
	old := exitLimit
	exitLimit = 500 * time.Millisecond
	t.Cleanup(func() { exitLimit = old })
	r, err := Record([]string{"bash", "-c", "sleep 30"}, 100*time.Millisecond, Options{SilentExt: true, Far2l: true})
	if err != nil {
		t.Fatal(err)
	}
	if !r.Killed || CheckExit(r).Result != "fail" {
		t.Errorf("a hanging manager: killed=%v, %v", r.Killed, CheckExit(r))
	}
}
