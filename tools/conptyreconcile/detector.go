package main

import (
	"sort"
	"strings"
)

// The full-screen detector, in the four layers §17 settled on, with the
// platform-specific part behind an interface so the decision logic is tested
// here rather than on a machine that happens to have vim installed.
//
// Why it exists at all: f4 tells the child a tall lie about the window height,
// and a program that draws a full-screen interface must be told the truth or
// it puts its bottom border thirty thousand rows above the visible slice. The
// detector is the bill for the lie, and §17 measured that neither mechanism
// proposed there is sufficient alone -- so the layers are explicit, ordered by
// how much they can be trusted, and each says which one answered.

// ProcessLister enumerates process ids and image names. Windows implements it
// with a Toolhelp snapshot; the tests implement it with a slice, which is the
// point of the interface.
type ProcessLister interface {
	List() ([]ProcessInfo, error)
}

type ProcessInfo struct {
	PID    uint32
	Parent uint32
	Name   string
}

// ProcessWatcher is layer 1: the only deterministic layer. f4 spawns the
// shell, not the program the user runs in it, so what it can observe is a new
// descendant appearing. Watching by image name against a configurable list is
// exact -- no inference, no stream parsing -- and it works on the build where
// the alternate screen is invisible.
//
// It is unavailable over plain ssh, which does not matter: §17 shows the tall
// viewport is not used there either, so no lie is told and no detector is
// needed.
type ProcessWatcher struct {
	lister   ProcessLister
	root     uint32
	watching map[string]bool

	// active is the set of matching descendants currently alive, so the
	// geometry goes back only when the last of them exits -- a program that
	// launches another must not flip it twice.
	active map[uint32]string
}

func NewProcessWatcher(lister ProcessLister, root uint32, names []string) *ProcessWatcher {
	w := &ProcessWatcher{
		lister:   lister,
		root:     root,
		watching: make(map[string]bool, len(names)),
		active:   map[uint32]string{},
	}
	for _, n := range names {
		n = strings.ToLower(strings.TrimSpace(n))
		if n != "" {
			w.watching[n] = true
		}
	}
	return w
}

// DefaultFullScreenPrograms is the starting list. It is a setting per §16, not
// a constant: which programs draw full screen is a property of the user's
// machine, and a list baked into the binary would be wrong on the first one
// that is not on it.
func DefaultFullScreenPrograms() []string {
	return []string{
		"vim.exe", "nvim.exe", "nano.exe", "emacs.exe",
		"far.exe", "far64.exe", "f4.exe", "mc.exe",
		"less.exe", "more.com", "htop.exe", "btop.exe",
		"msedit.exe", "edit.com",
	}
}

// Poll re-reads the process list and reports whether any watched program is
// running below the root, and its name.
func (w *ProcessWatcher) Poll() (bool, string) {
	if w == nil || w.lister == nil {
		return false, ""
	}
	all, err := w.lister.List()
	if err != nil {
		// A failed enumeration must not flip the geometry: keeping the last
		// answer is the safe reading, because a spurious switch redraws the
		// screen for no reason.
		return w.state()
	}

	desc := descendantsOf(all, w.root)
	seen := map[uint32]string{}
	for pid, name := range desc {
		if w.watching[strings.ToLower(name)] {
			seen[pid] = name
		}
	}
	w.active = seen
	return w.state()
}

func (w *ProcessWatcher) state() (bool, string) {
	if len(w.active) == 0 {
		return false, ""
	}
	names := make([]string, 0, len(w.active))
	for _, n := range w.active {
		names = append(names, n)
	}
	sort.Strings(names)
	return true, names[0]
}

// descendantsOf walks the parent links, so a program started by the shell that
// f4 started is found -- which is the whole case, since f4 spawns cmd.exe and
// the user types the program name into it.
func descendantsOf(all []ProcessInfo, root uint32) map[uint32]string {
	children := map[uint32][]ProcessInfo{}
	for _, p := range all {
		children[p.Parent] = append(children[p.Parent], p)
	}
	out := map[uint32]string{}
	var walk func(uint32, int)
	walk = func(pid uint32, depth int) {
		if depth > 32 { // a cycle in reported parents must not hang the poll
			return
		}
		for _, c := range children[pid] {
			if _, seen := out[c.PID]; seen {
				continue
			}
			out[c.PID] = c.Name
			walk(c.PID, depth+1)
		}
	}
	walk(root, 0)
	return out
}

// ---------------------------------------------------------------------------
// layer 3: signals in the frame
// ---------------------------------------------------------------------------

// FrameSignals is the heuristic layer, and is labelled as such. §17 measured
// two things worth watching. While the alternate screen was active, a frame
// carried roughly twice the usual terminator count, which looks like both
// buffers being painted. And a full-screen program fills the whole tall
// viewport instead of adding rows at the bottom.
//
// Being wrong here is cheap -- one badly drawn frame, no lost text -- which is
// what makes a heuristic acceptable at this level and nowhere above it.
type FrameSignals struct {
	Height int

	lastTerminators int
}

// Observe reports whether a frame looks like a full-screen program is running.
func (f *FrameSignals) Observe(frame []byte) (bool, string) {
	terms := strings.Count(string(frame), "\x1b[K\r\n")
	prev := f.lastTerminators
	f.lastTerminators = terms

	// Twice the previous count, with a previous count worth comparing to.
	if prev > 16 && terms > prev*3/2 {
		return true, "frame terminator count doubled"
	}

	// Content occupying nearly the whole tall buffer rather than its tail.
	if f.Height > 0 {
		rows := contentRowsInFrame(frame)
		if rows > f.Height*9/10 {
			return true, "content spans the whole viewport"
		}
	}
	return false, ""
}

// contentRowsInFrame counts rows carrying anything other than the erase that
// blank rows are made of.
func contentRowsInFrame(frame []byte) int {
	n := 0
	for _, run := range strings.Split(string(frame), "\x1b[K\r\n") {
		if strings.TrimSpace(stripEscapes([]byte(run))) != "" {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------------------
// the combined decision
// ---------------------------------------------------------------------------

// Detector combines the layers in order of trust and reports which one
// answered, so a log can say why the geometry changed.
type Detector struct {
	Alt     FullScreenState
	Process *ProcessWatcher
	Signals *FrameSignals

	// Forced is layer 4: the user's own key. No detector may be the last word.
	Forced    bool
	ForcedSet bool
}

// Decide returns whether the child needs a real-sized console, and why.
func (d *Detector) Decide() (bool, string) {
	if d.ForcedSet {
		return d.Forced, "forced by the user"
	}
	if d.Process != nil {
		if on, name := d.Process.Poll(); on {
			return true, "process: " + name
		}
	}
	if on, why := d.Alt.Active(); on {
		return true, why
	}
	if d.Signals != nil && d.Signals.lastTerminators != 0 {
		// Signals are only consulted when the layers above are silent, and
		// only report what Observe last concluded.
		if on, why := d.Signals.Observe(nil); on {
			return true, "heuristic: " + why
		}
	}
	return false, ""
}
