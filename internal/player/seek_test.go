package player

import (
	"bytes"
	"io"
	"music-engine/internal/audio"
	"os"
	"path/filepath"
	"testing"
)

type seekOutput struct{}
type seekStream struct{ playing bool }

func (*seekOutput) Open(io.Reader, audio.PCMFormat) (audio.Stream, error) { return &seekStream{}, nil }
func (s *seekStream) Play()                                               { s.playing = true }
func (s *seekStream) Pause()                                              { s.playing = false }
func (s *seekStream) IsPlaying() bool                                     { return s.playing }
func (*seekStream) Err() error                                            { return nil }
func (*seekStream) SetVolume(float64)                                     {}
func (*seekStream) Close() error                                          { return nil }

func TestSeekPreservesPauseVolumeAndRejectsInvalidPosition(t *testing.T) {
	frame := make([]byte, 417)
	copy(frame, []byte{0xff, 0xfb, 0x90, 0})
	path := filepath.Join(t.TempDir(), "audio.mp3")
	if err := os.WriteFile(path, bytes.Repeat(frame, 100), 0600); err != nil {
		t.Fatal(err)
	}
	p := NewWithOutput(&seekOutput{})
	defer p.Stop()
	if err := p.SeekMS(0); err != ErrNoTrack {
		t.Fatalf("no track: %v", err)
	}
	if err := p.Play(path); err != nil {
		t.Fatal(err)
	}
	p.SetVolume(.4)
	if err := p.SeekMS(500); err != nil {
		t.Fatal(err)
	}
	state := p.GetState()
	if state.Status != "PLAYING" || *state.PositionMS != 500 || state.Volume != .4 {
		t.Fatalf("seek: %+v", state)
	}
	p.Pause()
	if err := p.SeekMS(1000); err != nil {
		t.Fatal(err)
	}
	state = p.GetState()
	if state.Status != "PAUSED" || *state.PositionMS != 1000 {
		t.Fatalf("pause not preserved: %+v", state)
	}
	for _, invalid := range []int{-1, 100000} {
		if err := p.SeekMS(invalid); err == nil {
			t.Fatal("accepted invalid position")
		}
	}
	if *p.GetState().PositionMS != 1000 {
		t.Fatal("invalid seek changed position")
	}
	if err := p.SeekMS(0); err != nil {
		t.Fatal(err)
	}
	if *p.GetState().PositionMS != 0 {
		t.Fatal("cannot seek backward")
	}
	p.audio.(*seekStream).playing = false
	p.paused = false
	if p.GetState().StopReason != "ended" {
		t.Fatal("natural EOF not reported")
	}
	p.Stop()
	if p.GetState().StopReason != "manual" {
		t.Fatal("manual stop confused with EOF")
	}
}
