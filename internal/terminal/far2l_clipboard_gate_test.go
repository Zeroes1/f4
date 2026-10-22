package terminal

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

// The far2l clipboard of the embedded terminal is the user's clipboard. A
// program in the terminal may use it only for as long as the user has let its
// client in with CLIP_OPEN and until CLIP_CLOSE: a bare CLIP_GETDATA, a
// CLIP_GETDATA after CLIP_CLOSE or after a refusal must not give the text away,
// and a bare CLIP_SETDATA or CLIP_EMPTY must not change it (unxed/f4#2158,
// T-10 and T-05 (d) of docs/FAR2L_COMPAT.md).

const gateSecret = "secret-clip"

type gateEnv struct {
	tv    *TerminalView
	pty   *mockPty
	clip  string // what the program wrote last, "" if nothing
	wrote int
}

func newGateEnv(t *testing.T, auth int) *gateEnv {
	t.Helper()
	e := &gateEnv{tv: NewTerminalView(80, 24), pty: &mockPty{}}
	t.Cleanup(e.tv.Close)
	e.tv.Pty = e.pty
	e.tv.ClipboardReader = func() string { return gateSecret }
	e.tv.ClipboardWriter = func(s string) { e.clip = s; e.wrote++ }
	prev := vtui.GlobalClipboardAccessManager
	vtui.GlobalClipboardAccessManager = &mockAuth{val: auth}
	t.Cleanup(func() { vtui.GlobalClipboardAccessManager = prev })
	return e
}

// call sends one clipboard request and returns the reply stack, with the id
// popped (nil for requests that have no reply).
func (e *gateEnv) call(t *testing.T, sub byte, push func(*vtinput.Far2lStack)) *vtinput.Far2lStack {
	t.Helper()
	e.pty.mu.Lock()
	e.pty.written = nil
	e.pty.mu.Unlock()
	stk := vtinput.Far2lStack{}
	if push != nil {
		push(&stk)
	}
	stk.PushU8(sub)
	stk.PushU8('c')
	stk.PushU8(7)
	e.tv.ProcessFar2lInteract(stk)
	out := e.pty.String()
	if out == "" {
		return nil
	}
	if !strings.HasPrefix(out, "\x1b_far2l") || !strings.HasSuffix(out, "\x07") {
		t.Fatalf("bad reply %q", out)
	}
	raw, err := base64.StdEncoding.DecodeString(out[len("\x1b_far2l") : len(out)-1])
	if err != nil {
		t.Fatalf("reply is not base64: %q", out)
	}
	r := vtinput.Far2lStack(raw)
	_ = r.PopU8()
	return &r
}

func (e *gateEnv) open(t *testing.T) uint8 {
	t.Helper()
	r := e.call(t, 'o', func(s *vtinput.Far2lStack) { s.PushString("gate-client-id-32-chars-minimum-x") })
	st := r.PopU8() // the status is on top, the features are under it
	_ = r.PopU64()
	return st
}

func (e *gateEnv) get(t *testing.T) (string, bool) {
	t.Helper()
	r := e.call(t, 'g', func(s *vtinput.Far2lStack) { s.PushU32(1) })
	if r == nil {
		t.Fatal("no reply to CLIP_GETDATA")
	}
	raw := string(*r)
	return raw, strings.Contains(raw, gateSecret)
}

func TestFar2lClipboardGetWithoutOpenIsRefused(t *testing.T) {
	e := newGateEnv(t, 1)
	if raw, leaked := e.get(t); leaked {
		t.Fatalf("CLIP_GETDATA without CLIP_OPEN gave the clipboard away: %q", raw)
	}
}

func TestFar2lClipboardGetAfterRefusalIsRefused(t *testing.T) {
	e := newGateEnv(t, 0)
	if st := e.open(t); st != 0 {
		t.Fatalf("CLIP_OPEN of a refused client: status %d, want 0", st)
	}
	if raw, leaked := e.get(t); leaked {
		t.Fatalf("CLIP_GETDATA after a refused CLIP_OPEN gave the clipboard away: %q", raw)
	}
}

func TestFar2lClipboardGetWithoutAccessManagerIsRefused(t *testing.T) {
	e := newGateEnv(t, 1)
	vtui.GlobalClipboardAccessManager = nil
	if st := e.open(t); st != 0 {
		t.Fatalf("CLIP_OPEN with nobody to ask: status %d, want 0", st)
	}
	if raw, leaked := e.get(t); leaked {
		t.Fatalf("CLIP_GETDATA with nobody to ask gave the clipboard away: %q", raw)
	}
}

func TestFar2lClipboardGetAfterOpenWorks(t *testing.T) {
	e := newGateEnv(t, 1)
	if st := e.open(t); st != 1 {
		t.Fatalf("CLIP_OPEN of an allowed client: status %d, want 1", st)
	}
	if raw, leaked := e.get(t); !leaked {
		t.Fatalf("CLIP_GETDATA after CLIP_OPEN lost the text: %q", raw)
	}
}

func TestFar2lClipboardGetAfterLocalAnswerWorks(t *testing.T) {
	e := newGateEnv(t, -1) // "Use Local": the host answers for the program
	if st := e.open(t); st != 1 {
		t.Fatalf("CLIP_OPEN answered locally: status %d, want 1", st)
	}
	if _, leaked := e.get(t); !leaked {
		t.Fatal("CLIP_GETDATA after a local CLIP_OPEN lost the text")
	}
}

func TestFar2lClipboardGetAfterCloseIsRefused(t *testing.T) {
	e := newGateEnv(t, 1)
	e.open(t)
	e.call(t, 'c', nil)
	if raw, leaked := e.get(t); leaked {
		t.Fatalf("CLIP_GETDATA after CLIP_CLOSE gave the clipboard away: %q", raw)
	}
	// a new CLIP_OPEN opens it again (the client is allowed already)
	if st := e.open(t); st != 1 {
		t.Fatalf("second CLIP_OPEN: status %d, want 1", st)
	}
	if _, leaked := e.get(t); !leaked {
		t.Fatal("CLIP_GETDATA after the second CLIP_OPEN lost the text")
	}
}

func TestFar2lClipboardWritesWithoutOpenAreRefused(t *testing.T) {
	e := newGateEnv(t, 1)
	// CLIP_SETDATA
	e.call(t, 's', func(s *vtinput.Far2lStack) {
		s.PushBytes([]byte("planted"))
		s.PushU32(7)
		s.PushU32(1)
	})
	// CLIP_SETDATACHUNK then CLIP_SETDATA
	e.call(t, 'S', func(s *vtinput.Far2lStack) {
		s.PushBytes(make([]byte, 256))
		s.PushU16(1)
	})
	e.call(t, 's', func(s *vtinput.Far2lStack) {
		s.PushBytes([]byte("x"))
		s.PushU32(1)
		s.PushU32(1)
	})
	// CLIP_EMPTY
	e.call(t, 'e', nil)
	if e.wrote != 0 {
		t.Fatalf("the clipboard was written %d times (last %q) by a client that never opened it", e.wrote, e.clip)
	}
	e.tv.mu.Lock()
	chunks := len(e.tv.clipboardChunks)
	e.tv.mu.Unlock()
	if chunks != 0 {
		t.Fatalf("%d bytes of a chunked upload are kept for a client that never opened the clipboard", chunks)
	}
}

func TestFar2lClipboardWritesAfterCloseAreRefused(t *testing.T) {
	e := newGateEnv(t, 1)
	e.open(t)
	e.call(t, 'c', nil)
	e.call(t, 's', func(s *vtinput.Far2lStack) {
		s.PushBytes([]byte("late"))
		s.PushU32(4)
		s.PushU32(1)
	})
	e.call(t, 'e', nil)
	if e.wrote != 0 {
		t.Fatalf("the clipboard was written after CLIP_CLOSE: %q", e.clip)
	}
}

func TestFar2lClipboardIsAvailAndDataIdWithoutOpenGiveNothing(t *testing.T) {
	e := newGateEnv(t, 1)
	if r := e.call(t, 'a', func(s *vtinput.Far2lStack) { s.PushU32(1) }); r != nil && r.PopU8() != 0 {
		t.Error("CLIP_ISAVAILABLE without CLIP_OPEN says the format is there")
	}
	if r := e.call(t, 'i', func(s *vtinput.Far2lStack) { s.PushU32(1) }); r != nil && r.PopU64() != 0 {
		t.Error("CLIP_GETDATAID without CLIP_OPEN gives an id")
	}
}

func TestFar2lClipboardWritesAfterOpenWork(t *testing.T) {
	e := newGateEnv(t, 1)
	e.open(t)
	e.call(t, 's', func(s *vtinput.Far2lStack) {
		s.PushBytes([]byte("hello"))
		s.PushU32(5)
		s.PushU32(1)
	})
	if e.clip != "hello" {
		t.Fatalf("CLIP_SETDATA after CLIP_OPEN: clipboard %q, want hello", e.clip)
	}
}

// openFar2lClipboardForTest puts the terminal into the state CLIP_OPEN of an
// allowed client leaves it in, for tests of the clipboard data itself.
func openFar2lClipboardForTest(tv *TerminalView) {
	tv.mu.Lock()
	tv.clipOpen = true
	tv.mu.Unlock()
}
