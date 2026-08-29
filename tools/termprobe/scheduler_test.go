package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestWithTimeoutReturnsTheResultWhenTheStepFinishes(t *testing.T) {
	res := withTimeout("quick", time.Second, func() (string, error) {
		return "42 rows", nil
	})
	if res.Outcome != stepOK || res.Summary != "42 rows" {
		t.Fatalf("%+v", res)
	}
}

func TestWithTimeoutReportsAFailureWithoutHanging(t *testing.T) {
	res := withTimeout("bad", time.Second, func() (string, error) {
		return "", errors.New("E_INVALIDARG")
	})
	if res.Outcome != stepFailed || !strings.Contains(res.Err, "E_INVALIDARG") {
		t.Fatalf("%+v", res)
	}
}

func TestWithTimeoutAbandonsAHangingStep(t *testing.T) {
	// This is the case that cost four field runs: an operation that never
	// returns must cost its own budget and nothing more.
	start := time.Now()
	res := withTimeout("hangs", 80*time.Millisecond, func() (string, error) {
		select {} // never returns
	})
	if res.Outcome != stepHung {
		t.Fatalf("%+v", res)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("the watchdog waited %s, which is not a watchdog", time.Since(start))
	}
}

func TestWithTimeoutCatchesAPanicInsteadOfDyingWithIt(t *testing.T) {
	res := withTimeout("panics", time.Second, func() (string, error) {
		var p *int
		_ = *p // nil dereference
		return "", nil
	})
	if res.Outcome != stepFailed || !strings.Contains(res.Err, "panic") {
		t.Fatalf("%+v", res)
	}
}

func TestRunTasksKeepsGoingWhenOneTaskHangs(t *testing.T) {
	tasks := []task{
		{Name: "fast-1", Budget: time.Second, Run: func() (string, error) { return "a", nil }},
		{Name: "hangs", Budget: 60 * time.Millisecond, Run: func() (string, error) { select {} }},
		{Name: "fast-2", Budget: time.Second, Run: func() (string, error) { return "b", nil }},
		{Name: "fast-3", Budget: time.Second, Run: func() (string, error) { return "c", nil }},
	}
	var mu sync.Mutex
	emitted := 0
	res := runTasks(tasks, 2, time.Now().Add(5*time.Second), func(stepResult) {
		mu.Lock()
		emitted++
		mu.Unlock()
	})
	if len(res) != 4 || emitted != 4 {
		t.Fatalf("got %d results, %d emitted", len(res), emitted)
	}
	ok, hung := 0, 0
	for _, r := range res {
		switch r.Outcome {
		case stepOK:
			ok++
		case stepHung:
			hung++
		}
	}
	if ok != 3 || hung != 1 {
		t.Fatalf("ok=%d hung=%d", ok, hung)
	}
}

func TestRunTasksReportsSkippedRatherThanDroppingThem(t *testing.T) {
	tasks := []task{
		{Name: "one", Budget: time.Second, Run: func() (string, error) { return "", nil }},
		{Name: "two", Budget: time.Second, Run: func() (string, error) { return "", nil }},
	}
	res := runTasks(tasks, 1, time.Now().Add(-time.Second), func(stepResult) {})
	if len(res) != 2 {
		t.Fatalf("every task must be accounted for, got %d", len(res))
	}
	for _, r := range res {
		if r.Outcome != stepSkipped {
			t.Fatalf("%+v", r)
		}
	}
}

func TestRunTasksRunsInParallel(t *testing.T) {
	// Four 120ms tasks on four workers should take well under the 480ms they
	// would take serially.
	tasks := make([]task, 4)
	for i := range tasks {
		tasks[i] = task{Name: "sleep", Budget: time.Second, Run: func() (string, error) {
			time.Sleep(120 * time.Millisecond)
			return "", nil
		}}
	}
	start := time.Now()
	runTasks(tasks, 4, time.Now().Add(5*time.Second), func(stepResult) {})
	if elapsed := time.Since(start); elapsed > 400*time.Millisecond {
		t.Fatalf("four parallel 120ms tasks took %s", elapsed)
	}
}

func TestTaskBudgetIsClampedToTheDeadline(t *testing.T) {
	start := time.Now()
	runTasks([]task{
		{Name: "hangs", Budget: time.Hour, Run: func() (string, error) { select {} }},
	}, 1, time.Now().Add(100*time.Millisecond), func(stepResult) {})
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("a one-hour budget must be cut to the deadline, took %s", elapsed)
	}
}

func TestPlanBudgetAlwaysLeavesRoomForTheSummary(t *testing.T) {
	for _, total := range []time.Duration{30 * time.Second, 5 * time.Minute, 20 * time.Minute} {
		p := planBudget(total)
		sum := p.Smoke + p.Feasibility + p.Timing + p.Reserve
		if sum > total {
			t.Fatalf("total %s over-allocated: %+v", total, p)
		}
		if p.Reserve < 3*time.Second {
			t.Fatalf("total %s left no reserve: %+v", total, p)
		}
		if p.Feasibility <= 0 || p.Timing <= 0 {
			t.Fatalf("total %s starved a phase: %+v", total, p)
		}
	}
}
