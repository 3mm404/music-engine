// Package asio implements a Windows ASIO backend for multichannel PCM.
package asio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
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

type Processor func(in, out [][]float32)

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
	CanSampleRate(float64) error
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

func (o *Output) Open(source io.Reader, format audio.PCMFormat) (opened audio.Stream, openErr error) {
	defer func() {
		if openErr != nil {
			log.Printf("ASIO driver initialized: no; driver=%q error=%v", o.config.DriverName, openErr)
		}
	}()
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
	inputChannels, availableChannels := driver.Channels()
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
	if err := driver.CanSampleRate(float64(sampleRate)); err != nil {
		cleanup()
		return nil, fmt.Errorf("driver ASIO %q no soporta %d Hz: %w", o.config.DriverName, sampleRate, err)
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
		driverName:    o.config.DriverName,
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
	log.Printf("Audio backend: ASIO")
	log.Printf("ASIO driver: %s", o.config.DriverName)
	log.Printf("Sample rate: %d Hz (input PCM %d Hz)", sampleRate, format.SampleRate)
	log.Printf("Buffer size: %d frames", bufferSize)
	log.Printf("Output channels: %d opened (%d available); input channels: %d", channels, availableChannels, inputChannels)
	log.Printf("ASIO device output indices (zero-based): %v", outputs)
	log.Printf("PCM delivered to asio.Output: sample=int16le interleaved=true input_channels=%d", format.Channels)
	log.Printf("Driver initialized: yes")
	go outputStream.report()
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
	driver         device
	driverName     string
	source         io.Reader
	inputChannels  int
	channels       int
	inputRate      int
	outputRate     int
	bufferSize     int
	ring           *frameRing
	ctx            context.Context
	cancel         context.CancelFunc
	done           chan struct{}
	active         atomic.Bool
	volume         atomic.Uint32
	producerDone   atomic.Bool
	stateMu        sync.Mutex
	err            error
	closeOnce      sync.Once
	underruns      atomic.Uint64
	callbackCalls  atomic.Uint64
	callbackFrames atomic.Uint64
	consumedFrames atomic.Uint64
	producerFrames atomic.Uint64
	nonzeroSamples [maxChannels]atomic.Uint64
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
	frames := 0
	for _, channel := range outputs {
		frames = max(frames, len(channel))
	}
	s.callbackCalls.Add(1)
	s.callbackFrames.Add(uint64(frames))
	gain := math.Float32frombits(s.volume.Load())
	if !s.active.Load() {
		for _, channel := range outputs {
			clear(channel)
		}
		return
	}
	consumed, underrun := s.ring.ReadPlanarCount(outputs, gain)
	s.consumedFrames.Add(uint64(consumed))
	for channel, samples := range outputs {
		var nonzero uint64
		for _, sample := range samples {
			if sample != 0 {
				nonzero++
			}
		}
		s.nonzeroSamples[channel].Add(nonzero)
	}
	if underrun {
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
					written := s.ring.Write(block[:produced*s.channels])
					s.producerFrames.Add(uint64(written))
					if written != produced {
						s.setError(errors.New("overflow del ring buffer ASIO; se descartaron frames PCM"))
					}
				}
				return
			}
			produced++
		}
		if produced > 0 {
			written := s.ring.Write(block[:produced*s.channels])
			s.producerFrames.Add(uint64(written))
			if written != produced {
				s.setError(errors.New("overflow del ring buffer ASIO; se descartaron frames PCM"))
			}
		}
	}
}

type StreamDiagnostics struct {
	Driver         string
	InputRate      int
	OutputRate     int
	InputChannels  int
	OutputChannels int
	BufferSize     int
	ProducerFrames uint64
	CallbackCalls  uint64
	CallbackFrames uint64
	ConsumedFrames uint64
	UnderrunCalls  uint64
	OverflowFrames uint64
	BufferedFrames int
	NonzeroSamples []uint64
}

func (s *stream) Diagnostics() StreamDiagnostics {
	nonzero := make([]uint64, s.channels)
	for channel := range nonzero {
		nonzero[channel] = s.nonzeroSamples[channel].Load()
	}
	return StreamDiagnostics{
		Driver: s.driverName, InputRate: s.inputRate, OutputRate: s.outputRate,
		InputChannels: s.inputChannels, OutputChannels: s.channels, BufferSize: s.bufferSize,
		ProducerFrames: s.producerFrames.Load(), CallbackCalls: s.callbackCalls.Load(),
		CallbackFrames: s.callbackFrames.Load(), ConsumedFrames: s.consumedFrames.Load(),
		UnderrunCalls: s.underruns.Load(), OverflowFrames: s.ring.OverflowFrames(),
		BufferedFrames: s.ring.Available(),
		NonzeroSamples: nonzero,
	}
}

func (s *stream) report() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			d := s.Diagnostics()
			log.Printf("ASIO activity: driver=%q input_rate=%d output_rate=%d input_channels=%d output_channels=%d buffer_size=%d pcm_producer_frames=%d callback_calls=%d callback_frames=%d pcm_frames_consumed=%d underrun_callbacks=%d overflow_frames=%d ring_buffered_frames=%d nonzero_samples_per_channel=%v",
				d.Driver, d.InputRate, d.OutputRate, d.InputChannels, d.OutputChannels, d.BufferSize,
				d.ProducerFrames, d.CallbackCalls, d.CallbackFrames, d.ConsumedFrames,
				d.UnderrunCalls, d.OverflowFrames, d.BufferedFrames, d.NonzeroSamples)
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
