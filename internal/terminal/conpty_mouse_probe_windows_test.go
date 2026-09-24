//go:build windows

package terminal

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/unxed/f4/internal/keymap"
	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
	"golang.org/x/sys/windows"
)

// Issue #1294 investigation probe. It is not a regression test and asserts
// nothing about the outcome: it records what a console program receives
// from a real pseudoconsole when f4 forwards a mouse click to it, and writes
// the record to the log (go test -v) and, in GitHub Actions, to the job
// summary. Remove it once #1294 is settled.
//
// The program in the pseudoconsole is this test binary again
// (TestConPTYMouseProbeChild). It sets the console input mode the way Far
// Manager's SetFarConsoleMode does and writes every INPUT_RECORD it reads to
// a file. f4's own AnsiParser consumes what the pseudoconsole writes, so the
// mouse mode the probe encodes for is the one f4 derives in real use.
//
// Two variants run in separate pseudoconsoles:
//   - "f4": the click encoded by keymap.TranslateMouseInputWithMode, the
//     function PanelsFrame.ProcessMouse uses, from events shaped the way
//     vtinput's Windows console reader delivers them;
//   - "xterm": the same click written by hand in the SGR form that xterm's
//     ctlseqs describe, where the release carries the released button.
const conptyMouseProbeOutEnv = "F4_CONPTY_MOUSE_PROBE_OUT"

var procReadConsoleInputWProbe = windows.NewLazySystemDLL("kernel32.dll").NewProc("ReadConsoleInputW")

// TestConPTYMouseProbeChild is the program TestConPTYMouseReleaseProbe runs
// inside the pseudoconsole. Anywhere else it does nothing.
func TestConPTYMouseProbeChild(t *testing.T) {
	out := os.Getenv(conptyMouseProbeOutEnv)
	if out == "" {
		t.Skip("helper of TestConPTYMouseReleaseProbe; runs only inside its pseudoconsole")
	}
	runConPTYMouseProbeChild(out)
}

func runConPTYMouseProbeChild(outPath string) {
	f, err := os.OpenFile(outPath, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	say := func(format string, a ...any) { _, _ = fmt.Fprintf(f, format+"\n", a...) }

	name, _ := windows.UTF16PtrFromString("CONIN$")
	h, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		say("stopped: open CONIN$: %v", err)
		return
	}
	defer windows.CloseHandle(h)

	var mode uint32
	if err := windows.GetConsoleMode(h, &mode); err != nil {
		say("stopped: GetConsoleMode: %v", err)
		return
	}
	// As Far Manager's SetFarConsoleMode with mouse support on.
	newMode := mode | windows.ENABLE_WINDOW_INPUT | windows.ENABLE_MOUSE_INPUT | windows.ENABLE_EXTENDED_FLAGS
	newMode &^= windows.ENABLE_PROCESSED_INPUT | windows.ENABLE_LINE_INPUT | windows.ENABLE_ECHO_INPUT |
		windows.ENABLE_VIRTUAL_TERMINAL_INPUT | windows.ENABLE_QUICK_EDIT_MODE
	if err := windows.SetConsoleMode(h, newMode); err != nil {
		say("stopped: SetConsoleMode(%#x): %v", newMode, err)
		return
	}
	say("ready: input mode %#x -> %#x", mode, newMode)

	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		ev, err := windows.WaitForSingleObject(h, 15000)
		if err != nil {
			say("stopped: wait: %v", err)
			return
		}
		if ev == uint32(windows.WAIT_TIMEOUT) {
			say("stopped: no input for 15s")
			return
		}
		var rec struct {
			EventType uint16
			_         uint16
			Event     [16]byte
		}
		var n uint32
		r, _, callErr := procReadConsoleInputWProbe.Call(uintptr(h), uintptr(unsafe.Pointer(&rec)), 1, uintptr(unsafe.Pointer(&n)))
		if r == 0 {
			say("stopped: ReadConsoleInputW: %v", callErr)
			return
		}
		if n == 0 {
			continue
		}
		e := rec.Event[:]
		u16 := func(o int) uint16 { return binary.LittleEndian.Uint16(e[o:]) }
		u32 := func(o int) uint32 { return binary.LittleEndian.Uint32(e[o:]) }
		switch rec.EventType {
		case 0x0001: // KEY_EVENT
			down, ch := u32(0) != 0, u16(10)
			say("record KEY   down=%v vk=%#x char=%q ctrl=%#x repeat=%d", down, u16(6), rune(ch), u32(12), u16(4))
			if down && ch == 'q' {
				say("stopped: sentinel key 'q' received")
				return
			}
		case 0x0002: // MOUSE_EVENT
			say("record MOUSE pos=(%d,%d) buttons=%#x%s ctrl=%#x flags=%#x%s",
				int16(u16(0)), int16(u16(2)), u32(4), probeButtonNames(u32(4)), u32(8), u32(12), probeMouseFlagNames(u32(12)))
		case 0x0004: // WINDOW_BUFFER_SIZE_EVENT
			say("record SIZE  %dx%d", int16(u16(0)), int16(u16(2)))
		case 0x0008: // MENU_EVENT
			say("record MENU  %#x", u32(0))
		case 0x0010: // FOCUS_EVENT
			say("record FOCUS set=%v", u32(0) != 0)
		default:
			say("record type=%#x", rec.EventType)
		}
	}
	say("stopped: 60s limit")
}

func probeButtonNames(b uint32) string {
	var names []string
	for _, x := range []struct {
		bit  uint32
		name string
	}{{0x1, "left"}, {0x2, "right"}, {0x4, "middle"}} {
		if b&x.bit != 0 {
			names = append(names, x.name)
		}
	}
	if len(names) == 0 {
		return "(none)"
	}
	return "(" + strings.Join(names, "+") + ")"
}

func probeMouseFlagNames(f uint32) string {
	if f == 0 {
		return "(button event)"
	}
	var names []string
	for _, x := range []struct {
		bit  uint32
		name string
	}{{0x1, "MOUSE_MOVED"}, {0x2, "DOUBLE_CLICK"}, {0x4, "MOUSE_WHEELED"}, {0x8, "MOUSE_HWHEELED"}} {
		if f&x.bit != 0 {
			names = append(names, x.name)
		}
	}
	return "(" + strings.Join(names, "|") + ")"
}

func TestConPTYMouseReleaseProbe(t *testing.T) {
	if vtui.IsWine() {
		t.Skip("Wine has no ConPTY")
	}
	type host struct {
		name string
		api  *conPTYAPI
	}
	var hosts []host
	if ConPTYAvailable() {
		if api, err := systemConPTY(); err == nil {
			hosts = append(hosts, host{"inbox", api})
		}
	}
	if os.Getenv("F4_NATIVE_CONPTY_PACKAGE") == "1" {
		if api := installConPTYPackageForProbe(t); api != nil {
			hosts = append(hosts, host{"package", api})
		}
	}
	if len(hosts) == 0 {
		t.Skip("no ConPTY to probe")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable: %v", err)
	}
	for _, h := range hosts {
		for _, variant := range []string{"f4", "xterm"} {
			t.Run(h.name+"/"+variant, func(t *testing.T) {
				title := fmt.Sprintf("#1294 probe: %s/%s ConPTY %s, %s encoding", runtime.GOOS, runtime.GOARCH, h.api.path, variant)
				report := probeConPTYMouse(t, h.api, exe, variant)
				t.Logf("%s\n%s", title, report)
				appendProbeStepSummary(title, report)
			})
		}
	}
}

// installConPTYPackageForProbe installs the package the way
// TestConPTYPackageKeepsLongLinesWhole does, into a directory of its own.
func installConPTYPackageForProbe(t *testing.T) *conPTYAPI {
	t.Helper()
	pkg, ok := conPTYPackageFor(runtime.GOARCH)
	if !ok {
		t.Logf("no ConPTY package for %s", runtime.GOARCH)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	// Not t.TempDir(): conpty.dll stays mapped in this process until it exits.
	root, err := os.MkdirTemp("", "f4-conpty-mouse-probe-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	dir, err := installConPTYPackage(ctx, http.DefaultClient, root, pkg)
	if err != nil {
		t.Logf("install ConPTY package: %v", err)
		return nil
	}
	api, err := loadConPTYPackage(dir)
	if err != nil {
		t.Logf("load ConPTY package: %v", err)
		return nil
	}
	return api
}

type probeSeq struct{ what, bytes string }

func probeSequences(variant string, sgr bool) []probeSeq {
	if variant == "xterm" {
		return []probeSeq{
			{"press", "\x1b[<0;11;6M"},
			{"release", "\x1b[<0;11;6m"},
			{"hover", "\x1b[<35;13;6M"},
			{"press", "\x1b[<0;13;6M"},
			{"release", "\x1b[<0;13;6m"},
		}
	}
	// Shaped as vtinput's Windows console reader delivers a click: every
	// mouse record has KeyDown set; a release is a record with no buttons.
	events := []struct {
		what string
		e    vtinput.InputEvent
	}{
		{"press", vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 5, ButtonState: vtinput.FromLeft1stButtonPressed}},
		{"release", vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 10, MouseY: 5}},
		{"hover", vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 12, MouseY: 5, MouseEventFlags: vtinput.MouseMoved}},
		{"press", vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 12, MouseY: 5, ButtonState: vtinput.FromLeft1stButtonPressed}},
		{"release", vtinput.InputEvent{Type: vtinput.MouseEventType, KeyDown: true, MouseX: 12, MouseY: 5}},
	}
	out := make([]probeSeq, 0, len(events))
	for i := range events {
		out = append(out, probeSeq{events[i].what, keymap.TranslateMouseInputWithMode(&events[i].e, sgr)})
	}
	return out
}

var probeModeRequest = regexp.MustCompile(`\x1b\[\?[0-9;]*[hl]`)

func probeConPTYMouse(t *testing.T, api *conPTYAPI, exe, variant string) string {
	var rep strings.Builder
	outPath := filepath.Join(t.TempDir(), "records.txt")

	p, err := newPTYWithAPI(api)
	if err != nil {
		t.Fatalf("create pseudoconsole: %v", err)
	}
	defer p.Close()

	var mu sync.Mutex
	var raw bytes.Buffer
	tv := NewTerminalView(80, 24)
	parser := NewAnsiParser(tv, p)
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		buf := make([]byte, 32768)
		for {
			n, err := p.Read(buf)
			if n > 0 {
				mu.Lock()
				raw.Write(buf[:n])
				parser.Process(buf[:n])
				mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	records := func() []byte { b, _ := os.ReadFile(outPath); return b }
	waitFor := func(d time.Duration, cond func() bool) bool {
		for end := time.Now().Add(d); time.Now().Before(end); time.Sleep(50 * time.Millisecond) {
			if cond() {
				return true
			}
		}
		return cond()
	}

	t.Setenv(conptyMouseProbeOutEnv, outPath)
	if err := p.Run(fmt.Sprintf(`"%s" -test.run=^TestConPTYMouseProbeChild$`, exe)); err != nil {
		t.Fatalf("start the probe program: %v", err)
	}
	if !waitFor(30*time.Second, func() bool { return bytes.Contains(records(), []byte("ready:")) }) {
		fmt.Fprintf(&rep, "the probe program never got ready; its file:\n%s", records())
	} else {
		// ConPTY answers SetConsoleMode(ENABLE_MOUSE_INPUT) with a mode
		// request in its output; give it time to arrive.
		waitFor(5*time.Second, func() bool { mu.Lock(); defer mu.Unlock(); return tv.MouseTrackingMode != 0 })
		mu.Lock()
		mode, sgr, w32 := tv.MouseTrackingMode, tv.MouseSGRMode, tv.Win32InputMode
		mu.Unlock()
		fmt.Fprintf(&rep, "f4 view once the program enabled mouse input: MouseTrackingMode=%d MouseSGRMode=%v Win32InputMode=%v\n", mode, sgr, w32)
		if mode == 0 {
			rep.WriteString("(f4 forwards no mouse input in this state; the probe sends it anyway)\n")
		}
		for _, s := range probeSequences(variant, sgr) {
			fmt.Fprintf(&rep, "sent %-7s %q\n", s.what, s.bytes)
			if _, err := p.Write([]byte(s.bytes)); err != nil {
				fmt.Fprintf(&rep, "  write failed: %v\n", err)
			}
			time.Sleep(150 * time.Millisecond)
		}
		rep.WriteString("sent sentinel \"q\"\n")
		_, _ = p.Write([]byte("q"))
		if !waitFor(30*time.Second, func() bool { return bytes.Contains(records(), []byte("stopped:")) }) {
			rep.WriteString("the probe program did not stop within 30s\n")
		}
		fmt.Fprintf(&rep, "the program received:\n%s", records())
	}

	_ = p.Close()
	select {
	case <-readDone:
	case <-time.After(10 * time.Second):
		rep.WriteString("(the pseudoconsole output did not end within 10s of Close)\n")
	}
	mu.Lock()
	fmt.Fprintf(&rep, "private mode requests in the pseudoconsole output: %q\n", probeModeRequest.FindAll(raw.Bytes(), -1))
	mu.Unlock()
	return rep.String()
}

func appendProbeStepSummary(title, body string) {
	path := os.Getenv("GITHUB_STEP_SUMMARY")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "### %s\n\n```\n%s```\n\n", title, body)
}
