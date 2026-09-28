package backend

import (
	"fmt"
	"music-engine/internal/audio"
	"music-engine/internal/audio/asio"
	"music-engine/internal/audio/oto"
)

// New selects exactly one backend. A failure never selects another backend.
func New(name string, config asio.Config) (audio.Output, error) {
	switch name {
	case "", "asio":
		output, err := asio.NewOutput(config)
		if err != nil {
			return nil, err
		}
		return output, nil
	case "oto":
		return oto.NewOutput(), nil
	default:
		return nil, fmt.Errorf("backend desconocido %q; use oto o asio", name)
	}
}
