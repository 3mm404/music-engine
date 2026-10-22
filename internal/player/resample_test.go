package player

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"testing"
)

func pcmTone(rate, frames int, frequency float64) []byte {
	data := make([]byte, frames*4)
	for frame := 0; frame < frames; frame++ {
		sample := int16(math.Round(12000 * math.Sin(2*math.Pi*frequency*float64(frame)/float64(rate))))
		binary.LittleEndian.PutUint16(data[frame*4:], uint16(sample))
		binary.LittleEndian.PutUint16(data[frame*4+2:], uint16(-sample))
	}
	return data
}

func TestResampleRatesPreserveDurationPitchAndChannels(t *testing.T) {
	for _, rate := range []int{8000, 11025, 12000, 16000, 22050, 24000, 32000, 36000, 40000, 44100, 48000} {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			frames := rate / 10
			output, err := io.ReadAll(resamplePCM(bytes.NewReader(pcmTone(rate, frames, 500)), rate, 44100))
			if err != nil {
				t.Fatal(err)
			}
			wantFrames := (frames*44100 + rate - 1) / rate
			if len(output) != wantFrames*4 {
				t.Fatalf("duration changed: %d bytes, want %d", len(output), wantFrames*4)
			}
			var errorSum float64
			for i := 64; i < wantFrames-64; i++ {
				left := int16(binary.LittleEndian.Uint16(output[i*4:]))
				right := int16(binary.LittleEndian.Uint16(output[i*4+2:]))
				if left != -right {
					t.Fatal("stereo channels mixed or swapped")
				}
				expected := 12000 * math.Sin(2*math.Pi*500*float64(i)/44100)
				errorSum += math.Pow(float64(left)-expected, 2)
			}
			if rms := math.Sqrt(errorSum / float64(wantFrames-128)); rms > 100 {
				t.Fatalf("pitch/waveform changed: RMS error %f", rms)
			}
		})
	}
}

func TestResampleFiltersAliasing(t *testing.T) {
	output, err := io.ReadAll(resamplePCM(bytes.NewReader(pcmTone(48000, 4800, 22000)), 48000, 16000))
	if err != nil {
		t.Fatal(err)
	}
	var energy float64
	for i := 64; i < len(output)/4-64; i++ {
		energy += math.Pow(float64(int16(binary.LittleEndian.Uint16(output[i*4:]))), 2)
	}
	if rms := math.Sqrt(energy / float64(len(output)/4-128)); rms > 100 {
		t.Fatalf("aliasing not suppressed: RMS %f", rms)
	}
}

func TestResamplePartialReadsEmptyAndTruncatedInput(t *testing.T) {
	input := pcmTone(48000, 101, 500)
	want, err := io.ReadAll(resamplePCM(bytes.NewReader(input), 48000, 44100))
	if err != nil {
		t.Fatal(err)
	}
	reader := resamplePCM(bytes.NewReader(input), 48000, 44100)
	var actual []byte
	for {
		var b [1]byte
		n, err := reader.Read(b[:])
		actual = append(actual, b[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Equal(actual, want) {
		t.Fatal("partial reads changed PCM")
	}
	empty, err := io.ReadAll(resamplePCM(bytes.NewReader(nil), 48000, 44100))
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty input: %v", err)
	}
	if _, err := io.ReadAll(resamplePCM(bytes.NewReader([]byte{1, 2, 3}), 48000, 44100)); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated PCM: %v", err)
	}
	original := pcmTone(44100, 100, 500)
	unchanged, _ := io.ReadAll(resamplePCM(bytes.NewReader(original), 44100, 44100))
	if !bytes.Equal(unchanged, original) {
		t.Fatal("matching-rate input changed")
	}
}
