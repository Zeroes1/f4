package multiarc

import (
	"context"
	"strings"
	"testing"
)

// Every other test in this package substitutes runTool/lookupTool
// (withFakeTools), so the production bodies of both -- the actual
// exec.CommandContext call and the actual exec.LookPath call -- are never
// exercised anywhere. This file calls the real, unfaked package vars
// directly, the same way multiarc.go itself does at runtime. It uses the
// "go" binary as its subject rather than a Unix tool such as sh: this
// package's tests run on the linux, macOS and windows CI cells alike (it
// carries no build constraint of its own), and "go" is the one binary
// every one of those cells is guaranteed to have on PATH.

func TestRunToolRealCommandSucceeds(t *testing.T) {
	stdout, _, err := runTool(context.Background(), "go", "version")
	if err != nil {
		t.Fatalf("runTool(go version): %v", err)
	}
	if !strings.Contains(string(stdout), "go version") {
		t.Errorf("stdout = %q, want it to contain %q", stdout, "go version")
	}
}

func TestRunToolRealCommandFails(t *testing.T) {
	_, stderr, err := runTool(context.Background(), "go", "f4-multiarc-not-a-real-subcommand")
	if err == nil {
		t.Fatal("expected an unknown go subcommand to exit non-zero")
	}
	if len(stderr) == 0 {
		t.Error("expected go to explain the unknown subcommand on stderr")
	}
}

func TestRunToolRealCommandNotFound(t *testing.T) {
	if _, _, err := runTool(context.Background(), "f4-multiarc-nonexistent-tool"); err == nil {
		t.Fatal("expected an error for a binary that does not exist")
	}
}

func TestLookupToolReal(t *testing.T) {
	if _, err := lookupTool("go"); err != nil {
		t.Errorf("lookupTool(go): %v", err)
	}
	if _, err := lookupTool("f4-multiarc-nonexistent-tool"); err == nil {
		t.Error("expected lookupTool to fail for a binary that does not exist")
	}
}

func TestToolAvailableReal(t *testing.T) {
	if !toolAvailable("go") {
		t.Error("expected go to be available through the real lookupTool")
	}
	if toolAvailable("f4-multiarc-nonexistent-tool") {
		t.Error("expected a nonexistent tool to be reported unavailable")
	}
}
