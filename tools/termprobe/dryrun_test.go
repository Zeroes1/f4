package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestDryRunSchedulerSurvivesEverything is the harness check that would
// otherwise need Wine: it runs the exact task set the -dryrun flag runs,
// including a step that never returns and a step that panics, and proves the
// run completes promptly with every task accounted for.
func TestDryRunSchedulerSurvivesEverything(t *testing.T) {
	tasks := []task{
		{Name: "fast", Budget: 2 * time.Second, Run: func() (string, error) { return "returned at once", nil }},
		{Name: "slow but fine", Budget: 3 * time.Second, Run: func() (string, error) {
			time.Sleep(200 * time.Millisecond)
			return "took 200ms", nil
		}},
		{Name: "hangs forever", Budget: 500 * time.Millisecond, Run: func() (string, error) { select {} }},
		{Name: "panics", Budget: 2 * time.Second, Run: func() (string, error) {
			var p *int
			_ = *p
			return "", nil
		}},
		{Name: "fails", Budget: 2 * time.Second, Run: func() (string, error) { return "", os.ErrInvalid }},
	}
	start := time.Now()
	out := runTasks(tasks, 3, time.Now().Add(20*time.Second), func(stepResult) {})
	if len(out) != 5 {
		t.Fatalf("every task must be accounted for, got %d", len(out))
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("the dry run took %s; a hanging task must not dominate it", elapsed)
	}
	var names []string
	for _, s := range out {
		names = append(names, s.Name+"="+s.Outcome.String())
	}
	joined := strings.Join(names, " ")
	for _, want := range []string{"hangs forever=HUNG", "panics=FAILED", "fails=FAILED", "fast=ok"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in %q", want, joined)
		}
	}
}
