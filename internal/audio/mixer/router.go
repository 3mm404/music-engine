// Package mixer routes independent PCM zone streams into one interleaved output.
package mixer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"sync"

	"music-engine/internal/audio"
)

const maxOutputChannels = 64

type Route struct {
	ZoneID   string
	Channels []int
}

type Router struct {
	backend      audio.Output
	backendMu    sync.Mutex
	controlMu    sync.Mutex
	readMu       sync.Mutex
	mu           sync.Mutex
	routes       map[string][]int
	feeds        map[string]*feed
	maxChannels  int
	routed       bool
	directOpened bool
	closed       bool
	output       audio.Stream
	format       audio.PCMFormat
	pending      []byte
	pendingAt    int
}

type feed struct {
	zoneID   string
	source   io.Reader
	format   audio.PCMFormat
	frame    [4]byte
	playing  bool
	finished bool
	volume   float64
	err      error
}

type zoneOutput struct {
	router *Router
	zoneID string
}

type zoneStream struct {
	router *Router
	feed   *feed
}

func New(output audio.Output) *Router {
	return &Router{backend: output, routes: map[string][]int{}, feeds: map[string]*feed{}}
}

func (r *Router) OutputForZone(zoneID string) audio.Output {
	return &zoneOutput{router: r, zoneID: zoneID}
}

// Configure replaces all routes atomically. Channels are one-based, ordered,
// and exclusive; one channel downmixes stereo while two preserve L/R.
func (r *Router) Configure(routes []Route) error {
	configured := make(map[string][]int, len(routes))
	owners := make(map[int]string)
	maxChannels := 0
	for _, route := range routes {
		if route.ZoneID == "" || len(route.Channels) < 1 || len(route.Channels) > 2 {
			return fmt.Errorf("ruta invalida para zona %q: se requiere uno o dos canales", route.ZoneID)
		}
		if _, exists := configured[route.ZoneID]; exists {
			return fmt.Errorf("ruta duplicada para zona %q", route.ZoneID)
		}
		channels := append([]int(nil), route.Channels...)
		for _, channel := range channels {
			if channel < 1 || channel > maxOutputChannels {
				return fmt.Errorf("canal %d fuera del rango 1-%d", channel, maxOutputChannels)
			}
			if owner, exists := owners[channel]; exists {
				return fmt.Errorf("canal %d asignado a las zonas %q y %q", channel, owner, route.ZoneID)
			}
			owners[channel] = route.ZoneID
			if channel > maxChannels {
				maxChannels = channel
			}
		}
		configured[route.ZoneID] = channels
	}

	r.backendMu.Lock()
	defer r.backendMu.Unlock()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("router de audio cerrado")
	}
	if len(configured) > 0 && r.directOpened {
		return errors.New("no se puede activar routing despues de abrir una salida directa")
	}
	if r.output != nil {
		if len(configured) > 0 && maxChannels != r.format.Channels {
			return errors.New("no se puede cambiar la cantidad de canales despues de abrir el backend")
		}
		if len(configured) == 0 {
			maxChannels = r.format.Channels
		}
	}
	r.routes = configured
	r.maxChannels = maxChannels
	r.routed = len(configured) > 0
	return nil
}

func (o *zoneOutput) Open(source io.Reader, format audio.PCMFormat) (audio.Stream, error) {
	if source == nil || format.SampleRate <= 0 || (format.Channels != 1 && format.Channels != 2) || format.SampleFormat != audio.SignedInt16LE {
		return nil, fmt.Errorf("formato PCM de zona invalido: %+v", format)
	}
	r := o.router
	r.backendMu.Lock()
	defer r.backendMu.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, errors.New("router de audio cerrado")
	}
	if !r.routed {
		r.directOpened = true
		backend := r.backend
		r.mu.Unlock()
		if backend == nil {
			return nil, errors.New("backend de audio no configurado")
		}
		return backend.Open(source, format)
	}
	_, ok := r.routes[o.zoneID]
	if !ok {
		r.mu.Unlock()
		return nil, fmt.Errorf("zona %q sin canales de salida", o.zoneID)
	}
	input := &feed{zoneID: o.zoneID, source: source, format: format, volume: 1}
	if previous := r.feeds[o.zoneID]; previous != nil {
		previous.playing = false
		previous.finished = true
	}
	r.feeds[o.zoneID] = input
	outputFormat := audio.PCMFormat{SampleRate: format.SampleRate, Channels: r.maxChannels, SampleFormat: format.SampleFormat}
	if r.output != nil {
		if r.format.SampleRate != outputFormat.SampleRate || r.format.SampleFormat != outputFormat.SampleFormat {
			r.mu.Unlock()
			return nil, fmt.Errorf("formato PCM incompatible con el backend activo: requerido %+v, activo %+v", outputFormat, r.format)
		}
		r.mu.Unlock()
		return &zoneStream{router: r, feed: input}, nil
	}
	r.pending = make([]byte, outputFormat.Channels*2)
	r.pendingAt = len(r.pending)
	r.format = outputFormat
	r.mu.Unlock()
	if r.backend == nil {
		r.mu.Lock()
		if r.feeds[o.zoneID] == input {
			delete(r.feeds, o.zoneID)
		}
		r.mu.Unlock()
		return nil, errors.New("backend de audio no configurado")
	}
	output, err := r.backend.Open(r, outputFormat)
	if err != nil {
		r.mu.Lock()
		if r.feeds[o.zoneID] == input {
			delete(r.feeds, o.zoneID)
		}
		r.pending = nil
		r.format = audio.PCMFormat{}
		r.mu.Unlock()
		return nil, err
	}
	r.mu.Lock()
	r.output = output
	r.mu.Unlock()
	return &zoneStream{router: r, feed: input}, nil
}

func (s *zoneStream) Play() {
	r := s.router
	r.controlMu.Lock()
	defer r.controlMu.Unlock()
	r.mu.Lock()
	if r.closed || s.feed.finished {
		r.mu.Unlock()
		return
	}
	s.feed.playing = true
	output := r.output
	r.mu.Unlock()
	if output != nil {
		output.Play()
	}
}

func (s *zoneStream) Pause() {
	r := s.router
	r.mu.Lock()
	s.feed.playing = false
	r.mu.Unlock()
}

func (s *zoneStream) IsPlaying() bool {
	r := s.router
	r.mu.Lock()
	defer r.mu.Unlock()
	return !r.closed && s.feed.playing && !s.feed.finished
}

func (s *zoneStream) Err() error {
	r := s.router
	r.mu.Lock()
	feedErr, output := s.feed.err, r.output
	r.mu.Unlock()
	if output != nil {
		return errors.Join(feedErr, output.Err())
	}
	return feedErr
}

func (s *zoneStream) SetVolume(volume float64) {
	if math.IsNaN(volume) || volume < 0 {
		volume = 0
	} else if volume > 1 {
		volume = 1
	}
	r := s.router
	r.mu.Lock()
	s.feed.volume = volume
	r.mu.Unlock()
}

// Read keeps the backend stream continuous, writing silence on unrouted or
// inactive channels and never propagating one zone's EOF to other zones.
func (r *Router) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	r.readMu.Lock()
	defer r.readMu.Unlock()
	r.mu.Lock()
	if r.closed || len(r.pending) == 0 {
		r.mu.Unlock()
		return 0, errors.New("router PCM no inicializado")
	}
	r.mu.Unlock()
	total := 0
	for total < len(p) {
		if r.pendingAt == len(r.pending) {
			r.renderFrame()
			r.pendingAt = 0
		}
		copied := copy(p[total:], r.pending[r.pendingAt:])
		total += copied
		r.pendingAt += copied
	}
	return total, nil
}

func (r *Router) renderFrame() {
	r.mu.Lock()
	defer r.mu.Unlock()
	clear(r.pending)
	for zoneID, input := range r.feeds {
		if !input.playing || input.finished {
			continue
		}
		channels := r.routes[zoneID]
		if len(channels) == 0 {
			continue
		}
		size := input.format.Channels * 2
		if _, err := io.ReadFull(input.source, input.frame[:size]); err != nil {
			input.playing, input.finished = false, true
			if !errors.Is(err, io.EOF) {
				input.err = err
			}
			continue
		}
		left := int16(binary.LittleEndian.Uint16(input.frame[:2]))
		right := left
		if input.format.Channels == 2 {
			right = int16(binary.LittleEndian.Uint16(input.frame[2:4]))
		}
		if len(channels) == 1 {
			left = int16((int32(left) + int32(right)) / 2)
			writeSample(r.pending, channels[0], scale(left, input.volume))
		} else {
			writeSample(r.pending, channels[0], scale(left, input.volume))
			writeSample(r.pending, channels[1], scale(right, input.volume))
		}
	}
}

func writeSample(frame []byte, channel int, value int16) {
	binary.LittleEndian.PutUint16(frame[(channel-1)*2:], uint16(value))
}

func scale(sample int16, volume float64) int16 {
	value := math.Round(float64(sample) * volume)
	if value > math.MaxInt16 {
		value = math.MaxInt16
	} else if value < math.MinInt16 {
		value = math.MinInt16
	}
	return int16(value)
}

func (r *Router) Close() error {
	r.backendMu.Lock()
	defer r.backendMu.Unlock()
	r.controlMu.Lock()
	defer r.controlMu.Unlock()
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	for _, input := range r.feeds {
		input.playing = false
	}
	output := r.output
	r.mu.Unlock()
	if output == nil {
		return nil
	}
	output.Pause()
	return output.Err()
}
