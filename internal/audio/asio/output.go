// Package asio implements a Windows ASIO backend for multichannel PCM.
package asio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"music-engine/internal/audio"
)

const (
	maxChannels       = 64
	defaultRingBlocks = 8
)

type Config struct {
	DriverName string
	SampleRate int
	BufferSize int
	Channels   int
	RingBlocks int
}

func (c Config) Validate() error {
	if c.DriverName == "" {
		return errors.New("ENGINE_ASIO_DRIVER es obligatorio para el backend ASIO")
	}
	if c.SampleRate != 0 && (c.SampleRate < 8000 || c.SampleRate > 384000) {
		return fmt.Errorf("sample rate ASIO fuera de rango: %d", c.SampleRate)
	}
	if c.BufferSize < 0 || c.BufferSize > 65536 {
		return fmt.Errorf("buffer size ASIO invalido: %d", c.BufferSize)
	}
	if c.Channels < 0 || c.Channels > maxChannels {
		return fmt.Errorf("cantidad de canales ASIO invalida: %d", c.Channels)
	}
	if c.RingBlocks != 0 && (c.RingBlocks < 2 || c.RingBlocks > 64) {
		return fmt.Errorf("ring blocks ASIO debe estar entre 2 y 64: %d", c.RingBlocks)
	}
	return nil
}

type device interface {
	Channels() (int, int)
	SampleRate() float64
	SetSampleRate(float64) error
	BufferSizes() (int, int, int, int)
	StartWithBuffer([]int, []int, int, Processor) error
	ResetRequested() bool
	Stop() error
	Close() error
}

type Output struct {
	mu     sync.Mutex
	config Config
	stream *stream
	closed bool
	open   func(string) (device, error)
}

func NewOutput(config Config) (*Output, error) {
	config.DriverName = strings.TrimSpace(config.DriverName)
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if config.RingBlocks == 0 {
		config.RingBlocks = defaultRingBlocks
	}
	return &Output{config: config, open: openASIODriver}, nil
}

func AvailableDrivers() ([]string, error) {
	return availableDriverNames()
}

func (o *Output) Open(source io.Reader, format audio.PCMFormat) (audio.Stream, error) {
	if source == nil || format.SampleFormat != audio.SignedInt16LE || format.SampleRate <= 0 || format.Channels < 1 || format.Channels > maxChannels {
		return nil, fmt.Errorf("formato PCM ASIO invalido: %+v", format)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil, errors.New("salida ASIO cerrada")
	}
	if o.stream != nil {
		return nil, errors.New("ASIO solo admite un stream de salida por proceso")
	}

	open := o.open
	if open == nil {
		open = openASIODriver
	}
	driver, err := open(o.config.DriverName)
	if err != nil {
		return nil, err
	}
	cleanup := func() { driver.Close() }
	_, availableChannels := driver.Channels()
	channels := o.config.Channels
	if channels == 0 {
		channels = format.Channels
	}
	if channels < format.Channels || channels > availableChannels {
		cleanup()
		return nil, fmt.Errorf("driver %q ofrece %d canales; PCM requiere %d y se solicitaron %d", o.config.DriverName, availableChannels, format.Channels, channels)
	}

	sampleRate := o.config.SampleRate
	if sampleRate == 0 {
		sampleRate = format.SampleRate
	}
	if err := driver.SetSampleRate(float64(sampleRate)); err != nil {
		cleanup()
		return nil, fmt.Errorf("configurar sample rate ASIO %d: %w", sampleRate, err)
	}
	actualRate := driver.SampleRate()
	if math.Abs(actualRate-float64(sampleRate)) > 0.5 {
		cleanup()
		return nil, fmt.Errorf("el driver ASIO quedo en %.0f Hz; se solicitaron %d Hz", actualRate, sampleRate)
	}

	minSize, maxSize, preferredSize, granularity := driver.BufferSizes()
	bufferSize := o.config.BufferSize
	if bufferSize == 0 {
		bufferSize = preferredSize
	}
	if !supportsBufferSize(bufferSize, minSize, maxSize, preferredSize, granularity) {
		cleanup()
		return nil, fmt.Errorf("buffer ASIO %d no admitido por %q (min=%d max=%d preferido=%d granularidad=%d)", bufferSize, o.config.DriverName, minSize, maxSize, preferredSize, granularity)
	}
	capacity := bufferSize * o.config.RingBlocks
	if capacity < bufferSize || capacity > 1<<20 || !validRingCapacity(capacity, channels) {
		cleanup()
		return nil, fmt.Errorf("capacidad de ring buffer ASIO invalida: %d frames", capacity)
	}

	ctx, cancel := context.WithCancel(context.Background())
	outputStream := &stream{
		driver:        driver,
		source:        source,
		inputChannels: format.Channels,
		channels:      channels,
		inputRate:     format.SampleRate,
		outputRate:    sampleRate,
		bufferSize:    bufferSize,
		ring:          newFrameRing(capacity, channels),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
	}
	outputStream.volume.Store(math.Float32bits(1))
	outputs := make([]int, channels)
	for i := range outputs {
		outputs[i] = i
	}
	go outputStream.produce()
	if err := driver.StartWithBuffer(nil, outputs, bufferSize, outputStream.callback); err != nil {
		cancel()
		<-outputStream.done
		cleanup()
		return nil, fmt.Errorf("iniciar driver ASIO %q: %w", o.config.DriverName, err)
	}
	o.stream = outputStream
	return outputStream, nil
}

func supportsBufferSize(requested, minSize, maxSize, preferred, granularity int) bool {
	if requested <= 0 || minSize <= 0 || maxSize < minSize || requested < minSize || requested > maxSize {
		return false
	}
	switch {
	case granularity == -1:
		return requested&(requested-1) == 0
	case granularity == 0:
		return requested == preferred
	case granularity > 0:
		return (requested-minSize)%granularity == 0
	default:
		return false
	}
}

func (o *Output) Close() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil
	}
	o.closed = true
	if o.stream == nil {
		return nil
	}
	closeErr := o.stream.close()
	o.stream = nil
	return closeErr
}

type stream struct {
	driver        device
	source        io.Reader
	inputChannels int
	channels      int
	inputRate     int
	outputRate    int
	bufferSize    int
	ring          *frameRing
	ctx           context.Context
	cancel        context.CancelFunc
	done          chan struct{}
	active        atomic.Bool
	volume        atomic.Uint32
	producerDone  atomic.Bool
	stateMu       sync.Mutex
	err           error
	closeOnce     sync.Once
	underruns     atomic.Uint64
}

func (s *stream) Play()  { s.active.Store(true) }
func (s *stream) Pause() { s.active.Store(false) }

func (s *stream) IsPlaying() bool {
	return s.active.Load() && (!s.producerDone.Load() || s.ring.Available() > 0)
}

func (s *stream) SetVolume(volume float64) {
	if math.IsNaN(volume) || volume < 0 {
		volume = 0
	} else if volume > 1 {
		volume = 1
	}
	s.volume.Store(math.Float32bits(float32(volume)))
}

func (s *stream) Err() error {
	if s.driver.ResetRequested() {
		s.setError(errors.New("el driver ASIO solicito reinicializacion; reinicia el Engine para recuperar la salida"))
	}
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	return s.err
}

func (s *stream) Underruns() uint64 { return s.underruns.Load() }

func (s *stream) callback(_ [][]float32, outputs [][]float32) {
	gain := math.Float32frombits(s.volume.Load())
	if !s.active.Load() {
		for _, channel := range outputs {
			clear(channel)
		}
		return
	}
	if s.ring.ReadPlanar(outputs, gain) {
		s.underruns.Add(1)
	}
}

func (s *stream) produce() {
	defer close(s.done)
	defer s.producerDone.Store(true)
	converter := newPCMConverter(s.source, s.inputChannels, s.channels, s.inputRate, s.outputRate, s.bufferSize)
	block := make([]int16, s.bufferSize*s.channels)
	pauseTicker := time.NewTicker(2 * time.Millisecond)
	defer pauseTicker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}
		if !s.active.Load() {
			select {
			case <-s.ctx.Done():
				return
			case <-pauseTicker.C:
				continue
			}
		}
		free := s.ring.Free()
		if free == 0 {
			select {
			case <-s.ctx.Done():
				return
			case <-pauseTicker.C:
				continue
			}
		}
		frames := min(free, s.bufferSize)
		produced := 0
		for produced < frames {
			if err := converter.ReadFrame(block[produced*s.channels : (produced+1)*s.channels]); err != nil {
				if !errors.Is(err, io.EOF) && s.ctx.Err() == nil {
					s.setError(fmt.Errorf("leer PCM para ASIO: %w", err))
				}
				if produced > 0 {
					s.ring.Write(block[:produced*s.channels])
				}
				return
			}
			produced++
		}
		if produced > 0 {
			written := s.ring.Write(block[:produced*s.channels])
			if written != produced {
				s.setError(errors.New("overflow del ring buffer ASIO; se descartaron frames PCM"))
			}
		}
	}
}

func (s *stream) setError(err error) {
	s.stateMu.Lock()
	if s.err == nil {
		s.err = err
	}
	s.stateMu.Unlock()
}

func (s *stream) close() error {
	var closeErr error
	s.closeOnce.Do(func() {
		s.active.Store(false)
		s.cancel()
		closeErr = s.driver.Stop()
		<-s.done
		closeErr = errors.Join(closeErr, s.driver.Close())
	})
	return closeErr
}

var _ audio.Output = (*Output)(nil)
var _ audio.Stream = (*stream)(nil)
