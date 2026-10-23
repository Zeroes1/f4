package testutil

import (
	"testing"
	"time"

	"github.com/unxed/vtui"
)

// The harness's own teardown test: SwapFrameManager's restore closes the
// fresh manager, and a manager that leaks its task pump would leave every
// later test running against a queue nobody drains.
func TestFrameManagerShutdownStopsTaskPumpAndUnblocksPostTask(t *testing.T) {
	before, _, err := TaskPumpGoroutineProfile()
	if err != nil {
		t.Fatalf("capture initial goroutine profile: %v", err)
	}

	manager := vtui.NewFrameManager()
	manager.Init(vtui.NewSilentScreenBuf())
	t.Cleanup(manager.Shutdown)
	taskChan := manager.TaskChan
	if taskChan == nil {
		t.Fatal("Init did not create TaskChan")
	}
	started, profile, err := TaskPumpGoroutineProfile()
	if err != nil {
		t.Fatalf("capture running goroutine profile: %v", err)
	}
	if started <= before {
		t.Fatalf("Init did not start a visible task-pump goroutine: before=%d after=%d\n%s", before, started, profile)
	}

	manager.PostTask(func() {})

	start := make(chan struct{})
	postReturned := make(chan struct{})
	shutdownReturned := make(chan struct{})
	go func() {
		<-start
		manager.PostTask(func() {})
		close(postReturned)
	}()
	go func() {
		<-start
		manager.Shutdown()
		close(shutdownReturned)
	}()
	close(start)

	for _, operation := range []struct {
		name string
		done <-chan struct{}
	}{
		{name: "PostTask", done: postReturned},
		{name: "Shutdown", done: shutdownReturned},
	} {
		select {
		case <-operation.done:
		case <-time.After(time.Second):
			t.Fatalf("concurrent %s did not return after manager shutdown", operation.name)
		}
	}

	if manager.TaskChan != nil {
		t.Fatal("Shutdown left TaskChan attached to the manager")
	}
	select {
	case <-taskChan:
		t.Fatal("the stopped task pump delivered a queued task")
	default:
	}

	after, profile, err := WaitForTaskPumpExit(before, time.Second)
	if err != nil {
		t.Fatalf("capture final goroutine profile: %v", err)
	}
	if after > before {
		t.Fatalf("FrameManager task pump did not exit: before=%d after=%d\n%s", before, after, profile)
	}
}

// A test that ends its session drains the UI tasks afterwards, and with no UI
// loop running, ending the session stops the task pump and drops whatever was
// queued -- the drain's own marker task included. DrainUITasks then waited out
// its 30-second deadline: two tests in internal/app did, twice more each
// under the race shard's -cpu 1,4, two minutes on a pull request's critical
// path.
func TestDrainUITasksReturnsOnceTheManagerShutsDown(t *testing.T) {
	t.Cleanup(SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager.PostTask(vtui.FrameManager.Shutdown)

	drained := make(chan struct{})
	go func() {
		DrainUITasks()
		close(drained)
	}()
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("DrainUITasks went on waiting after the frame manager shut down")
	}
}

// A panels-frame test throws away the UI work it leaves queued, so that none
// of it runs in the next test. None of it may run on the way out either, and
// none of it may still be in the pump when the discard returns.
func TestDiscardUITasksEmptiesTheQueueWithoutRunningIt(t *testing.T) {
	t.Cleanup(SwapFrameManager(t))
	vtui.FrameManager.Init(vtui.NewSilentScreenBuf())
	ran := 0
	for i := 0; i < 3; i++ {
		vtui.FrameManager.PostTask(func() { ran++ })
	}

	DiscardUITasks()
	if ran != 0 {
		t.Fatalf("DiscardUITasks ran %d of the queued tasks", ran)
	}
	next := false
	vtui.FrameManager.PostTask(func() { next = true })
	select {
	case task := <-vtui.FrameManager.TaskChan:
		task()
	case <-time.After(5 * time.Second):
		t.Fatal("a task posted after DiscardUITasks never came out of the queue")
	}
	if ran != 0 || !next {
		t.Fatalf("a task posted before DiscardUITasks was still queued after it (ran %d)", ran)
	}
}

// Main's leak check used to judge the task pumps from one profile taken right
// after Shutdown, and failed a -race run over a pump whose deferred
// WaitGroup.Done had already let Shutdown return but which was still inside
// runtime.deferreturn when the profile was taken. A pump that is leaving must
// be waited out.
func TestWaitForTaskPumpExitWaitsOutALeavingPump(t *testing.T) {
	counts := []int{1, 1, 0}
	calls := 0
	sample := func() (int, string, error) {
		count := counts[calls]
		calls++
		return count, "", nil
	}

	count, _, err := waitForTaskPumpExit(sample, 0, time.Minute)
	if err != nil || count != 0 || calls != len(counts) {
		t.Fatalf("waitForTaskPumpExit = count %d, err %v after %d samples; want count 0 after %d samples",
			count, err, calls, len(counts))
	}
}

// Waiting must not hide a pump that never leaves: when the time is up the
// count and profile of that pump are what the check reports.
func TestWaitForTaskPumpExitReportsAPumpThatStays(t *testing.T) {
	sample := func() (int, string, error) { return 1, "stuck pump", nil }

	count, profile, err := waitForTaskPumpExit(sample, 0, 10*time.Millisecond)
	if err != nil || count != 1 || profile != "stuck pump" {
		t.Fatalf("waitForTaskPumpExit = count %d, profile %q, err %v; want the stuck pump reported", count, profile, err)
	}
}
