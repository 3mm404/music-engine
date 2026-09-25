package player

import (
	"context"
	"errors"
	"math"
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

func TestVolumeAndEmptyList(t *testing.T) {
	p := New()
	if state := p.GetState(); state.Status != "STOPPED" || state.Volume != 1 || state.Index != -1 {
		t.Fatalf("estado inicial: %+v", state)
	}
	for _, volume := range []float64{-1, 1.1, math.NaN(), math.Inf(1)} {
		if err := p.SetVolume(volume); err == nil {
			t.Fatalf("acepto volumen %v", volume)
		}
	}
	if err := p.SetVolume(0.35); err != nil {
		t.Fatal(err)
	}
	if p.GetState().Volume != 0.35 {
		t.Fatal("no conserva volumen detenido")
	}
	if err := p.Next(); err == nil {
		t.Fatal("Next acepto lista vacia")
	}
	if err := p.Previous(); err == nil {
		t.Fatal("Previous acepto lista vacia")
	}
}

func TestNavigationIntegration(t *testing.T) {
	source := os.Getenv("MUSIC_ENGINE_TEST_MP3")
	if source == "" {
		t.Skip("define MUSIC_ENGINE_TEST_MP3 para probar audio real")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	first, second := filepath.Join(dir, "uno.mp3"), filepath.Join(dir, "dos.mp3")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := New(first, second)
	defer p.Stop()
	if err := p.SetVolume(0.1); err != nil {
		t.Fatal(err)
	}
	if err := p.Play(first); err != nil {
		t.Fatal(err)
	}
	check := func(status string, index int) {
		t.Helper()
		state := p.GetState()
		if state.Status != status || state.Index != index || state.Volume != 0.1 {
			t.Fatalf("estado: %+v", state)
		}
	}
	check("PLAYING", 0)
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	check("PAUSED", 0)
	if err := p.Resume(); err != nil {
		t.Fatal(err)
	}
	check("PLAYING", 0)
	if err := p.Next(); err != nil {
		t.Fatal(err)
	}
	check("PLAYING", 1)
	if p.audio.player.Volume() != 0.1 {
		t.Fatal("Oto no conserva volumen")
	}
	if err := p.Next(); err != nil {
		t.Fatal(err)
	}
	check("PLAYING", 0)
	if err := p.Previous(); err != nil {
		t.Fatal(err)
	}
	check("PLAYING", 1)
	if err := p.Stop(); err != nil {
		t.Fatal(err)
	}
	check("STOPPED", 1)
	if err := p.Previous(); err != nil {
		t.Fatal(err)
	}
	check("PLAYING", 0)
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	// Una pista que desaparece no debe interrumpir ni cambiar la seleccion.
	if err := os.Remove(second); err != nil {
		t.Fatal(err)
	}
	if err := p.Next(); err == nil {
		t.Fatal("acepto pista eliminada")
	}
	check("PAUSED", 0)
}
