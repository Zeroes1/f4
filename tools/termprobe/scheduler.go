package main

import (
	"fmt"
	"sync"
	"time"
)

// Every step of this probe can hang. CreatePseudoConsole can hang at a large
// height, ClosePseudoConsole is documented to block until the host exits (and
// did, in the field, for a whole minute), ReadFile on a pipe nobody writes to
// blocks forever, and a child that never starts makes every wait run to its
// timeout. The first four field runs each died on a different one of these,
// and each time the probe stopped rather than continuing without that number.
//
// So hanging is treated as an ordinary outcome here, not an accident: every
// step runs under a watchdog, a step that does not return is abandoned and
// reported as hung, independent steps run in parallel so that one of them
// stalling costs only itself, and results are emitted the moment they exist.

type stepOutcome int

const (
	stepOK stepOutcome = iota
	stepFailed
	stepHung
	stepSkipped
)

func (o stepOutcome) String() string {
	switch o {
	case stepOK:
		return "ok"
	case stepFailed:
		return "FAILED"
	case stepHung:
		return "HUNG"
	default:
		return "skipped"
	}
}

type stepResult struct {
	Name    string
	Outcome stepOutcome
	Err     string
	Summary string
	Dur     time.Duration
}

// withTimeout runs fn on its own goroutine and returns as soon as fn finishes
// or d elapses, whichever comes first. A step that times out is abandoned:
// its goroutine keeps running and is never waited for, because waiting is
// exactly what must not happen. A panic inside fn is caught and reported as a
// failure rather than taking the process down with it.
func withTimeout(name string, d time.Duration, fn func() (string, error)) stepResult {
	start := time.Now()
	type outcome struct {
		summary string
		err     error
	}
	done := make(chan outcome, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- outcome{err: fmt.Errorf("panic: %v", r)}
			}
		}()
		s, err := fn()
		done <- outcome{summary: s, err: err}
	}()

	select {
	case o := <-done:
		res := stepResult{Name: name, Dur: time.Since(start), Summary: o.summary}
		if o.err != nil {
			res.Outcome = stepFailed
			res.Err = o.err.Error()
		} else {
			res.Outcome = stepOK
		}
		return res
	case <-time.After(d):
		return stepResult{
			Name:    name,
			Outcome: stepHung,
			Dur:     time.Since(start),
			Err:     fmt.Sprintf("no return within %s; abandoned", d),
		}
	}
}

// task is a unit of measurement that can be run on its own.
type task struct {
	Name   string
	Budget time.Duration
	Run    func() (string, error)
}

// runTasks executes tasks across `workers` goroutines, emitting each result as
// soon as it is known. Tasks started after the deadline are reported as
// skipped rather than silently dropped, so the log accounts for every one.
//
// Independent measurements run in parallel deliberately: nine heights that do
// not depend on each other should not be serialised behind whichever of them
// stalls. Timings taken this way are upper bounds, and the caller says so.
func runTasks(tasks []task, workers int, deadline time.Time, emit func(stepResult)) []stepResult {
	if workers < 1 {
		workers = 1
	}
	in := make(chan task)
	var mu sync.Mutex
	results := make([]stepResult, 0, len(tasks))

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range in {
				var res stepResult
				if remaining := time.Until(deadline); remaining <= 0 {
					res = stepResult{Name: t.Name, Outcome: stepSkipped, Err: "deadline reached"}
				} else {
					budget := t.Budget
					if budget > remaining {
						budget = remaining
					}
					res = withTimeout(t.Name, budget, t.Run)
				}
				mu.Lock()
				results = append(results, res)
				mu.Unlock()
				emit(res)
			}
		}()
	}

	for _, t := range tasks {
		in <- t
	}
	close(in)
	wg.Wait()

	return results
}

// budgetPlan splits a total run budget between the phases, so that the
// feasibility pass always completes and the precise-timing pass takes
// whatever is left rather than the other way round.
type budgetPlan struct {
	Smoke       time.Duration
	Feasibility time.Duration
	Timing      time.Duration
	Reserve     time.Duration
}

func planBudget(total time.Duration) budgetPlan {
	// The summary must always be printed, so a slice is held back for it.
	reserve := total / 20
	if reserve < 3*time.Second {
		reserve = 3 * time.Second
	}
	usable := total - reserve
	if usable < 0 {
		usable = 0
	}
	smoke := usable / 10
	if smoke > 20*time.Second {
		smoke = 20 * time.Second
	}
	rest := usable - smoke
	return budgetPlan{
		Smoke:       smoke,
		Feasibility: rest * 55 / 100,
		Timing:      rest * 45 / 100,
		Reserve:     reserve,
	}
}
