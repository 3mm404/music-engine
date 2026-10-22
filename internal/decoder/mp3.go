// Package decoder convierte MP3 a PCM; no conoce Oto ni dispositivos de audio.
package decoder

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	mp3 "github.com/hajimehoshi/go-mp3"
)

// memoryAudio retains io.Seeker so the MP3 decoder can measure the full PCM
// length of verified downloads. io.NopCloser would hide this capability.
type memoryAudio struct{ *bytes.Reader }

func (m *memoryAudio) Close() error { return nil }

func OpenBytes(data []byte) (*Stream, error) {
	return OpenReader(&memoryAudio{Reader: bytes.NewReader(data)})
}

// Stream produce PCM signed int16 little-endian, siempre de dos canales.
// El decoder procesa el audio a medida que el backend lo lee.
type Stream struct {
	pcmDecoder
	file io.ReadCloser
}

type pcmDecoder interface {
	io.ReadSeeker
	SampleRate() int
	Length() int64
}

func Open(path string) (*Stream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrir MP3 %q: %w", path, err)
	}
	return OpenReader(file)
}

func OpenReader(file io.ReadCloser) (*Stream, error) {
	decoded, err := mp3.NewDecoder(file)
	if err != nil {
		if seeker, ok := file.(io.Seeker); ok && strings.Contains(err.Error(), "MPEG version 2.5") {
			if _, seekErr := seeker.Seek(0, io.SeekStart); seekErr == nil {
				if fallback, fallbackErr := openExtended(file); fallbackErr == nil {
					return &Stream{pcmDecoder: fallback, file: file}, nil
				}
			}
		}
		file.Close()
		return nil, fmt.Errorf("decodificar MP3: %w", err)
	}
	return &Stream{pcmDecoder: decoded, file: file}, nil
}

// Close se llama solo cuando el backend ha dejado de leer el stream.
func (s *Stream) Close() error { return s.file.Close() }
