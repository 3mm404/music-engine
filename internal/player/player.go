// Package player controla una reproduccion local; el backend Oto vive en oto.go.
package player

import (
	"context"
	"errors"
	"sync"
	"time"

	"music-engine/internal/decoder"
)

var ErrNoTrack = errors.New("no hay un archivo cargado; llama a Play primero")

// Player conserva el stream mientras Oto lo lee. Sus metodos admiten concurrencia.
type Player struct {
	mu     sync.Mutex
	stream *decoder.Stream
	audio  *audioOutput
	paused bool
}

func New() *Player { return &Player{} }

// Play abre el archivo e inicia audio en segundo plano. Wait permite esperar el final.
// Un archivo invalido no interrumpe la pista actual.
func (p *Player) Play(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	source, err := decoder.Open(path)
	if err != nil {
		return err
	}
	audio, err := openOutput(source, source.SampleRate())
	if err != nil {
		source.Close()
		return err
	}
	if err := p.stopLocked(); err != nil {
		audio.pause()
		source.Close()
		return err
	}
	p.stream, p.audio, p.paused = source, audio, false
	audio.play()
	return nil
}

func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.audio == nil {
		return ErrNoTrack
	}
	p.audio.pause()
	p.paused = true
	return p.audio.err()
}

func (p *Player) Resume() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.audio == nil {
		return ErrNoTrack
	}
	if err := p.audio.err(); err != nil {
		return err
	}
	if p.paused {
		p.audio.play()
		p.paused = false
	}
	return nil
}

// Stop es idempotente y descarga el archivo. Para reiniciar, vuelve a llamar Play.
func (p *Player) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.stopLocked()
}

func (p *Player) stopLocked() error {
	if p.audio == nil {
		return nil
	}
	// Espera a que termine cualquier Read antes de cerrar el archivo.
	p.audio.pause()
	err := errors.Join(p.audio.err(), p.stream.Close())
	p.audio, p.stream, p.paused = nil, nil, false
	return err
}

// Wait espera la pista actual, incluyendo pausas. La cancelacion detiene el audio.
// Al terminar deja un margen para el audio ya entregado al dispositivo de Windows.
func (p *Player) Wait(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		p.mu.Lock()
		if p.audio == nil {
			p.mu.Unlock()
			return nil
		}
		current := p.audio
		err := current.err()
		finished := !p.paused && !p.audio.playing()
		p.mu.Unlock()
		if err != nil {
			p.mu.Lock()
			if p.audio == current {
				err = errors.Join(err, p.stopLocked())
			}
			p.mu.Unlock()
			return err
		}
		if finished {
			timer := time.NewTimer(2 * outputBuffer)
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.Join(ctx.Err(), p.Stop())
			case <-timer.C:
				p.mu.Lock()
				if p.audio == current && !p.paused && !p.audio.playing() {
					err := p.stopLocked()
					p.mu.Unlock()
					return err
				}
				p.mu.Unlock()
			}
		}
		select {
		case <-ctx.Done():
			return errors.Join(ctx.Err(), p.Stop())
		case <-ticker.C:
		}
	}
}
