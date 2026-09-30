package media

// The frame source (docs/VIDEO.md, V1): frames and their times, decoded by
// ffmpeg to raw RGBA and read off a pipe, at the size the consumer asks for
// and not the file's own. Every rung of the ladder below the X overlay - the
// terminal graphics, the ANSI half-block art, the poster - takes its pictures
// from here, so there is one decoding path and several consumers.
//
// The picture is sized to what the transport can carry: the consumer says
// how big and how many frames a second, and ffmpeg scales and drops frames
// before they ever reach f4.

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/unxed/vtui"
)

// VideoFrame is one decoded picture: Pix is Width*Height RGBA pixels, row by
// row, and PTS is when in the film it belongs.
type VideoFrame struct {
	Pix    []byte
	Width  int
	Height int
	PTS    time.Duration
}

// VideoFrameSpec says what pictures are wanted. Width and Height are the size
// of every frame (ffmpeg scales to it, keeping the proportions and padding the
// rest with black); FPS caps the frame rate; Start is where in the film to
// begin.
type VideoFrameSpec struct {
	Width, Height int
	FPS           int
	Start         time.Duration
}

func (s VideoFrameSpec) valid() error {
	if s.Width <= 0 || s.Height <= 0 {
		return fmt.Errorf("video frames: bad size %dx%d", s.Width, s.Height)
	}
	if s.FPS <= 0 {
		return fmt.Errorf("video frames: bad frame rate %d", s.FPS)
	}
	return nil
}

// FrameBytes is the size of one frame in the pipe.
func (s VideoFrameSpec) FrameBytes() int { return s.Width * s.Height * 4 }

// videoFrameArgs is ffmpeg's command line, kept as a plain function so its
// shape can be checked without ffmpeg or a video.
func videoFrameArgs(Path string, s VideoFrameSpec) []string {
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error"}
	if s.Start > 0 {
		// Before -i: seek by keyframe, which is fast; the frames that follow
		// are timed from Start.
		args = append(args, "-ss", strconv.FormatFloat(s.Start.Seconds(), 'f', 3, 64))
	}
	filter := fmt.Sprintf("fps=%d,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:black",
		s.FPS, s.Width, s.Height, s.Width, s.Height)
	return append(args,
		"-i", Path,
		"-an", "-sn", "-dn",
		"-vf", filter,
		"-pix_fmt", "rgba",
		"-f", "rawvideo",
		"-",
	)
}

// readVideoFrames reads whole frames from r until it ends, giving each to emit
// with its time (Start plus its number over FPS). It stops without error when
// emit returns false. A last frame cut short by the end of the stream is
// dropped, not reported: ffmpeg being killed mid-frame is how a source is
// closed.
func readVideoFrames(r io.Reader, s VideoFrameSpec, emit func(VideoFrame) bool) error {
	if err := s.valid(); err != nil {
		return err
	}
	br := bufio.NewReaderSize(r, s.FrameBytes())
	for n := 0; ; n++ {
		pix := make([]byte, s.FrameBytes())
		if _, err := io.ReadFull(br, pix); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil
			}
			return err
		}
		pts := s.Start + time.Duration(n)*time.Second/time.Duration(s.FPS)
		if !emit(VideoFrame{Pix: pix, Width: s.Width, Height: s.Height, PTS: pts}) {
			return nil
		}
	}
}

// VideoFrameSource is a running ffmpeg and the frames it produces.
type VideoFrameSource struct {
	// Frames delivers the pictures in order and is closed when the film ends,
	// the source is closed or ffmpeg fails (Err then says why).
	Frames <-chan VideoFrame

	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

// OpenVideoFrames starts ffmpeg on Path and returns the source. ErrNeedFFmpeg
// says there is no ffmpeg to be had.
func OpenVideoFrames(ctx context.Context, Path string, s VideoFrameSpec) (*VideoFrameSource, error) {
	if err := s.valid(); err != nil {
		return nil, err
	}
	bin, ok := ToolFFmpeg.Find()
	if !ok {
		return nil, ErrNeedFFmpeg
	}
	if _, err := os.Stat(Path); err != nil {
		return nil, err
	}
	return startVideoFrames(ctx, bin, Path, s)
}

// startVideoFrames is OpenVideoFrames with the binary already found.
func startVideoFrames(ctx context.Context, bin, Path string, s VideoFrameSpec) (*VideoFrameSource, error) {
	ctx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, bin, videoFrameArgs(Path, s)...) //nolint:gosec // bin is the ffmpeg ToolFFmpeg found; the arguments are built by videoFrameArgs
	cmd.Env = toolEnv()
	out, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("%s would not start: %w", filepath.Base(bin), err)
	}
	vtui.DebugLog("VIDEO: %s decoding %q at %dx%d, %d fps", bin, Path, s.Width, s.Height, s.FPS)

	frames := make(chan VideoFrame, 4)
	src := &VideoFrameSource{Frames: frames, cancel: cancel, done: make(chan struct{})}
	go func() {
		defer close(src.done)
		defer close(frames)
		err := readVideoFrames(out, s, func(f VideoFrame) bool {
			select {
			case frames <- f:
				return true
			case <-ctx.Done():
				return false
			}
		})
		waitErr := cmd.Wait()
		switch {
		case err != nil:
			src.err = err
		case ctx.Err() == nil && waitErr != nil:
			src.err = fmt.Errorf("%s: %w", filepath.Base(bin), waitErr)
		}
	}()
	return src, nil
}

// Err is why the source ended early, or nil when the film ran out or the
// source was closed. Valid once Frames is closed.
func (v *VideoFrameSource) Err() error {
	<-v.done
	return v.err
}

// Close stops ffmpeg and waits for the reader to finish.
func (v *VideoFrameSource) Close() {
	v.cancel()
	// Drain, so a reader blocked on a full channel sees the cancel.
	for range v.Frames {
	}
	<-v.done
}
