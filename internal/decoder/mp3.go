// Package decoder convierte MP3 a PCM; no conoce Oto ni dispositivos de audio.
package decoder

import (
	"bytes"
	"fmt"
	"io"
	"os"

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
	*mp3.Decoder
	file io.ReadCloser
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
		file.Close()
		return nil, fmt.Errorf("decodificar MP3: %w", err)
	}
	return &Stream{Decoder: decoded, file: file}, nil
}

// Close se llama solo cuando el backend ha dejado de leer el stream.
func (s *Stream) Close() error { return s.file.Close() }
