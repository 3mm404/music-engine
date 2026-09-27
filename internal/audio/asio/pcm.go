package asio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

type pcmConverter struct {
	source         io.Reader
	inputChannels  int
	outputChannels int
	inputRate      int
	outputRate     int
	block          []byte
	blockFrames    int
	blockAt        int
	pendingErr     error
	previous       []int16
	next           []int16
	position       float64
	inputBase      int64
	step           float64
	initialized    bool
	hasNext        bool
	done           bool
}

func newPCMConverter(source io.Reader, inputChannels, outputChannels, inputRate, outputRate, blockFrames int) *pcmConverter {
	if blockFrames < 1 {
		blockFrames = 1
	}
	return &pcmConverter{
		source: source, inputChannels: inputChannels, outputChannels: outputChannels,
		inputRate: inputRate, outputRate: outputRate,
		block:    make([]byte, blockFrames*inputChannels*2),
		previous: make([]int16, inputChannels), next: make([]int16, inputChannels),
		step: float64(inputRate) / float64(outputRate),
	}
}

// ReadFrame resamples one input stream frame into one interleaved output frame.
// The backend producer calls it; the ASIO callback never performs PCM reads.
func (c *pcmConverter) ReadFrame(output []int16) error {
	if len(output) != c.outputChannels || c.step <= 0 || c.inputChannels < 1 || c.outputChannels < c.inputChannels {
		return errors.New("configuracion PCM del conversor invalida")
	}
	if !c.initialized {
		if err := c.readSourceFrame(c.previous); err != nil {
			return err
		}
		err := c.readSourceFrame(c.next)
		c.hasNext = err == nil
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if !c.hasNext {
			copy(c.next, c.previous)
		}
		c.initialized = true
	}

	for c.hasNext && c.position-float64(c.inputBase) >= 1 {
		copy(c.previous, c.next)
		c.inputBase++
		if err := c.readSourceFrame(c.next); err != nil {
			if !errors.Is(err, io.EOF) {
				return err
			}
			c.hasNext = false
			copy(c.next, c.previous)
		}
	}
	if !c.hasNext && c.position-float64(c.inputBase) >= 1 {
		c.done = true
	}
	if c.done {
		return io.EOF
	}

	fraction := c.position - float64(c.inputBase)
	for channel := 0; channel < c.inputChannels; channel++ {
		first, second := float64(c.previous[channel]), float64(c.next[channel])
		value := math.Round(first + (second-first)*fraction)
		if value > math.MaxInt16 {
			value = math.MaxInt16
		} else if value < math.MinInt16 {
			value = math.MinInt16
		}
		output[channel] = int16(value)
	}
	clear(output[c.inputChannels:])
	c.position += c.step
	return nil
}

func (c *pcmConverter) readSourceFrame(frame []int16) error {
	frameBytes := c.inputChannels * 2
	if c.blockAt >= c.blockFrames {
		if c.pendingErr != nil {
			err := c.pendingErr
			c.pendingErr = nil
			if errors.Is(err, io.ErrUnexpectedEOF) {
				return io.EOF
			}
			return err
		}
		n, err := io.ReadFull(c.source, c.block)
		if n%frameBytes != 0 {
			return fmt.Errorf("PCM source returned a partial %d-byte frame", frameBytes)
		}
		c.blockFrames = n / frameBytes
		c.blockAt = 0
		c.pendingErr = err
		if c.blockFrames == 0 {
			if err == nil {
				return io.ErrNoProgress
			}
			return err
		}
	}
	start := c.blockAt * frameBytes
	for channel := range frame {
		frame[channel] = int16(binary.LittleEndian.Uint16(c.block[start+channel*2:]))
	}
	c.blockAt++
	if c.blockAt == c.blockFrames {
		c.blockFrames = 0
		c.blockAt = 0
	}
	return nil
}
