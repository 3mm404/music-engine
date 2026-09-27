// Package backend selects an audio output from local Engine environment settings.
package backend

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"music-engine/internal/audio"
	"music-engine/internal/audio/asio"
	"music-engine/internal/audio/oto"
)

func FromEnvironment() (audio.Output, error) {
	name := strings.ToLower(strings.TrimSpace(os.Getenv("ENGINE_AUDIO_BACKEND")))
	if name == "" {
		name = "asio"
	}
	if name == "oto" {
		return oto.NewOutput(), nil
	}
	if name != "asio" {
		return nil, fmt.Errorf("ENGINE_AUDIO_BACKEND debe ser oto o asio, no %q", name)
	}
	config := asio.Config{DriverName: strings.TrimSpace(os.Getenv("ENGINE_ASIO_DRIVER"))}
	var err error
	if config.SampleRate, err = envInt("ENGINE_ASIO_SAMPLE_RATE"); err != nil {
		return nil, err
	}
	if config.BufferSize, err = envInt("ENGINE_ASIO_BUFFER_SIZE"); err != nil {
		return nil, err
	}
	if config.Channels, err = envInt("ENGINE_ASIO_CHANNELS"); err != nil {
		return nil, err
	}
	output, err := asio.NewOutput(config)
	if err != nil {
		return nil, err
	}
	return output, nil
}

func envInt(name string) (int, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s debe ser un entero positivo", name)
	}
	return parsed, nil
}
