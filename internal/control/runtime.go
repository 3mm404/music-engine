package control

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"music-engine/internal/audio"
	"music-engine/internal/decoder"
	"music-engine/internal/engine"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Runtime synchronizes configuration, durable commands and observed zone state.
type Runtime struct {
	client      *Client
	journal     *Journal
	deviceID    string
	mu          sync.Mutex
	config      Config
	configError *ConfigError
	sequence    int64
	audio       *engine.Manager
	selected    map[string]int
	pending     map[string]*pendingPlayback
}

func (r *Runtime) apply(c Config) error {
	if c.DeviceID != r.deviceID || c.Revision < 1 || c.Zones == nil {
		return errors.New("configuración inválida o equipo incorrecto")
	}
	seen := map[string]bool{}
	for _, z := range c.Zones {
		if z.ID == "" || seen[z.ID] || z.Volume < 0 || z.Volume > 100 || (z.ChannelMode != "mono" && z.ChannelMode != "stereo") {
			return errors.New("zona inválida")
		}
		if r.audio == nil && z.Output != nil {
			return errors.New("esta etapa no admite salidas de audio; configure output=null")
		}
		if z.Playlist != nil && (z.Playlist.ID == "" || z.Playlist.Songs == nil) {
			return errors.New("playlist inválida")
		}
		seen[z.ID] = true
	}
	if r.audio != nil {
		if r.client != nil && r.client.profile != "" && c.Profile != r.client.profile {
			return errors.New("perfil diferente al negociado")
		}
		if err := validateAudio(c); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.Revision < r.config.Revision {
		return errors.New("revisión regresiva")
	}
	if r.audio != nil {
		if c.Revision == r.config.Revision && !sameSnapshot(c, r.config) {
			return errors.New("contenido diferente para la misma revision")
		}
		if err := r.applyAudio(c); err != nil {
			return err
		}
	}
	r.config = c
	r.configError = nil
	return nil
}

func (r *Runtime) report() Report {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sequence++
	report := Report{Sequence: r.sequence, ObservedAt: time.Now().UTC(), Revision: r.config.Revision, ConfigError: r.configError, Zones: []ZoneState{}}
	for _, z := range r.config.Zones {
		state := ZoneState{ZoneID: z.ID, State: "stopped", Volume: z.Volume, ChannelMode: &z.ChannelMode}
		if z.Playlist != nil {
			state.PlaylistID = &z.Playlist.ID
		}
		if r.audio != nil {
			r.audioReport(z, &state)
		}
		report.Zones = append(report.Zones, state)
	}
	return report
}

func failed(code, message string) Result {
	return Result{"failed", time.Now().UTC(), &Failure{code, message}}
}

func (r *Runtime) execute(cmd Command) Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cmd.ExpiresAt.IsZero() || !cmd.ExpiresAt.After(time.Now()) {
		return failed("command_expired", "Orden vencida")
	}
	var zone *Zone
	for i := range r.config.Zones {
		if r.config.Zones[i].ID == cmd.ZoneID {
			zone = &r.config.Zones[i]
			break
		}
	}
	if zone == nil {
		return failed("zone_not_assigned", "Zona no asignada")
	}
	if cmd.Revision != r.config.Revision {
		return failed("config_revision_mismatch", "Revisión distinta de la aplicada")
	}
	if r.audio != nil {
		if failure := validateSongSelection(cmd, zone); failure != nil {
			return failed(failure.Code, failure.Message)
		}
		return r.executeAudio(cmd, zone)
	}
	if cmd.Action != "stop" {
		return failed("unsupported_action", "Engine de configuración sin reproducción de audio")
	}
	return Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
}

func (r *Runtime) cycle(ctx context.Context) error {
	config, err := r.client.Configuration(ctx)
	if err != nil {
		return err
	}
	applyErr := r.apply(config)
	if applyErr != nil {
		r.mu.Lock()
		if r.configError == nil || r.configError.Revision != config.Revision || r.configError.Message != applyErr.Error() {
			log.Printf("Configuracion de audio rechazada (revision %d): %v", config.Revision, applyErr)
		}
		r.configError = &ConfigError{config.Revision, "unsupported_configuration", applyErr.Error()}
		r.mu.Unlock()
	}
	if r.audio != nil {
		if err := r.finishPending(ctx, applyErr == nil); err != nil {
			return err
		}
	}
	commands, err := r.client.Commands(ctx)
	if err != nil {
		return err
	}
	var previous int64
	for _, cmd := range commands {
		if cmd.ID == "" || cmd.Sequence <= previous {
			return errors.New("lote de órdenes inválido")
		}
		previous = cmd.Sequence
		result, seen := r.journal.results[cmd.ID]
		if !seen {
			if r.audio != nil {
				if err := r.supersede(cmd); err != nil {
					return err
				}
			}
			if err := r.journal.Record(cmd.ID, nil); err != nil {
				return fmt.Errorf("persistir inicio: %w", err)
			}
			executed := r.execute(cmd)
			if executed.Outcome == "pending" {
				continue
			}
			result = &executed
			if err := r.journal.Record(cmd.ID, result); err != nil {
				return fmt.Errorf("persistir resultado: %w", err)
			}
		} else if result == nil {
			r.mu.Lock()
			pending := r.pending[cmd.ID] != nil
			r.mu.Unlock()
			if pending {
				continue
			}
			unknown := failed("execution_unknown", "El proceso terminó sin persistir el resultado; no se repite la acción")
			result = &unknown
			if err := r.journal.Record(cmd.ID, result); err != nil {
				return err
			}
		}
		if err := r.client.Confirm(ctx, cmd.ID, *result); err != nil {
			return err
		}
	}
	return nil
}

func wait(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func delay(attempt int, err error) time.Duration {
	d := time.Second * time.Duration(1<<min(attempt, 5))
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	d += time.Duration(rand.Float64() * 0.2 * float64(d))
	var api *APIError
	if errors.As(err, &api) && api.RetryAfter > d {
		d = api.RetryAfter
	}
	return d
}

func Run(ctx context.Context, server, token, version, stateDir string) error {
	return RunProfile(ctx, server, token, version, stateDir, "configuration_only")
}

func RunProfile(ctx context.Context, server, token, version, stateDir, profile string) error {
	return RunProfileWithOutput(ctx, server, token, version, stateDir, profile, nil)
}

func RunProfileWithOutput(ctx context.Context, server, token, version, stateDir, profile string, output audio.Output) error {
	if profile != "configuration_only" && profile != PlaybackProfile && profile != MonoProfile {
		return errors.New("ENGINE_PROFILE invalido")
	}
	client, err := NewClient(server, token)
	if err != nil {
		return err
	}
	client.profile = profile
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return err
	}
	processLock, err := os.OpenFile(filepath.Join(stateDir, "agent.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer processLock.Close()
	if err := lockJournal(processLock); err != nil {
		return errors.New("ya hay un engine usando ENGINE_STATE_DIR")
	}
	var session Session
	for attempt := 0; ; attempt++ {
		session, err = client.Open(ctx, version)
		if err == nil {
			break
		}
		if ctx.Err() != nil {
			return nil
		}
		if terminal(err) {
			return err
		}
		log.Printf("No se pudo abrir sesión; se reintentará: %v", err)
		if !wait(ctx, delay(attempt, err)) {
			return nil
		}
	}
	journal, err := OpenJournal(stateDir, client.base+"|"+session.DeviceID)
	if err != nil {
		return err
	}
	defer journal.Close()
	r := &Runtime{client: client, journal: journal, deviceID: session.DeviceID}
	if profile == PlaybackProfile || profile == MonoProfile {
		media := decoder.HTTPS{Client: &http.Client{Transport: client.http.Transport}}
		if output == nil && profile == MonoProfile {
			r.audio, err = engine.NewRemoteMono(media)
		} else if output == nil {
			r.audio, err = engine.NewRemote(media)
		} else if profile == MonoProfile {
			r.audio, err = engine.NewRemoteMonoWithOutput(output, media)
		} else {
			r.audio, err = engine.NewRemoteWithOutput(output, media)
		}
		if err != nil {
			return err
		}
		defer r.audio.Close()
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	fatal := make(chan error, 2)
	wake := make(chan struct{}, 1)
	reportNow := make(chan struct{}, 1)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	if r.audio != nil {
		workers.Go(func() {
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			previous := map[string]string{}
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					r.mu.Lock()
					changed := false
					for _, id := range r.audio.IDs() {
						p, _ := r.audio.Zone(id)
						status := p.GetState().Status
						if previous[id] != status {
							changed = true
							previous[id] = status
						}
					}
					r.mu.Unlock()
					if changed {
						select {
						case wake <- struct{}{}:
						default:
						}
						select {
						case reportNow <- struct{}{}:
						default:
						}
					}
				}
			}
		})
	}
	workers.Go(func() {
		for {
			report := r.report()
			for attempt := 0; ; attempt++ {
				err := client.Heartbeat(ctx, report)
				if err == nil {
					break
				}
				if terminal(err) {
					fatal <- err
					return
				}
				if !wait(ctx, delay(attempt, err)) {
					return
				}
			}
			timer := time.NewTimer(time.Duration(session.HeartbeatSeconds) * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-reportNow:
				timer.Stop()
			case <-timer.C:
			}
		}
	})
	if session.WebSocket != nil {
		workers.Go(func() { client.watch(ctx, *session.WebSocket, wake, fatal) })
	}
	log.Printf("Engine %s conectado; perfil %s", session.DeviceID, profile)
	for attempt := 0; ; {
		err := r.cycle(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if terminal(err) {
			return err
		}
		period := time.Duration(session.PollSeconds) * time.Second
		if err != nil {
			// Persistence failures are fatal. Retrying after a failed fsync could replay an action.
			var pathErr *PersistenceError
			if errors.As(err, &pathErr) {
				return err
			}
			period = delay(attempt, err)
			attempt++
			log.Printf("Sincronización pendiente: %v", err)
		} else {
			attempt = 0
			select {
			case reportNow <- struct{}{}:
			default:
			}
		}
		timer := time.NewTimer(period)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case err := <-fatal:
			timer.Stop()
			return err
		case <-wake:
			timer.Stop()
		case <-timer.C:
		}
	}
}
