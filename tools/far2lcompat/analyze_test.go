package main

import (
	"encoding/base64"
	"strings"
	"testing"
)

func resultOf(rs []Result, id string) string {
	for _, r := range rs {
		if r.ID == id {
			return r.Result
		}
	}
	return ""
}

func TestCheckGoodManager(t *testing.T) {
	rec := "\x1b[?2004h\x1b[?9001h\x1b[>1u\x1b_far2l1\x1b\\" + "SCREEN" +
		"\x1b[<u\x1b[?9001l\x1b[?2004l\x1b_far2l0\x1b\\"
	mark := strings.Index(rec, "SCREEN") + 6
	rs := Check(Find([]byte(rec), mark))
	for _, id := range []string{"P-01", "P-02", "P-03", "P-04", "P-05"} {
		if got := resultOf(rs, id); got != "pass" {
			t.Errorf("%s = %q, want pass: %v", id, got, rs)
		}
	}
}

func TestCheckLeftovers(t *testing.T) {
	rec := "\x1b[?2004h\x1b[?1000h\x1b[?1006h" + "SCREEN" + "\x1b[?1000l"
	mark := strings.Index(rec, "SCREEN") + 6
	rs := Check(Find([]byte(rec), mark))
	if got := resultOf(rs, "P-01"); got != "fail" {
		t.Errorf("P-01 = %q, want fail", got)
	}
	var note string
	for _, r := range rs {
		if r.ID == "P-02" {
			note = r.Note
		}
	}
	if resultOf(rs, "P-02") != "fail" || !strings.Contains(note, "?2004h") || !strings.Contains(note, "?1006h") || strings.Contains(note, "?1000h") {
		t.Errorf("P-02 = %q", note)
	}
}

func TestFar2lKeyIsWhatTheParserOfTheHostReads(t *testing.T) {
	got := far2lKey(0x79, 0x44, 0, true)
	// repeat 1, vk 0x79, scan 0x44, modifiers 0, char 0, 'K'
	want := "\x1b_f2l" + base64.StdEncoding.EncodeToString([]byte{1, 0, 0x79, 0, 0x44, 0, 0, 0, 0, 0, 0, 0, 0, 0, 'K'}) + "\a"
	if got != want {
		t.Errorf("far2lKey = %q, want %q", got, want)
	}
}
