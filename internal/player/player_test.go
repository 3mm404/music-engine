package player

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMissingAndInvalidMP3(t *testing.T) {
	p := New()
	if !errors.Is(p.Pause(), ErrNoTrack) {
		t.Fatal("Pause sin pista debe fallar")
	}
	if !errors.Is(p.Resume(), ErrNoTrack) {
		t.Fatal("Resume sin pista debe fallar")
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := p.Play(filepath.Join(t.TempDir(), "missing.mp3")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error inesperado: %v", err)
	}
	path := filepath.Join(t.TempDir(), "invalid.mp3")
	if err := os.WriteFile(path, []byte("esto no es MP3"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := p.Play(path); err == nil {
		t.Fatal("acepto un archivo invalido")
	}
	if err := p.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
}

// Prueba optativa contra el dispositivo real; no agrega fixtures musicales al repo.
func TestPlaybackIntegration(t *testing.T) {
	path := os.Getenv("MUSIC_ENGINE_TEST_MP3")
	if path == "" {
		t.Skip("define MUSIC_ENGINE_TEST_MP3 para probar audio real")
	}
	p := New()
	defer p.Stop()
	if err := p.Play(path); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- p.Wait(context.Background()) }()
	select {
	case err := <-done:
		t.Fatalf("Wait termino durante pausa: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	if err := p.Play(filepath.Join(t.TempDir(), "missing.mp3")); err == nil {
		t.Fatal("archivo inexistente aceptado")
	}
	if err := p.Resume(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait no termino tras Stop")
	}
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := p.Play(path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := p.Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelacion: %v", err)
	}
	if !errors.Is(p.Resume(), ErrNoTrack) {
		t.Fatal("cancelar debe descargar la pista")
	}
}
