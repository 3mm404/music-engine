package asio

import (
	"encoding/binary"
	"io"
	"testing"
)

func TestPCMConverterPreservesInterleavedSamplesAndPadsChannels(t *testing.T) {
	converter := newPCMConverter(pcmReader(100, -200, 300, -400), 2, 4, 44100, 44100, 2)
	for _, want := range [][]int16{{100, -200, 0, 0}, {300, -400, 0, 0}} {
		got := make([]int16, 4)
		if err := converter.ReadFrame(got); err != nil {
			t.Fatal(err)
		}
		for channel := range want {
			if got[channel] != want[channel] {
				t.Fatalf("frame = %v, want %v", got, want)
			}
		}
	}
	if err := converter.ReadFrame(make([]int16, 4)); err != io.EOF {
		t.Fatalf("end of stream error = %v, want EOF", err)
	}
}

func TestPCMConverterResamplesAtConfiguredRate(t *testing.T) {
	converter := newPCMConverter(pcmReader(0, 1000), 1, 1, 2, 4, 2)
	var got []int16
	for {
		frame := make([]int16, 1)
		err := converter.ReadFrame(frame)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, frame[0])
	}
	want := []int16{0, 500, 1000, 1000}
	if len(got) != len(want) {
		t.Fatalf("resampled sample count = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("resampled samples = %v, want %v", got, want)
		}
	}
}

func TestPCMConverterRejectsPartialFrames(t *testing.T) {
	converter := newPCMConverter(&oneByteReader{}, 2, 2, 44100, 44100, 1)
	if err := converter.ReadFrame(make([]int16, 2)); err == nil {
		t.Fatal("partial interleaved PCM frame accepted")
	}
}

func pcmReader(samples ...int16) io.Reader {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(sample))
	}
	return &shortReader{data: data}
}

type shortReader struct {
	data []byte
}

func (r *shortReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := min(len(p), len(r.data))
	copy(p, r.data[:n])
	r.data = r.data[n:]
	return n, nil
}

type oneByteReader struct{ read bool }

func (r *oneByteReader) Read(p []byte) (int, error) {
	if r.read {
		return 0, io.EOF
	}
	r.read = true
	p[0] = 1
	return 1, nil
}
