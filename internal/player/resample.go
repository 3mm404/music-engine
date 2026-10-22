package player

import (
	"bufio"
	"encoding/binary"
	"io"
	"math"
	"sync"
)

const resampleTaps = 32
const resamplePhases = 1024

var filterCache sync.Map

type ratePair struct{ input, output int }
type filterBank [resamplePhases][resampleTaps]float64

// Coefficients use a windowed sinc low-pass filter to avoid aliasing on downsampling.
// A shared phase bank avoids trigonometry in the audio read path.
func resampleFilters(input, output int) *filterBank {
	key := ratePair{input, output}
	if existing, ok := filterCache.Load(key); ok {
		return existing.(*filterBank)
	}
	bank := new(filterBank)
	cutoff := math.Min(1, float64(output)/float64(input)) * .94
	for phase := range bank {
		fraction := float64(phase) / resamplePhases
		for tap := range bank[phase] {
			x := float64(tap-15) - fraction
			sinc := cutoff
			if math.Abs(x) > 1e-10 {
				sinc = math.Sin(math.Pi*cutoff*x) / (math.Pi * x)
			}
			window := .5 + .5*math.Cos(math.Pi*x/16)
			bank[phase][tap] = sinc * window
		}
	}
	actual, _ := filterCache.LoadOrStore(key, bank)
	return actual.(*filterBank)
}

type resampleReader struct {
	source                *bufio.Reader
	inputRate, outputRate int
	filters               *filterBank
	frames                [64][2]int16
	readFrames            int64
	position              int64
	eof                   bool
	pending               [4]byte
	pendingAt             int
}

// resamplePCM consumes stereo signed int16 LE and keeps memory bounded independently
// of track duration. Position is rational, so rates do not accumulate clock drift.
func resamplePCM(source io.Reader, inputRate, outputRate int) io.Reader {
	if inputRate == outputRate {
		return source
	}
	return &resampleReader{source: bufio.NewReaderSize(source, 4096), inputRate: inputRate,
		outputRate: outputRate, filters: resampleFilters(inputRate, outputRate), pendingAt: 4}
}

func (r *resampleReader) next() error {
	center := r.position / int64(r.outputRate)
	for !r.eof && r.readFrames <= center+16 {
		var frame [4]byte
		_, err := io.ReadFull(r.source, frame[:])
		if err == io.EOF {
			r.eof = true
			break
		}
		if err != nil {
			return err
		}
		r.frames[r.readFrames%64] = [2]int16{int16(binary.LittleEndian.Uint16(frame[:2])), int16(binary.LittleEndian.Uint16(frame[2:]))}
		r.readFrames++
	}
	if r.eof && center >= r.readFrames {
		return io.EOF
	}
	phase := int((r.position % int64(r.outputRate)) * resamplePhases / int64(r.outputRate))
	var samples [2]float64
	var weight float64
	for tap, coefficient := range r.filters[phase] {
		index := center + int64(tap) - 15
		if index < 0 || index >= r.readFrames {
			continue
		}
		frame := r.frames[index%64]
		samples[0] += float64(frame[0]) * coefficient
		samples[1] += float64(frame[1]) * coefficient
		weight += coefficient
	}
	for channel := range samples {
		value := math.Round(samples[channel] / weight)
		value = math.Max(-32768, math.Min(32767, value))
		binary.LittleEndian.PutUint16(r.pending[channel*2:], uint16(int16(value)))
	}
	r.pendingAt = 0
	r.position += int64(r.inputRate)
	return nil
}

func (r *resampleReader) Read(buffer []byte) (int, error) {
	written := 0
	for written < len(buffer) {
		if r.pendingAt == 4 {
			if err := r.next(); err != nil {
				return written, err
			}
		}
		n := copy(buffer[written:], r.pending[r.pendingAt:])
		written += n
		r.pendingAt += n
	}
	return written, nil
}
