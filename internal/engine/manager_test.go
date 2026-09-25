package engine

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"music-engine/internal/player"
)

func TestZonesAndLifecycle(t *testing.T) {
	for _, configs := range [][]ZoneConfig{{{ID: " "}}, {{ID: "A"}, {ID: "A"}}} {
		if _, err := New(configs); err == nil {
			t.Fatal("invalid zones accepted")
		}
	}
	m, err := New([]ZoneConfig{{ID: "A"}, {ID: "B"}, {ID: "C"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	if _, err := m.Zone("missing"); err == nil {
		t.Fatal("unknown zone accepted")
	}
	ids := m.IDs()
	ids[0] = "modified"
	if m.IDs()[0] != "A" || len(m.IDs()) != 3 {
		t.Fatal("zone IDs not isolated")
	}
	a, _ := m.Zone("A")
	b, _ := m.Zone("B")
	if err := a.SetVolume(.2); err != nil {
		t.Fatal(err)
	}
	if b.GetState().Volume != 1 {
		t.Fatal("volume leaked across zones")
	}
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Go(func() { _ = a.SetVolume(.3); _ = b.Stop(); _ = m.Close() })
	}
	wg.Wait()
	if err := a.Play("missing.mp3"); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed player restarted: %v", err)
	}
	if err := b.SetVolume(.8); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed volume: %v", err)
	}
	if a.GetState().Status != "STOPPED" {
		t.Fatal("close did not stop player")
	}
}

type monitoredPlayer struct {
	*player.Player
	polled chan struct{}
}

func (p *monitoredPlayer) GetState() player.State {
	select {
	case p.polled <- struct{}{}:
	default:
	}
	state := p.Player.GetState()
	state.Error = errors.New("simulated zone failure")
	return state
}

func TestMonitorWithoutConsoleAndErrorIsolation(t *testing.T) {
	var players []*monitoredPlayer
	m, err := newManager([]ZoneConfig{{ID: "A"}, {ID: "B"}}, func(paths []string) playback {
		p := &monitoredPlayer{Player: player.New(paths...), polled: make(chan struct{}, 1)}
		players = append(players, p)
		return p
	})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	// No public GetState calls: the manager alone must service both players,
	// including subsequent ticks after one reports an error.
	for i := 0; i < 2; i++ {
		for _, p := range players {
			select {
			case <-p.polled:
			case <-time.After(2 * time.Second):
				t.Fatal("monitor stopped servicing zones")
			}
		}
	}
}

func TestIndependentPlaybackIntegration(t *testing.T) {
	source := os.Getenv("MUSIC_ENGINE_TEST_MP3")
	if source == "" {
		t.Skip("define MUSIC_ENGINE_TEST_MP3 para probar audio real")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	second := filepath.Join(t.TempDir(), "second.mp3")
	if err := os.WriteFile(second, data, 0600); err != nil {
		t.Fatal(err)
	}
	m, err := New([]ZoneConfig{{ID: "A", Tracks: []string{source, second}}, {ID: "B", Tracks: []string{second, source}}})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	a, _ := m.Zone("A")
	b, _ := m.Zone("B")
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	check(a.SetVolume(.05))
	check(b.SetVolume(.08))
	check(a.Play(source))
	check(b.Play(second))
	assertB := func() {
		t.Helper()
		s := b.GetState()
		if s.Status != "PLAYING" || s.Track != second || s.Volume != .08 || s.Error != nil {
			t.Fatalf("zone B interrupted: %+v", s)
		}
	}
	for _, action := range []func() error{a.Pause, a.Resume, a.Next, a.Previous, a.Stop} {
		check(action())
		assertB()
	}
	check(a.Play(source))
	if err := a.Play(filepath.Join(t.TempDir(), "missing.mp3")); err == nil {
		t.Fatal("invalid track accepted")
	}
	assertB()
	check(b.Stop())
	if a.GetState().Status != "PLAYING" {
		t.Fatal("stopping B interrupted A")
	}
	check(m.Close())
	if a.GetState().Status != "STOPPED" || b.GetState().Status != "STOPPED" {
		t.Fatal("close left audio running")
	}
}
