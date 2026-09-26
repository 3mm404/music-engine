package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"

	"music-engine/internal/console"
	"music-engine/internal/engine"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run() (result error) {
	path := flag.String("file", "music/demo.mp3", "MP3 inicial de zona A")
	pathB := flag.String("file-b", "", "MP3 inicial de zona B; vacio inicia B detenida")
	headless := flag.Bool("headless", false, "ejecutar sin consola hasta Ctrl+C")
	flag.Parse()
	tracks, err := initialTracks(*path)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	tracksB := tracks
	if *pathB != "" {
		tracksB, err = initialTracks(*pathB)
		if err != nil {
			return err
		}
	}
	m, err := engine.New([]engine.ZoneConfig{{ID: "A", Tracks: tracks}, {ID: "B", Tracks: tracksB}})
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, m.Close()) }()
	a, _ := m.Zone("A")
	if err := a.Play(*path); err != nil {
		return err
	}
	if *pathB != "" {
		b, _ := m.Zone("B")
		if err := b.Play(*pathB); err != nil {
			return err
		}
	}
	if *headless {
		fmt.Fprintln(os.Stdout, "Engine activo sin consola (zonas A/B). Ctrl+C para cerrar.")
		<-ctx.Done()
		return nil
	}
	return console.Run(ctx, m, os.Stdin, os.Stdout)
}

func initialTracks(path string) ([]string, error) {
	if strings.Contains(path, "://") {
		return []string{path}, nil
	}
	return tracksIn(filepath.Dir(path))
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
