package control

import (
	"context"
	"errors"
	"log"
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
	observer       func(DesktopEvent)
	coverPaths     map[string]string
	telemetry      performanceSampler
	desktopResults map[string]Result
	localVolumes   map[string]int
	sharedStereo   bool
	client         *Client
	journal        *Journal
	deviceID       string
	mu             sync.Mutex
	config         Config
	configError    *ConfigError
	sequence       int64
	audio          *engine.Manager
	selected       map[string]int
	pending        map[string]*pendingPlayback
}

func Run(ctx context.Context, server, token, version, stateDir string) error {
	return RunProfile(ctx, server, token, version, stateDir, "configuration_only")
}

func RunProfile(ctx context.Context, server, token, version, stateDir, profile string) error {
	return RunProfileWithOutput(ctx, server, token, version, stateDir, profile, nil)
}

func RunProfileWithOutput(ctx context.Context, server, token, version, stateDir, profile string, output audio.Output) error {
	return RunProfileObserved(ctx, server, token, version, stateDir, profile, output, nil)
}

func RunProfileObserved(ctx context.Context, server, token, version, stateDir, profile string, output audio.Output, observer func(DesktopEvent)) error {
	return RunProfileDesktop(ctx, server, token, version, stateDir, profile, output, observer, nil)
}

// RunProfileDesktop compone sesión, diario y audio; posee todos sus recursos y espera a los workers antes de cerrar.
func RunProfileDesktop(ctx context.Context, server, token, version, stateDir, profile string, output audio.Output, observer func(DesktopEvent), requests <-chan DesktopRequest) error {
	notify := func(channel string, err error) {
		if observer != nil {
			event := DesktopEvent{Type: "connection", Channel: channel, Connected: err == nil, At: time.Now().UTC()}
			if err != nil {
				event.Message = err.Error()
			}
			observer(event)
		}
	}
	if profile != "configuration_only" && profile != PlaybackProfile && profile != MonoProfile {
		return errors.New("ENGINE_PROFILE invalido")
	}
	// 1. Preparación: cliente autenticado y bloqueo exclusivo del directorio de estado.
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
	// 2. Sesión: reintenta errores recuperables y termina ante rechazo definitivo.
	var session Session
	for attempt := 0; ; attempt++ {
		session, err = client.Open(ctx, version)
		notify("session", err)
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
	// 3. Recursos de ejecución: diario durable y reproductores por zona.
	journal, err := OpenJournal(stateDir, client.base+"|"+session.DeviceID)
	if err != nil {
		return err
	}
	defer journal.Close()
	r := &Runtime{client: client, journal: journal, deviceID: session.DeviceID, sharedStereo: audio.IsSharedStereo(output), observer: observer}
	if observer != nil {
		coverCtx, cancelCovers := context.WithCancel(ctx)
		coversDone := make(chan struct{})
		go func() {
			defer close(coversDone)
			r.cacheCovers(coverCtx, filepath.Join(stateDir, "covers"))
		}()
		defer func() { cancelCovers(); <-coversDone }()
	}
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
	// 4. Workers: observación Desktop, cambios de audio, heartbeat y WebSocket.
	// Solo el bucle principal procesa ciclos del servidor y órdenes locales.
	fatal := make(chan error, 2)
	wake := make(chan struct{}, 1)
	reportNow := make(chan struct{}, 1)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	if observer != nil {
		workers.Go(func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					observer(r.desktopSnapshot())
				}
			}
		})
	}
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
				notify("heartbeat", err)
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
	// 5. Bucle principal: sincronización, errores y despacho local serializado.
	log.Printf("Engine %s conectado; perfil %s", session.DeviceID, profile)
	for attempt := 0; ; {
		err := r.cycle(ctx)
		notify("sync", err)
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
		case request := <-requests:
			timer.Stop()
			r.executeDesktop(request)
			if observer != nil {
				observer(r.desktopSnapshot())
			}
		case <-timer.C:
		}
	}
}
