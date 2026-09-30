package media

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtinput"
	"github.com/unxed/vtui"
)

func TestFrameViewSpecSizesThePictureToTheScreen(t *testing.T) {
	text := vtui.NewScreenBuf()
	text.Writer = io.Discard
	text.AllocBuf(80, 25)
	spec, graphics := frameViewSpec(text, 40, 10, 3*time.Second)
	if graphics || spec.Width != 40 || spec.Height != 20 || spec.FPS != frameViewTextFPS || spec.Start != 3*time.Second {
		t.Fatalf("text spec = %+v graphics=%v, want 40x20 at %d fps", spec, graphics, frameViewTextFPS)
	}

	gfx := newImageTestScreen(t) // kitty, 8x16 cells
	spec, graphics = frameViewSpec(gfx, 100, 40, 0)
	if !graphics || spec.Width > frameViewMaxWidth || spec.Height > frameViewMaxHeight || spec.FPS != frameViewGraphicsFPS {
		t.Fatalf("graphics spec = %+v graphics=%v", spec, graphics)
	}
	if spec.Width%2 != 0 || spec.Height%2 != 0 {
		t.Errorf("odd frame size %dx%d", spec.Width, spec.Height)
	}
	small, _ := frameViewSpec(gfx, 10, 5, 0)
	if small.Width != 80 || small.Height != 80 {
		t.Errorf("a small area asked for %dx%d, want 80x80 (its own pixels)", small.Width, small.Height)
	}
}

func TestFrameVideoViewPlaysFramesAsHalfBlocks(t *testing.T) {
	useFakeFFmpeg(t, "head -c 4800 /dev/zero") // three 20x20 frames
	scr := vtui.NewScreenBuf()
	scr.Writer = io.Discard
	scr.AllocBuf(20, 12)
	vtui.SetDefaultPalette()

	fv := NewFrameVideoView(nil, videoFileForTest(t))
	fv.topBar.ColorIdx = 0 // the theme's palette is not loaded in a unit test
	fv.ResizeConsole(20, 12)
	fv.Show(scr) // starts the run

	deadline := time.Now().Add(10 * time.Second)
	for {
		fv.mu.Lock()
		done := fv.ended && fv.frame != nil
		fv.mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the frames never arrived")
		}
		time.Sleep(10 * time.Millisecond)
	}
	fv.Show(scr)
	var dump bytes.Buffer
	scr.Dump(&dump)
	if !strings.Contains(dump.String(), "▀") {
		t.Fatalf("no half blocks on the screen:\n%s", dump.String())
	}
	if !strings.Contains(fv.statusText(), "end") {
		t.Errorf("status %q does not say the film ended", fv.statusText())
	}

	// A resized screen restarts the run from where it was.
	fv.ResizeConsole(30, 12)
	fv.Show(scr)
	fv.mu.Lock()
	cols := fv.runCols
	fv.mu.Unlock()
	if cols != 30 {
		t.Errorf("the run was not restarted for the new size (cols %d)", cols)
	}

	if !fv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_SPACE}) || !fv.isPaused() {
		t.Error("space did not pause")
	}
	closed := 0
	fv.OnClose = func() { closed++ }
	if !fv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_ESCAPE}) || !fv.IsDone() || closed != 1 {
		t.Errorf("escape did not close the view (done=%v, callbacks=%d)", fv.IsDone(), closed)
	}
}

func TestFrameVideoViewReportsAFailedStartAndIgnoresOtherKeys(t *testing.T) {
	useFakeFFmpeg(t, "exit 3")
	scr := vtui.NewScreenBuf()
	scr.Writer = io.Discard
	scr.AllocBuf(20, 12)
	vtui.SetDefaultPalette()
	fv := NewFrameVideoView(nil, videoFileForTest(t))
	fv.topBar.ColorIdx = 0
	fv.ResizeConsole(20, 12)
	fv.Show(scr)
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(fv.statusText(), "exit") {
		if time.Now().After(deadline) {
			t.Fatalf("status %q never reported the failure", fv.statusText())
		}
		time.Sleep(10 * time.Millisecond)
	}
	if fv.ProcessKey(nil) || fv.ProcessKey(&vtinput.InputEvent{KeyDown: true, VirtualKeyCode: vtinput.VK_B}) {
		t.Error("an unrelated key was handled")
	}
	fv.Close()
}

// videoFileForTest is a file that exists, which is all the frame source asks of
// a video before it hands it to ffmpeg.
func videoFileForTest(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "clip.mp4")
	if err := os.WriteFile(path, []byte("not really a video"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
