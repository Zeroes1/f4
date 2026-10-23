package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// The reader behind TestKeyInventory finds a key however the source compares
// an event against it.
func TestKeyInventoryReadsTheKeysOfASource(t *testing.T) {
	const source = `package sample

import vi "github.com/unxed/vtinput"

func handle(e *vi.InputEvent) bool {
	if e.VirtualKeyCode == vi.VK_F5 {
		return true
	}
	if e.VirtualKeyCode == 'A' || 0x42 == e.VirtualKeyCode {
		return true
	}
	switch e.VirtualKeyCode {
	case 'C', vi.VK_TAB:
	}
	if e.Char == '\\' || e.Char == '*' {
		return true
	}
	return false
}

var table = map[uint16]bool{vi.VK_RETURN: true}

func quiet(n int) bool { return n == 'Z' }
`
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "sample.go", source, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	got := keyInventoryOfFile(t, "sample", file, fset)
	want := map[string]string{
		"sample.handle":    "A,B,C,Char '*',F5,OEM_5,TAB",
		"sample.var table": "RETURN",
	}
	if len(got) != len(want) {
		t.Errorf("functions found: %v", got)
	}
	for id, keys := range want {
		if strings.Join(got[id], ",") != keys {
			t.Errorf("%s: keys %v, want %s", id, got[id], keys)
		}
	}

	// A file without vtinput has no key event to compare.
	plain, err := parser.ParseFile(fset, "plain.go", "package plain\nfunc f(c rune) bool { return c == 'A' }\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	if found := keyInventoryOfFile(t, "plain", plain, fset); len(found) != 0 {
		t.Errorf("keys found in a file without vtinput: %v", found)
	}
}

// An audit fails on a chord no action declares, on a key the source compares
// that it does not list, and on a stale navigation key.
func TestKeyInventoryCheckEntry(t *testing.T) {
	ok := keyInventoryCheckEntry("x.f", []string{"F5", "UP"}, keyInventoryEntry{
		areas: []string{"Shell"}, chords: []string{"F5"}, nav: []string{"UP"}, rationale: "copy and move",
	})
	if len(ok) != 0 {
		t.Errorf("a correct audit failed: %v", ok)
	}
	for _, tc := range []struct {
		name  string
		keys  []string
		entry keyInventoryEntry
		want  string
	}{
		{"undeclared chord", []string{"Q"}, keyInventoryEntry{areas: []string{"Shell"}, chords: []string{"CtrlAltShiftQ"}, rationale: "r"}, "no action"},
		{"unlisted key", []string{"F5", "F6"}, keyInventoryEntry{areas: []string{"Shell"}, chords: []string{"F5"}, rationale: "r"}, "does not list"},
		{"stale navigation", []string{"F5"}, keyInventoryEntry{areas: []string{"Shell"}, chords: []string{"F5"}, nav: []string{"UP"}, rationale: "r"}, "no longer compares"},
		{"no rationale", []string{"UP"}, keyInventoryEntry{nav: []string{"UP"}}, "no rationale"},
		{"misspelled chord", []string{"T"}, keyInventoryEntry{areas: []string{"Shell"}, chords: []string{"Ctrl+Alt+T"}, rationale: "r"}, "not"},
	} {
		failures := strings.Join(keyInventoryCheckEntry("x.f", tc.keys, tc.entry), "\n")
		if !strings.Contains(failures, tc.want) {
			t.Errorf("%s: failures %q, want one saying %q", tc.name, failures, tc.want)
		}
	}
}
