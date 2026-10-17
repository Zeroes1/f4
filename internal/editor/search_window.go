package editor

import (
	"context"

	"github.com/unxed/f4/internal/piecetable"
	"github.com/unxed/f4/internal/textsearch"
)

// findLiteralWindowed is textsearch.FindMatch for a literal pattern, run over
// the buffer one window at a time instead of over a copy of all of it.
//
// A buffer that cannot be scanned in place — a file read through the chunk
// buffer, or any file once it has been edited — used to be assembled whole on
// the heap before the first byte was compared: a search of a 1 GB log cost a
// gigabyte, and on a remote file the whole download, to find a match on the
// next line. Read and Pt are the caller's captured view of the buffer, as for
// collectMatchSpans, and windows are read the same way, through
// readSearchWindow.
//
// Consecutive windows overlap by more than any match can be long, and a match
// is only taken from a window when it could not also have started in the next
// one, so the result is the one FindMatch would give for the whole buffer.
func (ev *EditorView) findLiteralWindowed(ctx context.Context, read fileChunkReader, Pt *piecetable.PieceTable,
	pattern string, caseSensitive, reverse, next bool, startOff int) (int, int, error) {

	size := Pt.Size()
	if size == 0 || pattern == "" {
		return -1, len(pattern), nil
	}
	// A folded match can be longer in bytes than the pattern it matched;
	// see collectMatchSpans.
	overlap := max(4*len(pattern), 64)
	buf := make([]byte, min(findAllWindow+overlap, size))

	if !reverse {
		pos := startOff
		if next {
			pos++
		}
		for pos = max(pos, 0); pos < size; {
			if err := ctx.Err(); err != nil {
				return -1, 0, err
			}
			win, err := ev.readSearchWindow(ctx, read, Pt, buf, pos, min(len(buf), size-pos))
			if err != nil {
				return -1, 0, err
			}
			if len(win) == 0 {
				break
			}
			last := pos+len(win) >= size
			off, mLen, _ := textsearch.FindMatch(win, pattern, caseSensitive, false, false, false, false, 0)
			// A match starting in the overlap may be cut short by the end of
			// the window, or be the second of two where the first is: the
			// next window, which starts before it, decides.
			if off >= 0 && (last || off < len(win)-overlap) {
				return pos + off, mLen, nil
			}
			if last {
				break
			}
			pos += max(len(win)-overlap, 1)
		}
		return -1, len(pattern), nil
	}

	end := startOff
	if next {
		end--
	}
	end = min(end, size)
	for end > 0 {
		if err := ctx.Err(); err != nil {
			return -1, 0, err
		}
		from := max(end-len(buf), 0)
		win, err := ev.readSearchWindow(ctx, read, Pt, buf, from, end-from)
		if err != nil {
			return -1, 0, err
		}
		// The match ending last before end is the one wanted, and any match
		// that ends by then and starts in this window lies wholly inside it.
		off, mLen, _ := textsearch.FindMatch(win, pattern, caseSensitive, true, false, false, false, len(win))
		if off >= 0 {
			return from + off, mLen, nil
		}
		if from == 0 {
			break
		}
		// The next window takes in a match that straddles this one's start.
		end = from + overlap
	}
	return -1, len(pattern), nil
}

// scansInPlace reports whether Pt hands out its whole text as one window,
// which a search reads with no copy at all.
func scansInPlace(Pt *piecetable.PieceTable) bool {
	if Pt.Size() == 0 {
		return true
	}
	_, ok := Pt.View(0, Pt.Size())
	return ok
}
