// agent synchronizes Laravel configuration and plays signed HTTPS MP3 by zone.
package main

import (
	"context"
	"fmt"
	"log"
	"music-engine/internal/audio"
	"music-engine/internal/audio/backend"
	"music-engine/internal/control"
	"os"
	"os/signal"
	"strings"
)

var version = "0.6.0"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	dir := os.Getenv("ENGINE_STATE_DIR")
	if dir == "" {
		dir = "engine-state"
	}
	profile := os.Getenv("ENGINE_PROFILE")
	if profile == "" {
		profile = control.PlaybackProfile
	}
	mode := os.Getenv("ENGINE_CHANNEL_MODE")
	if mode != "" && mode != "stereo" && mode != "mono" {
		fmt.Fprintln(os.Stderr, "ENGINE_CHANNEL_MODE debe ser mono o stereo")
		os.Exit(1)
	}
	if profile == control.PlaybackProfile || profile == control.MonoProfile {
		if mode == "mono" {
			profile = control.MonoProfile
		}
		if mode == "stereo" {
			profile = control.PlaybackProfile
		}
	}
	var output audio.Output
	if profile == control.PlaybackProfile || profile == control.MonoProfile {
		backendName := strings.ToLower(strings.TrimSpace(os.Getenv("ENGINE_AUDIO_BACKEND")))
		if backendName == "" {
			backendName = "oto"
		}
		log.Printf("Audio backend: %s", strings.ToUpper(backendName))
		if backendName == "asio" {
			log.Printf("Oto output: not selected (ASIO backend is exclusive; no automatic fallback)")
			log.Printf("ASIO driver requested: %q sample_rate=%q buffer_size=%q output_channels=%q",
				os.Getenv("ENGINE_ASIO_DRIVER"), os.Getenv("ENGINE_ASIO_SAMPLE_RATE"),
				os.Getenv("ENGINE_ASIO_BUFFER_SIZE"), os.Getenv("ENGINE_ASIO_CHANNELS"))
		}
		var err error
		output, err = backend.FromEnvironment()
		if err != nil {
			log.Fatal(err)
		}
	}
	if err := control.RunProfileWithOutput(ctx, os.Getenv("ENGINE_SERVER"), os.Getenv("ENGINE_TOKEN"), version, dir, profile, output); err != nil {
		log.Fatal(err)
	}
}
