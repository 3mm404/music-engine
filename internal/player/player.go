// Package player controls a local playback stream.
package player

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"music-engine/internal/audio"
	otooutput "music-engine/internal/audio/oto"
	"music-engine/internal/decoder"
)

var ErrNoTrack = errors.New("no hay un archivo cargado; llama a Play primero")

const playbackCompletionGrace = 100 * time.Millisecond

// Player conserva el stream mientras la salida de audio lo consume. Sus metodos admiten concurrencia.
type Player struct {
	mu           sync.Mutex
	stream       *decoder.Stream
	output       audio.Output
	audio        audio.Stream
	paused       bool
	tracks       []string
	index        int
	current      string
	volume       float64
	lastError    error
	stopReason   string
	remote       decoder.HTTPS
	cancel       context.CancelFunc
	generation   uint64
	loading      string
	jobs         sync.WaitGroup
	expectedRate int
	mono         bool
	progress     *pcmProgress
	positionMS   *int
	durationMS   *int
}

// NewRemote fixes the rate promised in configuration and permits a trusted HTTP transport.
func NewRemote(rate int, source decoder.HTTPS) *Player {
	return NewRemoteWithOutput(otooutput.NewOutput(), rate, source)
}

func NewRemoteWithOutput(output audio.Output, rate int, source decoder.HTTPS) *Player {
	p := NewWithOutput(output)
	p.expectedRate, p.remote = rate, source
	return p
}

func NewRemoteMono(rate int, source decoder.HTTPS) *Player {
	return NewRemoteMonoWithOutput(otooutput.NewOutput(), rate, source)
}

func NewRemoteMonoWithOutput(output audio.Output, rate int, source decoder.HTTPS) *Player {
	p := NewRemoteWithOutput(output, rate, source)
	p.mono = true
	return p
}

// New copia una lista opcional. El orden recibido define Next y Previous.
func New(paths ...string) *Player {
	return NewWithOutput(otooutput.NewOutput(), paths...)
}

func NewWithOutput(output audio.Output, paths ...string) *Player {
	tracks := append([]string(nil), paths...)
	return &Player{output: output, tracks: tracks, index: -1, volume: 1}
}

// Play abre el archivo e inicia audio en segundo plano. Wait permite esperar el final.
// Un archivo invalido no interrumpe la pista actual.
func (p *Player) Play(path string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playLocked(path)
}

func (p *Player) PlayVerified(path, hash string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !decoder.ValidURL(path) {
		return errors.New("audio requiere HTTPS")
	}
	return p.startRemoteLocked(path, hash)
}

func (p *Player) playLocked(path string) error {
	if path == "" {
		path = p.current
	}
	if path == "" {
		return ErrNoTrack
	}
	if strings.Contains(path, "://") {
		if !decoder.ValidURL(path) {
			return errors.New("audio requiere HTTPS")
		}
		return p.startRemoteLocked(path, "")
	}
	p.cancelLoadLocked()
	source, err := decoder.Open(path)
	if err != nil {
		return err
	}
	return p.installLocked(path, source)
}

func (p *Player) installLocked(path string, source *decoder.Stream) error {
	if p.expectedRate != 0 && source.SampleRate() != p.expectedRate {
		source.Close()
		return errors.New("frecuencia MP3 distinta de la configuracion")
	}
	var pcm io.Reader = source
	if p.mono {
		pcm = &monoReader{source: source}
	}
	progress := &pcmProgress{rate: source.SampleRate(), length: source.Length()}
	pcm = &progressReader{source: pcm, progress: progress}
	stream, err := p.output.Open(pcm, audio.PCMFormat{
		SampleRate:   source.SampleRate(),
		Channels:     2,
		SampleFormat: audio.SignedInt16LE,
	})
	if err != nil {
		source.Close()
		return err
	}
	if err := p.stopLocked(); err != nil {
		stream.Pause()
		source.Close()
		return err
	}
	p.stream, p.audio, p.paused = source, stream, false
	p.progress = progress
	p.positionMS, p.durationMS = progress.times()
	p.current, p.index, p.lastError = path, -1, nil
	p.stopReason = ""
	for i, track := range p.tracks {
		if track == path {
			p.index = i
			break
		}
		if strings.Contains(path, "://") {
			continue
		}
		a, _ := filepath.Abs(track)
		b, _ := filepath.Abs(path)
		if strings.EqualFold(a, b) {
			p.index = i
			break
		}
	}
	stream.SetVolume(p.volume)
	stream.Play()
	return nil
}

func (p *Player) cancelLoadLocked() {
	p.generation++
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
	// Workers never need p.mu to finish downloading. Waiting prevents overlapping
	// allocations when a caller rapidly switches songs.
	p.jobs.Wait()
	p.loading = ""
}

// startRemoteLocked inicia una carga cancelable. La generación evita instalar audio obsoleto después de Stop o de otra selección; requiere p.mu.
func (p *Player) startRemoteLocked(path, hash string) error {
	p.cancelLoadLocked()
	if err := p.stopLocked(); err != nil {
		return err
	}
	p.progress, p.positionMS, p.durationMS = nil, nil, nil
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	generation := p.generation
	p.current, p.index, p.lastError, p.loading = path, -1, nil, "LOADING"
	p.stopReason = ""
	for i, track := range p.tracks {
		if path == track {
			p.index = i
			break
		}
	}
	// Status callbacks and installation use separate events; the download worker
	// can always exit during cancellation even while the player lock is held.
	events := make(chan string, 3)
	type result struct {
		stream *decoder.Stream
		err    error
	}
	results := make(chan result)
	p.jobs.Add(1)
	sourceOptions := p.remote
	sourceOptions.SHA256 = hash
	go func() {
		defer p.jobs.Done()
		source, err := sourceOptions.Open(ctx, path, func() { events <- "RECOVERING" })
		select {
		case results <- result{source, err}:
		case <-ctx.Done():
			if source != nil {
				source.Close()
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case status := <-events:
				p.mu.Lock()
				if generation == p.generation {
					p.loading = status
				}
				p.mu.Unlock()
			case r := <-results:
				p.mu.Lock()
				defer p.mu.Unlock()
				if generation != p.generation {
					if r.stream != nil {
						r.stream.Close()
					}
					return
				}
				p.loading = ""
				if r.err == nil {
					r.err = p.installLocked(path, r.stream)
				}
				p.lastError = r.err
				cancel()
				return
			}
		}
	}()
	return nil
}

func (p *Player) Pause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.audio == nil {
		return ErrNoTrack
	}
	p.audio.Pause()
	p.paused = true
	return p.audio.Err()
}

func (p *Player) Resume() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.audio == nil {
		return ErrNoTrack
	}
	if err := p.audio.Err(); err != nil {
		return err
	}
	if p.paused {
		p.audio.Play()
		p.paused = false
	}
	return nil
}

// SeekMS rebuilds only this zone's output to discard buffered PCM and preserves pause/volume.
func (p *Player) SeekMS(position int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.audio == nil || p.stream == nil || p.loading != "" {
		return ErrNoTrack
	}
	length, rate := p.stream.Length(), p.stream.SampleRate()
	if position < 0 || length <= 0 || int64(position) > length*1000/(int64(rate)*4) {
		return errors.New("posicion fuera de la duracion de la cancion")
	}
	paused := p.paused
	p.audio.Pause()
	if closer, ok := p.audio.(interface{ Close() error }); ok {
		if err := closer.Close(); err != nil {
			return err
		}
	}
	p.audio = nil
	installed := false
	defer func() {
		if !installed {
			p.stream.Close()
			p.stream, p.progress, p.paused = nil, nil, false
		}
	}()
	offset := int64(position) * int64(rate) * 4 / 1000
	offset -= offset % 4
	if offset >= length {
		offset = length - 4
	}
	actual, err := p.stream.Seek(offset, io.SeekStart)
	if err != nil {
		p.lastError = err
		return err
	}
	var pcm io.Reader = p.stream
	if p.mono {
		pcm = &monoReader{source: p.stream}
	}
	progress := &pcmProgress{rate: rate, length: length}
	progress.bytes.Store(actual)
	stream, err := p.output.Open(&progressReader{source: pcm, progress: progress}, audio.PCMFormat{SampleRate: rate, Channels: 2, SampleFormat: audio.SignedInt16LE})
	if err != nil {
		p.lastError = err
		return err
	}
	p.audio, p.progress, p.paused, p.lastError = stream, progress, paused, nil
	installed = true
	p.positionMS, p.durationMS = progress.times()
	stream.SetVolume(p.volume)
	if !paused {
		stream.Play()
	}
	return nil
}

// Stop es idempotente y descarga el archivo. Para reiniciar, vuelve a llamar Play.
func (p *Player) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancelLoadLocked()
	p.lastError = nil
	p.stopReason = "manual"
	err := p.stopLocked()
	if p.positionMS != nil {
		zero := 0
		p.positionMS = &zero
	}
	return err
}

// stopLocked cancela la carga y libera stream/salida de la pista; requiere p.mu. No cierra el backend compartido de otras zonas.
func (p *Player) stopLocked() error {
	if p.audio == nil {
		return nil
	}
	// Espera a que termine cualquier Read antes de cerrar el archivo.
	p.audio.Pause()
	if p.progress != nil {
		p.positionMS, p.durationMS = p.progress.times()
		p.progress = nil
	}
	err := errors.Join(p.audio.Err(), p.stream.Close())
	if closer, ok := p.audio.(interface{ Close() error }); ok {
		err = errors.Join(err, closer.Close())
	}
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
		if p.audio == nil && p.loading == "" {
			err := p.lastError
			p.mu.Unlock()
			return err
		}
		if p.audio == nil {
			p.mu.Unlock()
			select {
			case <-ctx.Done():
				return errors.Join(ctx.Err(), p.Stop())
			case <-ticker.C:
			}
			continue
		}
		current := p.audio
		err := current.Err()
		finished := !p.paused && !p.audio.IsPlaying()
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
			timer := time.NewTimer(playbackCompletionGrace)
			select {
			case <-ctx.Done():
				timer.Stop()
				return errors.Join(ctx.Err(), p.Stop())
			case <-timer.C:
				p.mu.Lock()
				if p.audio == current && !p.paused && !p.audio.IsPlaying() {
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
	Status     string
	Track      string
	Index      int // Base cero; -1 si la pista no pertenece a la lista.
	Total      int
	Volume     float64 // 0 (silencio) a 1 (100%).
	Error      error
	PositionMS *int
	DurationMS *int
	StopReason string
}

// GetState tambien detecta EOF y libera el archivo, sin avanzar automaticamente.
func (p *Player) GetState() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.audio != nil && (p.audio.Err() != nil || (!p.paused && !p.audio.IsPlaying())) {
		p.lastError = p.stopLocked()
		p.stopReason = "ended"
		if p.lastError != nil {
			p.stopReason = "error"
		}
	}
	status := "STOPPED"
	if p.lastError != nil {
		status = "ERROR"
	}
	if p.loading != "" {
		status = p.loading
	}
	if p.audio != nil {
		status = "PLAYING"
		if p.paused {
			status = "PAUSED"
		}
	}
	track := p.current
	if strings.Contains(track, "://") {
		if u, err := url.Parse(track); err == nil {
			u.RawQuery = ""
			track = u.String()
		}
	}
	position, duration := p.positionMS, p.durationMS
	if p.progress != nil {
		position, duration = p.progress.times()
	}
	return State{Status: status, Track: track, Index: p.index, Total: len(p.tracks), Volume: p.volume, Error: p.lastError, PositionMS: position, DurationMS: duration, StopReason: p.stopReason}
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
		p.audio.SetVolume(volume)
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
