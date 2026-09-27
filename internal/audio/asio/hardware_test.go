package asio_test

import (
	"encoding/binary"
	"io"
	"math"
	"os"
	"testing"
	"time"

	"music-engine/internal/audio"
	"music-engine/internal/audio/asio"
	"music-engine/internal/audio/mixer"
)

type observedOutput struct {
	*asio.Output
	stream audio.Stream
}

func (o *observedOutput) Open(r io.Reader, f audio.PCMFormat) (audio.Stream, error) {
	s, err := o.Output.Open(r, f)
	o.stream = s
	return s, err
}

// Infinite stereo 440 Hz at -30 dBFS, read only by the ASIO producer.
type tone struct{ frame int }

func (r *tone) Read(p []byte) (int, error) {
	n := len(p) / 4 * 4
	for i := 0; i < n; i += 4 {
		v := int16(1036 * math.Sin(2*math.Pi*440*float64(r.frame)/44100))
		binary.LittleEndian.PutUint16(p[i:], uint16(v))
		binary.LittleEndian.PutUint16(p[i+2:], uint16(v))
		r.frame++
	}
	return n, nil
}

// Opt-in real streaming test: emits a quiet tone through Router -> ASIO CH1-2.
// A passing test proves callback consumption, not reception on the Dante network.
func TestRealDVSStreamingOptional(t *testing.T) {
	if os.Getenv("MUSIC_ENGINE_ASIO_STREAM_TEST") != "1" {
		t.Skip("set MUSIC_ENGINE_ASIO_STREAM_TEST=1 to emit a -30 dBFS tone for 10 seconds")
	}
	o, err := asio.NewOutput(asio.Config{DriverName: "Dante Virtual Soundcard (x64)", SampleRate: 48000, BufferSize: 256, Channels: 8})
	if err != nil {
		t.Fatal(err)
	}
	observed := &observedOutput{Output: o}
	router := mixer.New(observed)
	defer router.Close()
	if err := router.Configure([]mixer.Route{{ZoneID: "probe", Channels: []int{1, 2}}, {ZoneID: "silent", Channels: []int{7, 8}}}); err != nil {
		t.Fatal(err)
	}
	s, err := router.OutputForZone("probe").Open(&tone{}, audio.PCMFormat{SampleRate: 44100, Channels: 2, SampleFormat: audio.SignedInt16LE})
	if err != nil {
		t.Fatal(err)
	}
	s.Play()
	time.Sleep(10 * time.Second)
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	d := observed.stream.(interface{ Diagnostics() asio.StreamDiagnostics }).Diagnostics()
	t.Logf("Router -> real ASIO diagnostics: %+v", d)
	if d.NonzeroSamples[0] == 0 || d.NonzeroSamples[1] == 0 {
		t.Fatal("no signal on ASIO channels 1-2")
	}
	for channel := 2; channel < len(d.NonzeroSamples); channel++ {
		if d.NonzeroSamples[channel] != 0 {
			t.Fatalf("unexpected signal on channel %d", channel+1)
		}
	}
	if d.CallbackCalls == 0 || d.ProducerFrames == 0 || d.ConsumedFrames == 0 || d.OverflowFrames != 0 {
		t.Fatalf("stream did not deliver PCM cleanly: %+v", d)
	}
}
