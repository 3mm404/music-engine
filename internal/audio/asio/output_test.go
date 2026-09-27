package asio

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"music-engine/internal/audio"
)

type fakeDevice struct {
	mu          sync.Mutex
	inputCount  int
	outputCount int
	rate        float64
	minBuffer   int
	maxBuffer   int
	preferred   int
	granularity int
	callback    Processor
	opened      bool
	stopped     bool
	closed      bool
	startedSize int
	startedOut  []int
	startErr    error
	stopErr     error
	reset       bool
}

func (d *fakeDevice) Channels() (int, int) { return d.inputCount, d.outputCount }
func (d *fakeDevice) SampleRate() float64  { return d.rate }
func (d *fakeDevice) SetSampleRate(rate float64) error {
	d.rate = rate
	return nil
}
func (d *fakeDevice) BufferSizes() (int, int, int, int) {
	return d.minBuffer, d.maxBuffer, d.preferred, d.granularity
}
func (d *fakeDevice) StartWithBuffer(_ []int, output []int, size int, callback Processor) error {
	if d.startErr != nil {
		return d.startErr
	}
	d.callback = callback
	d.startedSize = size
	d.startedOut = append([]int(nil), output...)
	d.opened = true
	return nil
}
func (d *fakeDevice) ResetRequested() bool { return d.reset }
func (d *fakeDevice) Stop() error {
	d.stopped = true
	return d.stopErr
}
func (d *fakeDevice) Close() error {
	d.closed = true
	return nil
}

func TestOutputValidatesDeviceChannelsAndStartsOneMultichannelStream(t *testing.T) {
	device := newFakeDevice(8)
	output := testOutput(device, Config{DriverName: "test", SampleRate: 44100, BufferSize: 8, Channels: 4, RingBlocks: 2})
	streamHandle, err := output.Open(testPCM(100, 200, 300, 400, 500, 600, 700, 800), audio.PCMFormat{
		SampleRate: 44100, Channels: 2, SampleFormat: audio.SignedInt16LE,
	})
	if err != nil {
		t.Fatal(err)
	}
	if device.startedSize != 8 || len(device.startedOut) != 4 || device.startedOut[3] != 3 {
		t.Fatalf("ASIO stream configuration: buffer=%d outputs=%v", device.startedSize, device.startedOut)
	}
	streamHandle.Play()
	waitForRing(t, streamHandle.(*stream), 4)
	channels := make([][]float32, 4)
	for i := range channels {
		channels[i] = make([]float32, 4)
	}
	device.callback(nil, channels)
	want := [][]float32{{100.0 / 32768, 300.0 / 32768, 500.0 / 32768, 700.0 / 32768}, {200.0 / 32768, 400.0 / 32768, 600.0 / 32768, 800.0 / 32768}, make([]float32, 4), make([]float32, 4)}
	for channel := range want {
		for frame := range want[channel] {
			if channels[channel][frame] != want[channel][frame] {
				t.Fatalf("ASIO plane %d = %v, want %v", channel, channels[channel], want[channel])
			}
		}
	}
	if streamHandle.IsPlaying() {
		t.Fatal("finite source should be stopped after queued frames drain")
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
	if !device.stopped || !device.closed {
		t.Fatalf("device lifecycle stopped=%v closed=%v", device.stopped, device.closed)
	}
	if err := output.Close(); err != nil {
		t.Fatalf("second close: %v", err)
	}
}

func TestListInstalledASIODrivers(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("ASIO host is Windows amd64 only")
	}
	names, err := AvailableDrivers()
	if err != nil {
		t.Logf("no ASIO drivers registered: %v", err)
		return
	}
	t.Logf("registered ASIO drivers: %v", names)
}

// Set MUSIC_ENGINE_ASIO_TEST_DRIVER to initialize a real driver. This test
// does not start the audio callback or write sound to the device.
func TestOpenInstalledASIODriverOptional(t *testing.T) {
	if runtime.GOOS != "windows" || runtime.GOARCH != "amd64" {
		t.Skip("ASIO host is Windows amd64 only")
	}
	name := os.Getenv("MUSIC_ENGINE_ASIO_TEST_DRIVER")
	if name == "" {
		t.Skip("set MUSIC_ENGINE_ASIO_TEST_DRIVER to initialize a real ASIO driver")
	}
	driver, err := openASIODriver(name)
	if err != nil {
		t.Fatal(err)
	}
	defer driver.Close()
	_, channels := driver.Channels()
	minSize, maxSize, preferred, granularity := driver.BufferSizes()
	t.Logf("opened ASIO driver %q: %.0f Hz, %d outputs, buffers %d-%d preferred %d granularity %d", name, driver.SampleRate(), channels, minSize, maxSize, preferred, granularity)
	if channels < 1 || minSize < 1 || maxSize < minSize {
		t.Fatalf("driver reported invalid capabilities: channels=%d buffers=%d-%d", channels, minSize, maxSize)
	}
}

func TestOutputRejectsInsufficientChannelsAndUnsupportedBuffer(t *testing.T) {
	device := newFakeDevice(2)
	output := testOutput(device, Config{DriverName: "test", BufferSize: 10, Channels: 2, RingBlocks: 2})
	_, err := output.Open(testPCM(1, 2, 3, 4), audio.PCMFormat{SampleRate: 44100, Channels: 2, SampleFormat: audio.SignedInt16LE})
	if err == nil {
		t.Fatal("unsupported buffer size accepted")
	}
	if !device.closed {
		t.Fatal("driver not closed after configuration failure")
	}

	device = newFakeDevice(2)
	output = testOutput(device, Config{DriverName: "test", BufferSize: 8, Channels: 4, RingBlocks: 2})
	_, err = output.Open(testPCM(1, 2), audio.PCMFormat{SampleRate: 44100, Channels: 2, SampleFormat: audio.SignedInt16LE})
	if err == nil {
		t.Fatal("requested channels beyond device capacity accepted")
	}
	if !device.closed {
		t.Fatal("driver not closed after channel validation failure")
	}
}

func TestOutputReportsStartupAndShutdownErrors(t *testing.T) {
	startErr := errors.New("start failure")
	device := newFakeDevice(2)
	device.startErr = startErr
	output := testOutput(device, Config{DriverName: "test", BufferSize: 8, Channels: 2, RingBlocks: 2})
	_, err := output.Open(testPCM(1, 2), audio.PCMFormat{SampleRate: 44100, Channels: 2, SampleFormat: audio.SignedInt16LE})
	if !errors.Is(err, startErr) || !device.closed {
		t.Fatalf("startup error=%v closed=%v", err, device.closed)
	}

	device = newFakeDevice(2)
	device.stopErr = errors.New("stop failure")
	output = testOutput(device, Config{DriverName: "test", BufferSize: 8, Channels: 2, RingBlocks: 2})
	if _, err = output.Open(testPCM(1, 2, 3, 4), audio.PCMFormat{SampleRate: 44100, Channels: 2, SampleFormat: audio.SignedInt16LE}); err != nil {
		t.Fatal(err)
	}
	if err = output.Close(); err == nil {
		t.Fatal("shutdown error was swallowed")
	}
}

func TestConfigRejectsInvalidValues(t *testing.T) {
	for _, config := range []Config{
		{},
		{DriverName: "test", SampleRate: 1000},
		{DriverName: "test", BufferSize: -1},
		{DriverName: "test", Channels: maxChannels + 1},
		{DriverName: "test", RingBlocks: 1},
	} {
		if _, err := NewOutput(config); err == nil {
			t.Fatalf("accepted config %+v", config)
		}
	}
}

func TestBufferSizeValidation(t *testing.T) {
	if !supportsBufferSize(256, 64, 512, 256, -1) || supportsBufferSize(192, 64, 512, 256, -1) {
		t.Fatal("power-of-two granularity validation failed")
	}
	if !supportsBufferSize(256, 128, 512, 256, 0) || supportsBufferSize(128, 128, 512, 256, 0) {
		t.Fatal("fixed-buffer validation failed")
	}
	if !supportsBufferSize(192, 64, 512, 128, 64) || supportsBufferSize(200, 64, 512, 128, 64) {
		t.Fatal("step granularity validation failed")
	}
}

func TestCallbackUnderrunWritesSilence(t *testing.T) {
	device := newFakeDevice(2)
	output := testOutput(device, Config{DriverName: "test", BufferSize: 8, Channels: 2, RingBlocks: 2})
	streamHandle, err := output.Open(testPCM(10, 20), audio.PCMFormat{SampleRate: 44100, Channels: 2, SampleFormat: audio.SignedInt16LE})
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	streamHandle.Play()
	waitForRing(t, streamHandle.(*stream), 1)
	device.callback(nil, [][]float32{make([]float32, 2), make([]float32, 2)})
	channels := [][]float32{make([]float32, 2), make([]float32, 2)}
	device.callback(nil, channels)
	if channels[0][0] != 0 || channels[0][1] != 0 || channels[1][0] != 0 || channels[1][1] != 0 {
		t.Fatalf("underrun must output silence, got %v", channels)
	}
	if streamHandle.(*stream).Underruns() == 0 {
		t.Fatal("underrun was not counted")
	}
}

func testOutput(fake *fakeDevice, config Config) *Output {
	output, err := NewOutput(config)
	if err != nil {
		panic(err)
	}
	output.open = func(string) (device, error) { return fake, nil }
	return output
}

func newFakeDevice(channels int) *fakeDevice {
	return &fakeDevice{outputCount: channels, rate: 44100, minBuffer: 4, maxBuffer: 64, preferred: 16, granularity: 4}
}

func testPCM(samples ...int16) io.Reader {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	return &testPCMReader{data: data}
}

type testPCMReader struct {
	data []byte
}

func (r *testPCMReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), len(r.data))
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

func waitForRing(t *testing.T, output *stream, frames int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if output.ring.Available() >= frames || output.producerDone.Load() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("producer did not fill ring: available=%d", output.ring.Available())
}
