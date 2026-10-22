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
	rec, mark, err := Record([]string{"bash", "-c", script}, 300*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
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
