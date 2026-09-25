package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"

	"music-engine/internal/console"
	"music-engine/internal/player"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run() error {
	// Conserva la ruta predeterminada que ya funciona en esta maquina.
	path := flag.String("file", "C:\\Users\\perez\\Desktop\\dev\\music-engine\\music\\demo.mp3", "ruta del MP3 inicial")
	flag.Parse()
	tracks, err := tracksIn(filepath.Dir(*path))
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	p := player.New(tracks...)
	defer p.Stop()
	if err := p.Play(*path); err != nil {
		return err
	}
	return console.Run(ctx, p, os.Stdin, os.Stdout)
}

// Lista simple, ordenada por nombre y cargada una vez al iniciar.
func tracksIn(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("leer carpeta de musica: %w", err)
	}
	var tracks []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".mp3") {
			tracks = append(tracks, filepath.Join(dir, entry.Name()))
		}
	}
	sort.Strings(tracks)
	return tracks, nil
}
