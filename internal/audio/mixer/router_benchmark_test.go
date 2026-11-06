package mixer

import (
	"fmt"
	"io"
	"testing"

	"music-engine/internal/audio"
)

// continuousPCM represents a decoded stereo source with no I/O or end of stream.
type continuousPCM struct{}

func (continuousPCM) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

func BenchmarkRoutePCM(b *testing.B) {
	const frames = 256
	format := audio.PCMFormat{SampleRate: 44100, Channels: 2, SampleFormat: audio.SignedInt16LE}

	for _, zones := range []int{2, 8} {
		b.Run(fmt.Sprintf("%d_zones", zones), func(b *testing.B) {
			backend := &captureOutput{}
			router := New(backend)
			b.Cleanup(func() {
				if err := router.Close(); err != nil {
					b.Error(err)
				}
			})

			routes := make([]Route, zones)
			for i := range routes {
				routes[i] = Route{ZoneID: fmt.Sprint(i), Channels: []int{2*i + 1, 2*i + 2}}
			}
			if err := router.Configure(routes); err != nil {
				b.Fatal(err)
			}
			for _, route := range routes {
				stream, err := router.OutputForZone(route.ZoneID).Open(continuousPCM{}, format)
				if err != nil {
					b.Fatal(err)
				}
				stream.Play()
			}

			buffer := make([]byte, frames*zones*4)
			b.SetBytes(int64(len(buffer)))
			b.ReportAllocs()
			for b.Loop() {
				if _, err := io.ReadFull(backend.source, buffer); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
