package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"music-engine/internal/player"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("file", "music/demo.mp3", "ruta del archivo MP3")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	p := player.New()
	defer p.Stop()
	if err := p.Play(*path); err != nil {
		return err
	}
	fmt.Printf("Reproduciendo %s. Ctrl+C para detener.\n", *path)
	if err := p.Wait(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	fmt.Println("Reproduccion finalizada.")
	return nil
}
