package terminal

import (
	"sync"
	"time"
)

// terminalRedrawInterval bounds how often a burst of PTY output can wake the
// UI renderer. PTY data is still parsed immediately; only redundant frame
// requests are coalesced while the renderer is catching up.
const terminalRedrawInterval = 16 * time.Millisecond

type TerminalRedrawScheduler struct {
	mu      sync.Mutex
	pending bool
	missed  bool
	stopped bool
	redraw  func()
}

func NewTerminalRedrawScheduler(redraw func()) *TerminalRedrawScheduler {
	return &TerminalRedrawScheduler{redraw: redraw}
}

func (s *TerminalRedrawScheduler) Request() {
	if s == nil {
		return
	}

	s.mu.Lock()
	if s.stopped {
		s.mu.Unlock()
		return
	}
	if s.pending {
		// Remember that newer output arrived while the window is open; the
		// timer below turns it into exactly one trailing frame. Dropping
		// these outright is what made a program that paints once and then
		// falls silent invisible: mc writes its whole screen in a handful
		// of reads a millisecond apart, so every chunk after the first
		// landed inside the window and nothing ever woke the renderer
		// again (#249).
		s.missed = true
		s.mu.Unlock()
		return
	}
	s.pending = true
	redraw := s.redraw
	s.mu.Unlock()

	// Keep the first frame of a burst responsive, then suppress further
	// requests until the interval expires. FrameManager.Redraw itself is
	// asynchronous and non-blocking, so this is safe from the PTY reader.
	if redraw != nil {
		redraw()
	}
	s.arm()
}

// arm closes the coalescing window after terminalRedrawInterval. If
// requests arrived while it was open, one trailing frame is drawn and a new
// window is opened for whatever arrives during that frame, so a sustained
// stream still costs at most one frame per interval while the last chunk of
// a burst is never left unseen.
func (s *TerminalRedrawScheduler) arm() {
	time.AfterFunc(terminalRedrawInterval, func() {
		s.mu.Lock()
		if s.stopped {
			s.pending = false
			s.missed = false
			s.mu.Unlock()
			return
		}
		if !s.missed {
			s.pending = false
			s.mu.Unlock()
			return
		}
		s.missed = false
		redraw := s.redraw
		s.mu.Unlock()

		if redraw != nil {
			redraw()
		}
		s.arm()
	})
}

func (s *TerminalRedrawScheduler) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.stopped = true
	s.pending = false
	s.missed = false
	s.mu.Unlock()
}
