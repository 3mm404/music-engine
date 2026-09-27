package asio

import (
	"runtime"
	"sync"
	"testing"
)

func TestFrameRingInterleavedToPlanar(t *testing.T) {
	ring := newFrameRing(4, 4)
	if written := ring.Write([]int16{100, 200, 300, 400, -100, -200, -300, -400}); written != 2 {
		t.Fatalf("wrote %d frames, want 2", written)
	}
	outputs := [][]float32{make([]float32, 2), make([]float32, 2), make([]float32, 2), make([]float32, 2)}
	if underrun := ring.ReadPlanar(outputs, 1); underrun {
		t.Fatal("unexpected underrun")
	}
	want := [][]float32{{100.0 / 32768, -100.0 / 32768}, {200.0 / 32768, -200.0 / 32768}, {300.0 / 32768, -300.0 / 32768}, {400.0 / 32768, -400.0 / 32768}}
	for channel := range want {
		for frame := range want[channel] {
			if outputs[channel][frame] != want[channel][frame] {
				t.Fatalf("channel %d frame %d = %v, want %v", channel, frame, outputs[channel][frame], want[channel][frame])
			}
		}
	}
}

func TestFrameRingUnderrunIsSilenceAndOverflowDropsNewest(t *testing.T) {
	ring := newFrameRing(2, 2)
	if written := ring.Write([]int16{10, 20, 30, 40, 50, 60}); written != 2 {
		t.Fatalf("overflow wrote %d frames, want 2", written)
	}
	if got := ring.OverflowFrames(); got != 1 {
		t.Fatalf("overflow count = %d, want 1", got)
	}
	outputs := [][]float32{make([]float32, 3), make([]float32, 3)}
	if underrun := ring.ReadPlanar(outputs, 1); !underrun {
		t.Fatal("expected underrun")
	}
	if outputs[0][2] != 0 || outputs[1][2] != 0 {
		t.Fatalf("underrun output must be silence: %v", outputs)
	}
}

func TestFrameRingConcurrentSingleProducerConsumer(t *testing.T) {
	const frames = 10000
	ring := newFrameRing(32, 2)
	var group sync.WaitGroup
	group.Go(func() {
		for sample := int16(1); sample <= frames; sample++ {
			for ring.Write([]int16{sample, sample}) == 0 {
				runtime.Gosched()
			}
		}
	})
	outputs := [][]float32{make([]float32, 1), make([]float32, 1)}
	read := 0
	for read < frames {
		underrun := ring.ReadPlanar(outputs, 1)
		if underrun {
			runtime.Gosched()
			continue
		}
		if outputs[0][0] != outputs[1][0] {
			t.Fatalf("torn interleaved frame at read %d: %v", read, outputs)
		}
		read++
	}
	group.Wait()
}
