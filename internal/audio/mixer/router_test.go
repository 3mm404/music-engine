package mixer

import (
	"bytes"
	"encoding/binary"
	"io"
	"sync"
	"testing"

	"music-engine/internal/audio"
)

type captureOutput struct {
	mu     sync.Mutex
	source io.Reader
	format audio.PCMFormat
	stream *captureStream
	calls  int
}

type captureStream struct {
	mu      sync.Mutex
	playing bool
}

func (o *captureOutput) Open(source io.Reader, format audio.PCMFormat) (audio.Stream, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.calls++
	o.source, o.format = source, format
	o.stream = &captureStream{}
	return o.stream, nil
}

func (s *captureStream) Play() {
	s.mu.Lock()
	s.playing = true
	s.mu.Unlock()
}

func (s *captureStream) Pause() {
	s.mu.Lock()
	s.playing = false
	s.mu.Unlock()
}

func (s *captureStream) IsPlaying() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.playing
}

func (s *captureStream) Err() error        { return nil }
func (s *captureStream) SetVolume(float64) {}

func (o *captureOutput) readFrame(t *testing.T) []int16 {
	t.Helper()
	o.mu.Lock()
	source, channels := o.source, o.format.Channels
	o.mu.Unlock()
	frame := make([]byte, channels*2)
	if _, err := io.ReadFull(source, frame); err != nil {
		t.Fatal(err)
	}
	samples := make([]int16, channels)
	for i := range samples {
		samples[i] = int16(binary.LittleEndian.Uint16(frame[i*2:]))
	}
	return samples
}

func pcm(samples ...int16) io.Reader {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	return bytes.NewReader(data)
}

func openZone(t *testing.T, router *Router, zoneID string, source io.Reader, channels int) audio.Stream {
	t.Helper()
	stream, err := router.OutputForZone(zoneID).Open(source, audio.PCMFormat{
		SampleRate:   44100,
		Channels:     channels,
		SampleFormat: audio.SignedInt16LE,
	})
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func TestRoutesSimultaneousStereoZonesToInterleavedFrame(t *testing.T) {
	backend := &captureOutput{}
	router := New(backend)
	if err := router.Configure([]Route{{ZoneID: "pool", Channels: []int{1, 2}}, {ZoneID: "lobby", Channels: []int{3, 4}}}); err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	pool := openZone(t, router, "pool", pcm(100, 200), 2)
	lobby := openZone(t, router, "lobby", pcm(300, 400), 2)
	pool.Play()
	lobby.Play()
	if got, want := backend.readFrame(t), []int16{100, 200, 300, 400}; !equalSamples(got, want) {
		t.Fatalf("interleaved frame = %v, want %v", got, want)
	}
	if backend.calls != 1 || backend.format.Channels != 4 {
		t.Fatalf("opened %d backend streams with format %+v, want one 4-channel stream", backend.calls, backend.format)
	}
}

func TestClearingRoutesKeepsContinuousSilenceAndCanRestoreThem(t *testing.T) {
	backend := &captureOutput{}
	router := New(backend)
	if err := router.Configure([]Route{{ZoneID: "A", Channels: []int{1, 2}}}); err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	stream := openZone(t, router, "A", pcm(10, 20, 30, 40), 2)
	stream.Play()
	if got, want := backend.readFrame(t), []int16{10, 20}; !equalSamples(got, want) {
		t.Fatalf("initial frame = %v, want %v", got, want)
	}
	if err := router.Configure(nil); err != nil {
		t.Fatal(err)
	}
	if got, want := backend.readFrame(t), []int16{0, 0}; !equalSamples(got, want) {
		t.Fatalf("unrouted frame = %v, want %v", got, want)
	}
	if err := router.Configure([]Route{{ZoneID: "A", Channels: []int{1, 2}}}); err != nil {
		t.Fatal(err)
	}
	if got, want := backend.readFrame(t), []int16{30, 40}; !equalSamples(got, want) {
		t.Fatalf("restored frame = %v, want %v", got, want)
	}
}

func TestMonoRoutingSilenceVolumePauseAndEndIsolation(t *testing.T) {
	backend := &captureOutput{}
	router := New(backend)
	if err := router.Configure([]Route{{ZoneID: "stereo", Channels: []int{1, 2}}, {ZoneID: "mono", Channels: []int{4}}}); err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	stereo := openZone(t, router, "stereo", pcm(100, 200, 100, 200, 100, 200, 100, 200, 100, 200), 2)
	mono := openZone(t, router, "mono", pcm(300, 400, 300, 400), 2)
	stereo.SetVolume(0.5)
	stereo.Play()
	mono.Play()
	if got, want := backend.readFrame(t), []int16{50, 100, 0, 350}; !equalSamples(got, want) {
		t.Fatalf("mono/stereo frame = %v, want %v", got, want)
	}
	if got, want := backend.readFrame(t), []int16{50, 100, 0, 350}; !equalSamples(got, want) {
		t.Fatalf("second frame = %v, want %v", got, want)
	}
	if !mono.IsPlaying() || !stereo.IsPlaying() {
		t.Fatal("zone unexpectedly stopped before its final frame was consumed")
	}
	if got, want := backend.readFrame(t), []int16{50, 100, 0, 0}; !equalSamples(got, want) {
		t.Fatalf("EOF isolation = %v, want %v", got, want)
	}
	if mono.IsPlaying() || !stereo.IsPlaying() {
		t.Fatal("EOF in mono zone changed the stereo zone state")
	}
	if got, want := backend.readFrame(t), []int16{50, 100, 0, 0}; !equalSamples(got, want) {
		t.Fatalf("post-EOF silence = %v, want %v", got, want)
	}
	stereo.Pause()
	if got, want := backend.readFrame(t), []int16{0, 0, 0, 0}; !equalSamples(got, want) {
		t.Fatalf("paused frame = %v, want %v", got, want)
	}
	if stereo.IsPlaying() {
		t.Fatal("paused zone is still reported playing")
	}
	stereo.Play()
	if got, want := backend.readFrame(t), []int16{50, 100, 0, 0}; !equalSamples(got, want) {
		t.Fatalf("resumed frame = %v, want %v", got, want)
	}
	stereo.Pause() // Player.Stop pauses and discards its logical stream.
	if got, want := backend.readFrame(t), []int16{0, 0, 0, 0}; !equalSamples(got, want) {
		t.Fatalf("stopped-zone frame = %v, want %v", got, want)
	}
}

func TestRouterAcceptsPartialSourceReads(t *testing.T) {
	backend := &captureOutput{}
	router := New(backend)
	if err := router.Configure([]Route{{ZoneID: "A", Channels: []int{1, 2}}}); err != nil {
		t.Fatal(err)
	}
	defer router.Close()
	stream := openZone(t, router, "A", &chunkReader{source: pcm(123, -456)}, 2)
	stream.Play()
	if got, want := backend.readFrame(t), []int16{123, -456}; !equalSamples(got, want) {
		t.Fatalf("partial-read frame = %v, want %v", got, want)
	}
}

func TestConfigureRejectsInvalidAndOverlappingRoutes(t *testing.T) {
	tests := []struct {
		name   string
		routes []Route
	}{
		{"empty route", []Route{{ZoneID: "A"}}},
		{"too many channels", []Route{{ZoneID: "A", Channels: []int{1, 2, 3}}}},
		{"zero channel", []Route{{ZoneID: "A", Channels: []int{0}}}},
		{"channel limit", []Route{{ZoneID: "A", Channels: []int{maxOutputChannels + 1}}}},
		{"duplicate channels", []Route{{ZoneID: "A", Channels: []int{1, 1}}}},
		{"overlapping zones", []Route{{ZoneID: "A", Channels: []int{1, 2}}, {ZoneID: "B", Channels: []int{2}}}},
		{"duplicate zone", []Route{{ZoneID: "A", Channels: []int{1}}, {ZoneID: "A", Channels: []int{2}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := New(nil).Configure(test.routes); err == nil {
				t.Fatalf("Configure(%v) succeeded", test.routes)
			}
		})
	}
}

func TestConcurrentReadsAndZoneControlsAndClose(t *testing.T) {
	backend := &captureOutput{}
	router := New(backend)
	if err := router.Configure([]Route{{ZoneID: "A", Channels: []int{1, 2}}, {ZoneID: "B", Channels: []int{3, 4}}}); err != nil {
		t.Fatal(err)
	}
	a := openZone(t, router, "A", &repeatingReader{frame: pcmBytes(10, 20)}, 2)
	b := openZone(t, router, "B", &repeatingReader{frame: pcmBytes(30, 40)}, 2)
	a.Play()
	b.Play()
	var group sync.WaitGroup
	group.Go(func() {
		for i := 0; i < 300; i++ {
			backend.readFrame(t)
		}
	})
	group.Go(func() {
		for i := 0; i < 300; i++ {
			a.SetVolume(float64(i%101) / 100)
			if i%2 == 0 {
				a.Pause()
			} else {
				a.Play()
			}
		}
	})
	group.Go(func() {
		for i := 0; i < 300; i++ {
			b.SetVolume(float64(100-i%101) / 100)
		}
	})
	group.Wait()
	if err := router.Close(); err != nil {
		t.Fatal(err)
	}
	if backend.stream.IsPlaying() {
		t.Fatal("backend remains active after router close")
	}
	if err := router.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func equalSamples(a, b []int16) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

type chunkReader struct {
	source io.Reader
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	return r.source.Read(p)
}

type repeatingReader struct {
	frame []byte
}

func (r *repeatingReader) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		n += copy(p[n:], r.frame[:min(len(r.frame), len(p)-n)])
	}
	return n, nil
}

func pcmBytes(samples ...int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	return data
}
