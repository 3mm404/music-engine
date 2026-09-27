// Package oto adapts Oto v3 to the shared PCM output interface.
package oto

import (
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"

	"music-engine/internal/audio"
)

const outputBuffer = 50 * time.Millisecond

var device struct {
	sync.Mutex
	context   *oto.Context
	format    audio.PCMFormat
	attempted bool
	err       error
}

type Output struct{}

func NewOutput() *Output { return &Output{} }

type stream struct {
	context *oto.Context
	player  *oto.Player
}

func (o *Output) Open(source io.Reader, format audio.PCMFormat) (audio.Stream, error) {
	if format.SampleFormat != audio.SignedInt16LE {
		return nil, fmt.Errorf("formato PCM no compatible con Oto: %d", format.SampleFormat)
	}
	if format.SampleRate <= 0 || format.Channels <= 0 {
		return nil, fmt.Errorf("formato PCM invalido: %d Hz, %d canales", format.SampleRate, format.Channels)
	}

	device.Lock()
	defer device.Unlock()
	if !device.attempted {
		device.attempted = true
		var ready chan struct{}
		device.context, ready, device.err = oto.NewContext(&oto.NewContextOptions{
			SampleRate:   format.SampleRate,
			ChannelCount: format.Channels,
			Format:       oto.FormatSignedInt16LE,
			BufferSize:   outputBuffer,
		})
		if device.err == nil {
			<-ready
			device.err = device.context.Err()
		}
		device.format = format
	}
	if device.err != nil {
		return nil, fmt.Errorf("inicializar Oto: %w", device.err)
	}
	if device.format != format {
		return nil, fmt.Errorf("el contexto usa %d Hz y %d canales; el stream requiere %d Hz y %d canales", device.format.SampleRate, device.format.Channels, format.SampleRate, format.Channels)
	}
	if err := device.context.Err(); err != nil {
		return nil, fmt.Errorf("salida de audio: %w", err)
	}
	return &stream{context: device.context, player: device.context.NewPlayer(source)}, nil
}

func (s *stream) Play()           { s.player.Play() }
func (s *stream) Pause()          { s.player.PauseAndStopReading() }
func (s *stream) IsPlaying() bool { return s.player.IsPlaying() }
func (s *stream) SetVolume(volume float64) {
	s.player.SetVolume(volume)
}

func (s *stream) Err() error {
	if err := s.context.Err(); err != nil {
		return fmt.Errorf("salida de audio: %w", err)
	}
	return s.player.Err()
}
