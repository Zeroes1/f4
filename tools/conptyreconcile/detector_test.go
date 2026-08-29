package main

import "testing"

// A process list the test controls, which is the point of the interface: the
// decision logic is exercised here, on any machine, rather than on one that
// happens to have vim installed.
type fakeLister struct {
	procs []ProcessInfo
	err   error
	calls int
}

func (f *fakeLister) List() ([]ProcessInfo, error) {
	f.calls++
	return f.procs, f.err
}

func TestWatcherFindsAProgramStartedByTheShell(t *testing.T) {
	// f4 spawns cmd.exe; the user types vim into it. The program f4 must
	// react to is a grandchild, not a child, which is why the walk exists.
	l := &fakeLister{procs: []ProcessInfo{
		{PID: 100, Parent: 1, Name: "f4.exe"},
		{PID: 200, Parent: 100, Name: "cmd.exe"},
		{PID: 300, Parent: 200, Name: "vim.exe"},
	}}
	w := NewProcessWatcher(l, 100, []string{"vim.exe"})

	on, name := w.Poll()
	if !on || name != "vim.exe" {
		t.Fatalf("expected vim to be found, got %v %q", on, name)
	}

	l.procs = l.procs[:2] // vim exits
	if on, _ := w.Poll(); on {
		t.Fatal("the geometry must go back when the program exits")
	}
}

func TestWatcherIgnoresProgramsOutsideTheTree(t *testing.T) {
	l := &fakeLister{procs: []ProcessInfo{
		{PID: 100, Parent: 1, Name: "f4.exe"},
		{PID: 900, Parent: 1, Name: "vim.exe"}, // someone else's vim
	}}
	w := NewProcessWatcher(l, 100, []string{"vim.exe"})
	if on, _ := w.Poll(); on {
		t.Fatal("a vim started elsewhere must not resize our console")
	}
}

func TestWatcherWaitsForTheLastOneToExit(t *testing.T) {
	l := &fakeLister{procs: []ProcessInfo{
		{PID: 100, Parent: 1, Name: "f4.exe"},
		{PID: 200, Parent: 100, Name: "cmd.exe"},
		{PID: 300, Parent: 200, Name: "vim.exe"},
		{PID: 301, Parent: 300, Name: "less.exe"},
	}}
	w := NewProcessWatcher(l, 100, []string{"vim.exe", "less.exe"})
	if on, _ := w.Poll(); !on {
		t.Fatal("two watched programs should be active")
	}
	l.procs = l.procs[:3] // less exits, vim stays
	if on, _ := w.Poll(); !on {
		t.Fatal("the geometry must not flip while one is still running")
	}
	l.procs = l.procs[:2]
	if on, _ := w.Poll(); on {
		t.Fatal("now it must go back")
	}
}

func TestWatcherKeepsItsAnswerWhenEnumerationFails(t *testing.T) {
	l := &fakeLister{procs: []ProcessInfo{
		{PID: 100, Parent: 1, Name: "f4.exe"},
		{PID: 200, Parent: 100, Name: "vim.exe"},
	}}
	w := NewProcessWatcher(l, 100, []string{"vim.exe"})
	if on, _ := w.Poll(); !on {
		t.Fatal("setup")
	}
	l.err = errFake{}
	if on, _ := w.Poll(); !on {
		t.Fatal("a failed enumeration must not flip the geometry; a spurious switch redraws for nothing")
	}
}

type errFake struct{}

func (errFake) Error() string { return "no snapshot" }

func TestWatcherSurvivesACycleInParentLinks(t *testing.T) {
	// Reported parents can be nonsense after a pid is reused; the walk must
	// terminate regardless.
	l := &fakeLister{procs: []ProcessInfo{
		{PID: 100, Parent: 200, Name: "a.exe"},
		{PID: 200, Parent: 100, Name: "b.exe"},
	}}
	w := NewProcessWatcher(l, 100, []string{"b.exe"})
	done := make(chan struct{})
	go func() { w.Poll(); close(done) }()
	<-done
}

func TestWatcherMatchesNamesCaseInsensitively(t *testing.T) {
	l := &fakeLister{procs: []ProcessInfo{
		{PID: 100, Parent: 1, Name: "f4.exe"},
		{PID: 200, Parent: 100, Name: "VIM.EXE"},
	}}
	w := NewProcessWatcher(l, 100, []string{"vim.exe"})
	if on, _ := w.Poll(); !on {
		t.Fatal("Windows image names are not case sensitive")
	}
}

func TestDetectorPrefersTheDeterministicLayer(t *testing.T) {
	l := &fakeLister{procs: []ProcessInfo{
		{PID: 100, Parent: 1, Name: "f4.exe"},
		{PID: 200, Parent: 100, Name: "vim.exe"},
	}}
	d := &Detector{Process: NewProcessWatcher(l, 100, []string{"vim.exe"})}
	d.Alt.Feed([]byte("\x1b[?1049h"))

	on, why := d.Decide()
	if !on || why != "process: vim.exe" {
		t.Fatalf("the process layer must answer first: %v %q", on, why)
	}
}

func TestDetectorFallsBackToTheAlternateScreen(t *testing.T) {
	// The 24H2 case: no watched process, but the emitter passes 1049 through.
	d := &Detector{}
	d.Alt.Feed([]byte("\x1b[?1049h"))
	if on, why := d.Decide(); !on || why != "alternate screen" {
		t.Fatalf("got %v %q", on, why)
	}
}

func TestUserOverrideBeatsEverything(t *testing.T) {
	l := &fakeLister{procs: []ProcessInfo{
		{PID: 100, Parent: 1, Name: "f4.exe"},
		{PID: 200, Parent: 100, Name: "vim.exe"},
	}}
	d := &Detector{Process: NewProcessWatcher(l, 100, []string{"vim.exe"})}
	d.Forced, d.ForcedSet = false, true
	if on, why := d.Decide(); on || why != "forced by the user" {
		t.Fatalf("the user must be able to overrule the detector: %v %q", on, why)
	}
}

func TestFrameSignalsAreLabelledAsHeuristics(t *testing.T) {
	f := &FrameSignals{Height: 100}

	// A quiet frame: a few content rows in a tall buffer.
	quiet := []byte("one\x1b[K\r\ntwo\x1b[K\r\n" + repeatStr("\x1b[K\r\n", 90))
	if on, _ := f.Observe(quiet); on {
		t.Fatal("ordinary output must not look full-screen")
	}

	// A frame whose content fills the buffer.
	var full []byte
	for i := 0; i < 95; i++ {
		full = append(full, []byte("row\x1b[K\r\n")...)
	}
	if on, why := f.Observe(full); !on || why == "" {
		t.Fatalf("a frame filling the viewport should fire: %v %q", on, why)
	}
}

func repeatStr(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}

// FuzzDetectorLayersNeverPanic: the detector runs on every chunk and every
// frame, so it is on the hottest path in the terminal.
func FuzzDetectorLayersNeverPanic(f *testing.F) {
	f.Add([]byte("\x1b[?1049h"), []byte("x\x1b[K\r\n"))
	f.Add([]byte{}, []byte{})
	f.Add([]byte{0x1b}, []byte{0x1b, '['})

	f.Fuzz(func(t *testing.T, chunk, frame []byte) {
		d := &Detector{Signals: &FrameSignals{Height: 50}}
		d.Alt.Feed(chunk)
		d.Signals.Observe(frame)
		d.Decide()
	})
}
