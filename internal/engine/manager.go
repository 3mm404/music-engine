// Package engine owns independent zone players and their lifecycle, without a UI.
package engine

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"music-engine/internal/decoder"
	"music-engine/internal/player"
)

var ErrClosed = errors.New("el administrador de reproductores esta cerrado")

type ZoneConfig struct {
	ID     string
	Tracks []string
}

type playback interface {
	Play(string) error
	Pause() error
	Resume() error
	Stop() error
	Next() error
	Previous() error
	SetVolume(float64) error
	GetState() player.State
}

// Manager owns independent zones. Reconfigure changes membership; Close releases all players.
// Players share the Oto device, but never their track, volume or transport state.
type Manager struct {
	mu       sync.RWMutex
	closed   bool
	factory  func([]string) playback
	zones    map[string]*Zone
	ids      []string
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	closeErr error
}

func New(configs []ZoneConfig) (*Manager, error) {
	return newManager(configs, func(tracks []string) playback { return player.New(tracks...) })
}

func NewRemote(source decoder.HTTPS) (*Manager, error) {
	return newManager(nil, func([]string) playback { return player.NewRemote(44100, source) })
}

func NewRemoteMono(source decoder.HTTPS) (*Manager, error) {
	return newManager(nil, func([]string) playback { return player.NewRemoteMono(44100, source) })
}

// Reconfigure changes membership, retaining every unchanged zone and its audio.
func (m *Manager) Reconfigure(configs []ZoneConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrClosed
	}
	seen := map[string]bool{}
	for _, c := range configs {
		if strings.TrimSpace(c.ID) == "" || seen[c.ID] {
			return errors.New("zona invalida")
		}
		seen[c.ID] = true
	}
	for id, z := range m.zones {
		if !seen[id] {
			z.mu.Lock()
			z.closed = true
			z.p.Stop()
			z.mu.Unlock()
			delete(m.zones, id)
		}
	}
	m.ids = nil
	for _, c := range configs {
		m.ids = append(m.ids, c.ID)
		if m.zones[c.ID] == nil {
			m.zones[c.ID] = &Zone{p: m.factory(append([]string(nil), c.Tracks...))}
		}
	}
	return nil
}

func newManager(configs []ZoneConfig, factory func([]string) playback) (*Manager, error) {
	seen := make(map[string]bool, len(configs))
	for _, config := range configs {
		if strings.TrimSpace(config.ID) == "" || seen[config.ID] {
			return nil, fmt.Errorf("identificador de zona vacio o duplicado: %q", config.ID)
		}
		seen[config.ID] = true
	}
	m := &Manager{factory: factory, zones: make(map[string]*Zone, len(configs)), stop: make(chan struct{}), done: make(chan struct{})}
	for _, config := range configs {
		m.ids = append(m.ids, config.ID)
		m.zones[config.ID] = &Zone{p: factory(append([]string(nil), config.Tracks...))}
	}
	go m.monitor()
	return m, nil
}

// monitor detects EOF/errors and releases streams even with no console or polling client.
// A failed zone remains observable and does not stop the other zones.
func (m *Manager) monitor() {
	defer close(m.done)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.mu.RLock()
			for _, zone := range m.zones {
				zone.GetState()
			}
			m.mu.RUnlock()
		}
	}
}

func (m *Manager) IDs() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]string(nil), m.ids...)
}

func (m *Manager) Zone(id string) (*Zone, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	z, ok := m.zones[id]
	if !ok {
		return nil, fmt.Errorf("zona desconocida: %q", id)
	}
	return z, nil
}

// Close is concurrent-safe and idempotent; retained zone handles cannot restart audio.
func (m *Manager) Close() error {
	m.once.Do(func() {
		close(m.stop)
		<-m.done
		m.mu.Lock()
		defer m.mu.Unlock()
		m.closed = true
		for _, id := range m.ids {
			z := m.zones[id]
			z.mu.Lock()
			z.closed = true
			if err := z.p.Stop(); err != nil {
				m.closeErr = errors.Join(m.closeErr, fmt.Errorf("zona %s: %w", id, err))
			}
			z.mu.Unlock()
		}
	})
	return m.closeErr
}

// Zone serializes its own operations and never exposes its underlying player.
type Zone struct {
	mu     sync.Mutex
	p      playback
	closed bool
}

func (z *Zone) apply(fn func(playback) error) error {
	z.mu.Lock()
	defer z.mu.Unlock()
	if z.closed {
		return ErrClosed
	}
	return fn(z.p)
}

func (z *Zone) Play(path string) error {
	return z.apply(func(p playback) error { return p.Play(path) })
}

func (z *Zone) PlayVerified(path, hash string) error {
	return z.apply(func(p playback) error {
		verified, ok := p.(interface{ PlayVerified(string, string) error })
		if !ok {
			return errors.New("reproductor sin verificacion de contenido")
		}
		return verified.PlayVerified(path, hash)
	})
}
func (z *Zone) Pause() error    { return z.apply(playback.Pause) }
func (z *Zone) Resume() error   { return z.apply(playback.Resume) }
func (z *Zone) Stop() error     { return z.apply(playback.Stop) }
func (z *Zone) Next() error     { return z.apply(playback.Next) }
func (z *Zone) Previous() error { return z.apply(playback.Previous) }
func (z *Zone) SetVolume(v float64) error {
	return z.apply(func(p playback) error { return p.SetVolume(v) })
}
func (z *Zone) GetState() player.State {
	z.mu.Lock()
	defer z.mu.Unlock()
	return z.p.GetState()
}
