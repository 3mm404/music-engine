// Package decoder convierte MP3 a PCM; no conoce Oto ni dispositivos de audio.
package decoder

import (
	"fmt"
	"os"

	mp3 "github.com/hajimehoshi/go-mp3"
)

// Stream produce PCM signed int16 little-endian, siempre de dos canales.
// El decoder procesa el audio a medida que el backend lo lee.
type Stream struct {
	*mp3.Decoder
	file *os.File
}

func Open(path string) (*Stream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("abrir MP3 %q: %w", path, err)
	}
	decoded, err := mp3.NewDecoder(file)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("decodificar MP3 %q: %w", path, err)
	}
	return &Stream{Decoder: decoded, file: file}, nil
}

// Close se llama solo cuando el backend ha dejado de leer el stream.
func (s *Stream) Close() error { return s.file.Close() }
