// Package player controla una reproduccion local; el backend Oto vive en oto.go.
package player

import (
	"context"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"music-engine/internal/decoder"
)

var ErrNoTrack = errors.New("no hay un archivo cargado; llama a Play primero")

// Player conserva el stream mientras Oto lo lee. Sus metodos admiten concurrencia.
type Player struct {
	mu        sync.Mutex
	stream    *decoder.Stream
	audio     *audioOutput
	paused    bool
	tracks    []string
	index     int
	current   string
	volume    float64
	lastError error
}

// New copia una lista opcional. El orden recibido define Next y Previous.
func New(paths ...string) *Player {
	tracks := append([]string(nil), paths...)
	return &Player{tracks: tracks, index: -1, volume: 1}
}

// Play abre el archivo e inicia audio en segundo plano. Wait permite esperar el final.
// Un archivo invalido no interrumpe la pista actual.
func (p *Player) Play(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playLocked(path)
}

func (p *Player) playLocked(path string) error {
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
	p.current, p.index, p.lastError = path, -1, nil
	for i, track := range p.tracks {
		a, _ := filepath.Abs(track)
		b, _ := filepath.Abs(path)
		if strings.EqualFold(a, b) {
			p.index = i
			break
		}
	}
	audio.setVolume(p.volume)
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

// State es una copia del estado actual; Track conserva la seleccion tras Stop.
type State struct {
	Status string
	Track  string
	Index  int // Base cero; -1 si la pista no pertenece a la lista.
	Total  int
	Volume float64 // 0 (silencio) a 1 (100%).
	Error  error
}

// GetState tambien detecta EOF y libera el archivo, sin avanzar automaticamente.
func (p *Player) GetState() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.audio != nil && (p.audio.err() != nil || (!p.paused && !p.audio.playing())) {
		p.lastError = p.stopLocked()
	}
	status := "STOPPED"
	if p.audio != nil {
		status = "PLAYING"
		if p.paused {
			status = "PAUSED"
		}
	}
	return State{Status: status, Track: p.current, Index: p.index, Total: len(p.tracks), Volume: p.volume, Error: p.lastError}
}

// SetVolume conserva la ganancia para pistas futuras, incluso estando detenido.
func (p *Player) SetVolume(volume float64) error {
	if math.IsNaN(volume) || math.IsInf(volume, 0) || volume < 0 || volume > 1 {
		return fmt.Errorf("el volumen debe estar entre 0 y 1")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volume = volume
	if p.audio != nil {
		p.audio.setVolume(volume)
	}
	return nil
}

func (p *Player) Next() error     { return p.move(1) }
func (p *Player) Previous() error { return p.move(-1) }

// La navegacion es circular e inicia la pista, incluso desde pausa o Stop.
func (p *Player) move(step int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.tracks) == 0 {
		return errors.New("la lista de MP3 esta vacia")
	}
	next := (p.index + step + len(p.tracks)) % len(p.tracks)
	if p.index == -1 {
		next = 0
		if step < 0 {
			next = len(p.tracks) - 1
		}
	}
	if err := p.playLocked(p.tracks[next]); err != nil {
		return err
	}
	p.index = next
	return nil
}
