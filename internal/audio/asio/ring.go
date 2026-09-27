package asio

import (
	"math"
	"sync/atomic"
)

// frameRing is a bounded single-producer/single-consumer ring. The producer
// runs outside the ASIO callback; the callback only reads atomics and samples.
type frameRing struct {
	data     []int16
	channels int
	capacity uint64
	read     atomic.Uint64
	write    atomic.Uint64
	overflow atomic.Uint64
}

func newFrameRing(capacity, channels int) *frameRing {
	if capacity < 1 || channels < 1 {
		panic("frame ring requires positive capacity and channels")
	}
	return &frameRing{data: make([]int16, capacity*channels), channels: channels, capacity: uint64(capacity)}
}

func (r *frameRing) Available() int {
	return int(r.write.Load() - r.read.Load())
}

func (r *frameRing) Free() int {
	available := r.Available()
	if available >= int(r.capacity) {
		return 0
	}
	return int(r.capacity) - available
}

// Write stores complete interleaved frames and drops newest frames on overflow.
func (r *frameRing) Write(samples []int16) int {
	frames := len(samples) / r.channels
	if frames == 0 {
		return 0
	}
	writeAt := r.write.Load()
	readAt := r.read.Load()
	free := int(r.capacity - (writeAt - readAt))
	if free < 0 {
		free = 0
	}
	written := min(frames, free)
	for frame := 0; frame < written; frame++ {
		index := int((writeAt+uint64(frame))%r.capacity) * r.channels
		copy(r.data[index:index+r.channels], samples[frame*r.channels:(frame+1)*r.channels])
	}
	r.write.Store(writeAt + uint64(written))
	if written < frames {
		r.overflow.Add(uint64(frames - written))
	}
	return written
}

// ReadPlanar fills driver-owned channel buffers. Missing frames are silence.
// It returns true if any part of this callback underruns.
func (r *frameRing) ReadPlanar(outputs [][]float32, gain float32) bool {
	frames := 0
	for _, channel := range outputs {
		if len(channel) > frames {
			frames = len(channel)
		}
	}
	readAt := r.read.Load()
	underrun := false
	for frame := 0; frame < frames; frame++ {
		writeAt := r.write.Load()
		if readAt >= writeAt {
			underrun = true
			for channel := range outputs {
				if frame < len(outputs[channel]) {
					outputs[channel][frame] = 0
				}
			}
			continue
		}
		index := int(readAt%r.capacity) * r.channels
		for channel := range outputs {
			if frame >= len(outputs[channel]) {
				continue
			}
			if channel < r.channels {
				outputs[channel][frame] = float32(r.data[index+channel]) * (1.0 / 32768.0) * gain
			} else {
				outputs[channel][frame] = 0
			}
		}
		readAt++
	}
	r.read.Store(readAt)
	return underrun
}

func (r *frameRing) OverflowFrames() uint64 { return r.overflow.Load() }

func validRingCapacity(capacity, channels int) bool {
	return capacity > 0 && channels > 0 && capacity <= math.MaxInt/channels
}
