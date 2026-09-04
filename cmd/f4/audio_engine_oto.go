//go:build !noffi && (windows || ((linux || darwin || freebsd) && (amd64 || arm64)))

package main

import (
	"errors"
	"io"
	"math"
	"os"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	"github.com/hajimehoshi/go-mp3"
)

// audioEngine owns the single output device of the process and plays one
// MP3 at a time. Decoding is go-mp3 (pure Go); output is oto, which talks to
// ALSA / CoreAudio / WASAPI through purego, so no cgo is needed on the three
// desktop platforms. Both were already in the module graph via ebiten.
//
// The oto context is created lazily on the first Load and its sample rate is
// fixed for the life of the process, so a track recorded at another rate is
// resampled (linear interpolation) before it reaches the device. That is not
// audiophile-grade, but it keeps one context, one player type and no
// re-initialisation of the device between tracks.
//
// All methods are safe to call from the UI thread; nothing here blocks on
// the device except NewContext, which is done once.
type audioEngine struct {
	mu      sync.Mutex
	ctx     *oto.Context
	ctxRate int
	ctxErr  error

	player *oto.Player
	file   *os.File
	tap    *pcmTap

	duration time.Duration
	info     audioTrackInfo
	volume   float64
	loaded   bool
}

func newAudioEngine() *audioEngine {
	return &audioEngine{volume: 0.8}
}

func (a *audioEngine) ensureContext(rate int) error {
	if a.ctx != nil || a.ctxErr != nil {
		return a.ctxErr
	}
	ctx, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   rate,
		ChannelCount: 2,
		Format:       oto.FormatSignedInt16LE,
	})
	if err != nil {
		a.ctxErr = errors.Join(errAudioUnavailable, err)
		return a.ctxErr
	}
	<-ready
	a.ctx = ctx
	a.ctxRate = rate
	return nil
}

// Load opens path, decodes its header and prepares a paused player. Play
// starts it. Any previously loaded track is dropped.
func (a *audioEngine) Load(path string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unloadLocked()

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	dec, err := mp3.NewDecoder(f)
	if err != nil {
		f.Close()
		return err
	}
	srcRate := dec.SampleRate()
	if err := a.ensureContext(srcRate); err != nil {
		f.Close()
		return err
	}
	var src io.Reader = dec
	if srcRate != a.ctxRate {
		src = newLinearResampler(dec, srcRate, a.ctxRate)
	}
	a.duration = 0
	if n := dec.Length(); n > 0 {
		a.duration = time.Duration(float64(n) / float64(srcRate*audioBytesPerFrame) * float64(time.Second))
	}
	a.info = audioTrackInfo{SampleRate: srcRate}
	if st, err := f.Stat(); err == nil && a.duration > 0 {
		a.info.BitrateKbps = int(math.Round(float64(st.Size()) * 8 / a.duration.Seconds() / 1000))
	}
	a.info.Mono = mp3FirstFrameIsMono(path)

	a.tap = newPCMTap(src, a.ctxRate)
	a.file = f
	a.player = a.ctx.NewPlayer(a.tap)
	a.player.SetVolume(a.volume)
	a.loaded = true
	return nil
}

func (a *audioEngine) unloadLocked() {
	if a.player != nil {
		a.player.Pause()
		//_ = a.player.Close() // fix linter error
		// SA1019: (*github.com/ebitengine/oto/v3.Player).Close is deprecated: as of v3.4. you don't have to call Close. (staticcheck)
		a.player = nil
	}
	if a.file != nil {
		a.file.Close()
		a.file = nil
	}
	a.tap = nil
	a.loaded = false
}

func (a *audioEngine) Close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unloadLocked()
}

func (a *audioEngine) Play() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.player != nil {
		a.player.Play()
	}
}

func (a *audioEngine) Pause() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.player != nil {
		a.player.Pause()
	}
}

// TogglePause flips between playing and paused and reports the new state.
func (a *audioEngine) TogglePause() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.player == nil {
		return false
	}
	if a.player.IsPlaying() {
		a.player.Pause()
		return false
	}
	a.player.Play()
	return true
}

// Stop unloads the track entirely; Play after Stop needs a Load first.
func (a *audioEngine) Stop() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.unloadLocked()
}

func (a *audioEngine) IsPlaying() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.player != nil && a.player.IsPlaying()
}

func (a *audioEngine) IsLoaded() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loaded
}

// Finished is true once the decoder hit EOF and the player drained its
// buffer — the moment to advance to the next track.
func (a *audioEngine) Finished() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.loaded && a.tap != nil && a.tap.eof() && !a.player.IsPlaying()
}

func (a *audioEngine) Volume() float64 { return a.volume }

func (a *audioEngine) SetVolume(v float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.volume = math.Max(0, math.Min(1, v))
	if a.player != nil {
		a.player.SetVolume(a.volume)
	}
}

func (a *audioEngine) Position() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.tap == nil {
		return 0
	}
	// Subtract what oto has read from us but not yet played, so the
	// clock does not run ahead of the speakers by the buffer size.
	consumed := a.tap.bytesRead()
	if a.player != nil {
		consumed -= int64(a.player.BufferedSize())
	}
	if consumed < 0 {
		consumed = 0
	}
	return time.Duration(float64(consumed) / float64(a.ctxRate*audioBytesPerFrame) * float64(time.Second))
}

func (a *audioEngine) Duration() time.Duration {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.duration
}

func (a *audioEngine) Info() audioTrackInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.info
}

// Spectrum returns bands bar heights in 0..1 computed from the most recent
// PCM that passed through the tap. See pcmTap.spectrum.
func (a *audioEngine) Spectrum(bands int) []float64 {
	a.mu.Lock()
	tap := a.tap
	a.mu.Unlock()
	if tap == nil {
		return make([]float64, bands)
	}
	return tap.spectrum(bands)
}
