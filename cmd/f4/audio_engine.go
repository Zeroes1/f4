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

// audioTrackInfo is what the panel prints in the "128kbps 44kHz stereo"
// slot. Bitrate is the average over the file (size / duration), which is
// what most players show for VBR anyway; the channel mode comes from the
// first frame header because go-mp3 always emits stereo PCM.
type audioTrackInfo struct {
	BitrateKbps int
	SampleRate  int
	Mono        bool
}

const audioBytesPerFrame = 4 // go-mp3 output: 16-bit LE stereo

var errAudioUnavailable = errors.New("audio output is not available on this system")

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
		_ = a.player.Close()
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

// pcmTap sits between the decoder and the device. It counts bytes for the
// position clock and keeps the last pcmTapWindow mono samples for the
// spectrum display. It is the only place that sees the PCM, so it is cheap:
// one pass over the bytes oto asks for anyway.
type pcmTap struct {
	r    io.Reader
	rate int

	mu   sync.Mutex
	n    int64
	done bool
	ring [pcmTapWindow]float64
	pos  int
}

const pcmTapWindow = 512

func newPCMTap(r io.Reader, rate int) *pcmTap {
	return &pcmTap{r: r, rate: rate}
}

func (t *pcmTap) Read(p []byte) (int, error) {
	n, err := t.r.Read(p)
	t.mu.Lock()
	t.n += int64(n)
	// Keep a few hundred samples, subsampled: enough for a 16-band bar
	// display, far too few for anything an analyser would call a
	// spectrum. Taking every 4th frame keeps the loop trivial.
	for i := 0; i+3 < n; i += audioBytesPerFrame * 4 {
		l := int16(uint16(p[i]) | uint16(p[i+1])<<8)
		r := int16(uint16(p[i+2]) | uint16(p[i+3])<<8)
		t.ring[t.pos] = (float64(l) + float64(r)) / (2 * 32768)
		t.pos = (t.pos + 1) % pcmTapWindow
	}
	if err == io.EOF {
		t.done = true
	}
	t.mu.Unlock()
	return n, err
}

func (t *pcmTap) bytesRead() int64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.n
}

func (t *pcmTap) eof() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.done
}

// spectrum evaluates the Goertzel filter at bands log-spaced centre
// frequencies between 60 Hz and ~12 kHz over the ring buffer. That is
// bands×window multiply-adds (about 8k for 16 bands), which is nothing at
// the ~7 Hz the panel repaints; an FFT would be no faster to write or run
// at this size. Output is dB-ish scaled into 0..1.
func (t *pcmTap) spectrum(bands int) []float64 {
	out := make([]float64, bands)
	if bands <= 0 {
		return out
	}
	t.mu.Lock()
	var buf [pcmTapWindow]float64
	copy(buf[:], t.ring[:])
	rate := float64(t.rate) / 4 // subsampled in Read
	t.mu.Unlock()
	lo, hi := 60.0, math.Min(12000, rate/2*0.95)
	for b := 0; b < bands; b++ {
		f := lo * math.Pow(hi/lo, float64(b)/float64(max(bands-1, 1)))
		coeff := 2 * math.Cos(2*math.Pi*f/rate)
		var s0, s1, s2 float64
		for i := 0; i < pcmTapWindow; i++ {
			// Hann window keeps neighbouring bands from bleeding into
			// each other on a flat spectrum.
			w := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/pcmTapWindow)
			s0 = buf[i]*w + coeff*s1 - s2
			s2, s1 = s1, s0
		}
		power := s1*s1 + s2*s2 - coeff*s1*s2
		mag := math.Sqrt(math.Max(power, 0)) / (pcmTapWindow / 4)
		db := 20 * math.Log10(mag+1e-9)
		// -50 dB .. 0 dB → 0..1
		v := (db + 50) / 50
		out[b] = math.Max(0, math.Min(1, v))
	}
	return out
}

// linearResampler converts 16-bit stereo LE PCM from srcRate to dstRate.
// It reads whole frames from the source and interpolates between adjacent
// ones; good enough for the 44.1↔48 kHz cases that occur in practice.
type linearResampler struct {
	src     io.Reader
	step    float64 // source frames per output frame
	pos     float64 // fractional position in source frames
	prev    [2]int16
	cur     [2]int16
	have    bool
	eof     bool
	scratch [audioBytesPerFrame]byte
}

func newLinearResampler(src io.Reader, srcRate, dstRate int) *linearResampler {
	return &linearResampler{src: src, step: float64(srcRate) / float64(dstRate)}
}

func (r *linearResampler) next() bool {
	if _, err := io.ReadFull(r.src, r.scratch[:]); err != nil {
		r.eof = true
		return false
	}
	r.prev = r.cur
	r.cur[0] = int16(uint16(r.scratch[0]) | uint16(r.scratch[1])<<8)
	r.cur[1] = int16(uint16(r.scratch[2]) | uint16(r.scratch[3])<<8)
	return true
}

func (r *linearResampler) Read(p []byte) (int, error) {
	if !r.have {
		if !r.next() {
			return 0, io.EOF
		}
		r.prev = r.cur
		r.have = true
	}
	n := 0
	for n+audioBytesPerFrame <= len(p) {
		for r.pos >= 1 {
			if !r.next() {
				if n == 0 {
					return 0, io.EOF
				}
				return n, nil
			}
			r.pos--
		}
		for ch := 0; ch < 2; ch++ {
			v := float64(r.prev[ch])*(1-r.pos) + float64(r.cur[ch])*r.pos
			s := int16(v)
			p[n+ch*2] = byte(s)
			p[n+ch*2+1] = byte(uint16(s) >> 8)
		}
		n += audioBytesPerFrame
		r.pos += r.step
	}
	return n, nil
}

// mp3FirstFrameIsMono skips an ID3v2 tag if present and reads the channel
// mode bits of the first frame header. Any doubt → stereo, which is what
// the decoder outputs regardless.
func mp3FirstFrameIsMono(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	buf := make([]byte, 64*1024)
	n, _ := io.ReadFull(f, buf)
	buf = buf[:n]
	off := 0
	if len(buf) >= 10 && buf[0] == 'I' && buf[1] == 'D' && buf[2] == '3' {
		size := int(buf[6]&0x7f)<<21 | int(buf[7]&0x7f)<<14 | int(buf[8]&0x7f)<<7 | int(buf[9]&0x7f)
		off = 10 + size
	}
	for i := off; i+3 < len(buf); i++ {
		if buf[i] == 0xFF && buf[i+1]&0xE0 == 0xE0 && buf[i+1]&0x18 != 0x08 && buf[i+1]&0x06 != 0 {
			return buf[i+3]>>6 == 3
		}
	}
	return false
}
