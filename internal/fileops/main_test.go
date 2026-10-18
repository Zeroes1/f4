package fileops

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/unxed/f4/internal/testutil"
	"go.uber.org/goleak"
)

// Enqueue reports "operation went to the background" from a goroutine that
// sleeps half a second first, and the toast it shows reads vtui.FrameManager.
// A test that enqueues, finishes inside that half second and then swaps the
// frame manager in its cleanup races that read — which is what QueueShowToast
// is a seam for. The application supplies the real toast; here it says nothing.
func TestMain(m *testing.M) {
	QueueShowToast = func(string, time.Duration) {}
	result := m.Run()
	// Same opt-in goroutine leak check as testutil.Main; this package does not
	// go through it because it needs no screen or profile directory.
	if os.Getenv("F4_GOLEAK") != "" {
		if err := testutil.FindGoroutineLeaks(5*time.Second, goleak.IgnoreCurrent()); err != nil {
			fmt.Fprintf(os.Stderr, "goroutine leak after test teardown (F4_GOLEAK): %v\n", err)
			result = 1
		}
	}
	os.Exit(result)
}
