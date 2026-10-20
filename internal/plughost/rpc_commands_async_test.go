package plughost

import (
	"errors"
	"testing"
	"time"

	"github.com/unxed/f4/vfs"
	"github.com/unxed/vtui"
)

func TestRPCCommandDoesNotBlockUIForHostPrompts(t *testing.T) {
	old := uiGoroutine.Load()
	uiGoroutine.Store(currentGoroutineID())
	t.Cleanup(func() { uiGoroutine.Store(old) })
	called := make(chan bool, 1)
	done := make(chan struct{})
	transport := &guardTestTransport{}
	guard := &uiGuard{inner: transport}
	transport.during = func() { called <- guard.uiBlocked(); close(done) }
	startRPCCommand(guard, "test", "add", nil)
	select {
	case blocked := <-called:
		if blocked {
			t.Fatal("RPC command blocks UI and prevents host prompts")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("command was not dispatched")
	}
	<-done
}

type failingRPCCommandTransport struct{}

func (failingRPCCommandTransport) Call(string, any, any) error {
	return errors.New("authentication failed")
}

type rpcErrorTestApp struct {
	vfs.App
	messageCalled bool
}

func (app *rpcErrorTestApp) Message(string, string, []string) int {
	app.messageCalled = true
	return -1
}

func TestRPCCommandErrorShowsDialogWithoutSynchronousAppMessage(t *testing.T) {
	oldManager, oldApp := vtui.FrameManager, App
	fm := vtui.NewFrameManager()
	fm.Init(vtui.NewSilentScreenBuf())
	vtui.FrameManager, App = fm, nil
	t.Cleanup(func() { fm.Shutdown(); vtui.FrameManager, App = oldManager, oldApp })
	app := &rpcErrorTestApp{}
	startRPCCommand(failingRPCCommandTransport{}, "CloudFox", "add", app)
	select {
	case task := <-fm.TaskChan:
		task()
	case <-time.After(5 * time.Second):
		t.Fatal("command error did not reach the UI")
	}
	if app.messageCalled {
		t.Fatal("error handler used App.Message, which posts and waits on the same UI thread")
	}
	if _, ok := fm.GetTopFrame().(*vtui.Window); !ok {
		t.Fatalf("error dialog missing: %T", fm.GetTopFrame())
	}
}
