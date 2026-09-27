// agent synchronizes Laravel configuration and plays signed HTTPS MP3 by zone.
package main

import (
	"context"
	"fmt"
	"log"
	"music-engine/internal/control"
	"os"
	"os/signal"
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
	if err := control.RunProfile(ctx, os.Getenv("ENGINE_SERVER"), os.Getenv("ENGINE_TOKEN"), version, dir, profile); err != nil {
		log.Fatal(err)
	}
}
