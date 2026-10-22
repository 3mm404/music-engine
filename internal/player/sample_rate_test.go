package player

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"music-engine/internal/audio"
	"music-engine/internal/decoder"
)

type rateOutput struct {
	source io.Reader
	format audio.PCMFormat
}

func (o *rateOutput) Open(source io.Reader, format audio.PCMFormat) (audio.Stream, error) {
	o.source, o.format = source, format
	return &seekStream{}, nil
}

func TestMP3RatesUseStableOutputAndSeek(t *testing.T) {
	for _, fixture := range []struct {
		rate, size int
		header     []byte
	}{
		{44100, 417, []byte{0xff, 0xfb, 0x90, 0}}, {48000, 384, []byte{0xff, 0xfb, 0x94, 0}},
		{32000, 576, []byte{0xff, 0xfb, 0x98, 0}}, {22050, 208, []byte{0xff, 0xf3, 0x80, 0}},
		{24000, 192, []byte{0xff, 0xf3, 0x84, 0}}, {16000, 288, []byte{0xff, 0xf3, 0x88, 0}},
		{11025, 417, []byte{0xff, 0xe3, 0x80, 0}}, {12000, 384, []byte{0xff, 0xe3, 0x84, 0}},
		{8000, 576, []byte{0xff, 0xe3, 0x88, 0}},
	} {
		t.Run(fmt.Sprint(fixture.rate), func(t *testing.T) {
			frame := make([]byte, fixture.size)
			copy(frame, fixture.header)
			path := filepath.Join(t.TempDir(), "audio.mp3")
			if err := os.WriteFile(path, bytes.Repeat(frame, 120), 0600); err != nil {
				t.Fatal(err)
			}
			output := &rateOutput{}
			p := NewRemoteWithOutput(output, 44100, decoder.HTTPS{})
			defer p.Stop()
			if err := p.Play(path); err != nil {
				t.Fatal(err)
			}
			if p.stream.SampleRate() != fixture.rate {
				t.Fatalf("fixture rate: %d", p.stream.SampleRate())
			}
			if output.format.SampleRate != 44100 {
				t.Fatalf("output clock changed: %+v", output.format)
			}
			inputFrames := p.stream.Length() / 4
			pcm, err := io.ReadAll(output.source)
			if err != nil {
				t.Fatal(err)
			}
			expected := (inputFrames*44100 + int64(fixture.rate) - 1) / int64(fixture.rate) * 4
			if int64(len(pcm)) != expected {
				t.Fatalf("duration changed: %d bytes, want %d", len(pcm), expected)
			}
			p.Pause()
			if err := p.SeekMS(500); err != nil {
				t.Fatal(err)
			}
			state := p.GetState()
			if state.Status != "PAUSED" || state.PositionMS == nil || (*state.PositionMS < 499 || *state.PositionMS > 500) {
				t.Fatalf("seek/pause changed: %+v", state)
			}
			if output.format.SampleRate != 44100 {
				t.Fatal("seek changed output clock")
			}
		})
	}
}
