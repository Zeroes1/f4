package plughost

import (
	"errors"
	"testing"
)

type guardTestTransport struct{ during func() }

func (t *guardTestTransport) Call(method string, params any, result any) error {
	if t.during != nil {
		t.during()
	}
	return nil
}

// A call made from the UI goroutine marks the plugin as blocking it until the
// call returns; a call from any other goroutine does not.
func TestUIGuardTracksCallsFromTheUIGoroutine(t *testing.T) {
	old := uiGoroutine.Load()
	t.Cleanup(func() { uiGoroutine.Store(old) })

	var inside bool
	tr := &guardTestTransport{}
	g := &uiGuard{inner: tr}
	tr.during = func() { inside = g.uiBlocked() }

	uiGoroutine.Store(0) // not learned: nothing is known to be the UI goroutine
	if err := g.Call("VFS.ReadDir", nil, nil); err != nil || inside {
		t.Fatalf("call with an unknown UI goroutine: err=%v blocked=%v", err, inside)
	}

	uiGoroutine.Store(currentGoroutineID())
	if err := g.Call("VFS.ReadDir", nil, nil); err != nil || !inside {
		t.Fatalf("call from the UI goroutine: err=%v blocked=%v, want blocked", err, inside)
	}
	if g.uiBlocked() {
		t.Fatal("still blocked after the call returned")
	}

	uiGoroutine.Store(currentGoroutineID() + 1) // some other goroutine is the UI one
	if err := g.Call("VFS.ReadDir", nil, nil); err != nil || inside {
		t.Fatalf("call from a background goroutine: err=%v blocked=%v", err, inside)
	}
}

// Host.InputBox and Host.Menu refuse instead of deadlocking while the UI
// goroutine waits for the plugin.
func TestHostDialogsRefusedWhileUIWaitsForPlugin(t *testing.T) {
	old := uiGoroutine.Load()
	t.Cleanup(func() { uiGoroutine.Store(old) })
	uiGoroutine.Store(currentGoroutineID())

	tr := &guardTestTransport{}
	g := &uiGuard{inner: tr}
	methods := newHostMethods(newLuaTestHostAPI(), g, "test", nil)

	tr.during = func() {
		for _, m := range []string{"Host.InputBox", "Host.Menu"} {
			if _, err := methods[m](nil); !errors.Is(err, errUIBlocked) {
				t.Errorf("%s while the UI goroutine waits: err = %v, want errUIBlocked", m, err)
			}
		}
	}
	if err := g.Call("VFS.ReadDir", nil, nil); err != nil {
		t.Fatal(err)
	}
	if uiBlockedFor(&guardTestTransport{}) {
		t.Error("a bare transport reported as blocking the UI")
	}
}
