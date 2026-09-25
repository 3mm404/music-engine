// agent synchronizes Laravel configuration without opening an audio device.
package main

import (
	"context"
	"log"
	"music-engine/internal/control"
	"os"
	"os/signal"
)

var version = "0.2.0"

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	dir := os.Getenv("ENGINE_STATE_DIR")
	if dir == "" {
		dir = "engine-state"
	}
	if err := control.Run(ctx, os.Getenv("ENGINE_SERVER"), os.Getenv("ENGINE_TOKEN"), version, dir); err != nil {
		log.Fatal(err)
	}
}
