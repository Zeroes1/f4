package main

import "testing"

func TestHostResultsPassAndFail(t *testing.T) {
	files := map[string]string{
		"T01a": "$'\\E[0'\n", "T01b": "$'\\E[12;40'\n", "T01c": "$'\\E[?62;'\n",
		"T03a": "$'\\E[?1'\n", "T05a": "$'\\E_far2lok\\a\\E[0'\n", "T10a": "''\n",
	}
	keyOut := map[string]string{
		"T02a": "\x1b[200~x\ry\x1b[201~", "T02b": "x\ry", "T03b": "\x1b[97;6u",
		"T04c": "\x1b[65;30;97;1;0;1_\x1b[65;30;97;0;0;1_", "T07b": "a\x1b[97;6u",
	}
	got := map[string]string{}
	for _, r := range hostResults(files, keyOut) {
		got[r.ID] = r.Result
	}
	for _, id := range []string{"T-01", "T-02", "T-03", "T-04", "T-05", "T-10"} {
		if got[id] != "pass" {
			t.Errorf("%s = %q, want pass", id, got[id])
		}
	}
	if got["T-07"] != "fail" {
		t.Errorf("T-07 = %q, want fail (the kitty key arrived after the program ended)", got["T-07"])
	}
}

func TestHostResultsMissingAnswersFail(t *testing.T) {
	for _, r := range hostResults(map[string]string{}, map[string]string{}) {
		if r.Result != "fail" {
			t.Errorf("%s = %q with no answers, want fail", r.ID, r.Result)
		}
	}
}

func TestHostProbeScriptsAreEmbedded(t *testing.T) {
	if len(hostProbe1) < 100 || len(hostProbe2) < 100 {
		t.Fatal("the probe scripts are not embedded")
	}
}

func TestCatV(t *testing.T) {
	if got := catV("\x1b[97;6u\r\x01\x7fa\n"); got != "^[[97;6u^M^A^?a\n" {
		t.Errorf("catV = %q", got)
	}
}
