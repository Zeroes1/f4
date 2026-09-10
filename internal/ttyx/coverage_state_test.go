package ttyx

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestSourceStrings(t *testing.T) {
	cases := []struct {
		name   string
		source Source
		want   string
	}{
		{name: "none", source: SourceNone, want: "none"},
		{name: "window id", source: SourceWindowID, want: "WINDOWID"},
		{name: "process", source: SourceProcess, want: "_NET_WM_PID"},
		{name: "active", source: SourceActive, want: "_NET_ACTIVE_WINDOW"},
		{name: "unknown", source: Source(99), want: "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.source.String(); got != tc.want {
				t.Fatalf("Source(%d).String() = %q, want %q", tc.source, got, tc.want)
			}
		})
	}
}

func TestContainsPID(t *testing.T) {
	if !containsPID([]uint32{3, 7, 11}, 7) {
		t.Fatal("containsPID missed a present pid")
	}
	if containsPID([]uint32{3, 7, 11}, 8) {
		t.Fatal("containsPID found an absent pid")
	}
}

func TestAncestorPIDsStopsAtInputBoundaries(t *testing.T) {
	parent := func(pid int) (int, bool) { return pid + 1, true }

	if got := ancestorPIDs(0, parent); len(got) != 0 {
		t.Fatalf("zero pid produced %v", got)
	}
	if got := ancestorPIDs(-1, parent); len(got) != 0 {
		t.Fatalf("negative pid produced %v", got)
	}
	if got := ancestorPIDs(1, parent); len(got) != 32 {
		t.Fatalf("ancestor walk exceeded its bound: got %d entries", len(got))
	}

	if got := ancestorPIDs(1, func(int) (int, bool) { return 0, false }); len(got) != 1 {
		t.Fatalf("a missing parent should end the walk: %v", got)
	}

	if int64(^uint32(0))+1 <= int64(^uint32(0)) {
		t.Fatal("test bound is not above uint32")
	}
	if got := ancestorPIDs(int(int64(^uint32(0))+1), parent); len(got) != 0 {
		t.Fatalf("an out-of-range pid produced %v", got)
	}
}

func TestParentPIDForMissingProcess(t *testing.T) {
	if _, ok := parentPID(-1); ok {
		t.Fatal("a missing process must not have a parent")
	}
}

func TestSessionStateWithoutDisplay(t *testing.T) {
	s := &Session{alive: true, focused: true, changed: make(chan struct{}, 1)}
	if !s.Focused() {
		t.Fatal("Focused should report cached focus while the session is alive")
	}
	if s.Alive() {
		t.Fatal("Alive must require an X connection")
	}

	s.alive = false
	if s.Focused() {
		t.Fatal("a dead session must not report focus")
	}
}

func TestUngrabKeysClearsConfiguredSetWithoutDisplay(t *testing.T) {
	s := &Session{keys: &keyState{
		combos: []Combo{{Keysym: 1}},
		codes:  []xproto.Keycode{2},
	}}
	s.UngrabKeys()
	if len(s.keys.combos) != 0 || len(s.keys.codes) != 0 {
		t.Fatalf("UngrabKeys left state behind: combos=%v codes=%v", s.keys.combos, s.keys.codes)
	}
}

func TestOnKeyWithoutTranslatorDoesNothing(t *testing.T) {
	s := &Session{keys: &keyState{events: nil}}
	s.onKey(24, 0, true)
}

func TestDispatchIgnoresUnrelatedEvents(t *testing.T) {
	s := &Session{win: 7, alive: true, changed: make(chan struct{}, 1)}

	s.dispatch(xproto.FocusInEvent{Mode: xproto.NotifyModeGrab})
	s.dispatch(xproto.FocusInEvent{Detail: xproto.NotifyDetailInferior})
	if s.focused {
		t.Fatal("non-focus focus events changed the state")
	}

	s.dispatch(xproto.ConfigureNotifyEvent{Window: 8})
	s.dispatch(xproto.UnmapNotifyEvent{Window: 8})
	s.dispatch(xproto.DestroyNotifyEvent{Window: 8})
	s.dispatch(xproto.KeyPressEvent{Detail: 24})
	s.dispatch(xproto.KeyReleaseEvent{Detail: 24})
	if !s.alive {
		t.Fatal("events for another window changed the session")
	}
}

func TestFollowedParentLeavesEmptyOverlayUnchanged(t *testing.T) {
	s := &Session{}
	o := &Overlay{s: s}
	o.followedParent(20, 30)
	if got := o.rect; got != (Rect{}) {
		t.Fatalf("empty overlay moved to %+v", got)
	}
}
