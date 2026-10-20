package control

import "time"

// DesktopEvent deliberately excludes tokens, signed URLs and session secrets.
type DesktopEvent struct {
	Type      string        `json:"type"`
	Channel   string        `json:"channel,omitempty"`
	Connected bool          `json:"connected"`
	Message   string        `json:"message,omitempty"`
	At        time.Time     `json:"at"`
	Report    *Report       `json:"report,omitempty"`
	Zones     []DesktopZone `json:"zones,omitempty"`
	RequestID string        `json:"request_id,omitempty"`
	Result    *Result       `json:"result,omitempty"`
}

type DesktopZone struct {
	ID       string   `json:"zone_id"`
	Name     string   `json:"name"`
	Playlist string   `json:"playlist"`
	Songs    []string `json:"songs"`
}

// DesktopRequest is scoped to this process and the currently applied revision.
type DesktopRequest struct {
	RequestID string  `json:"request_id"`
	ZoneID    string  `json:"zone_id"`
	Revision  int64   `json:"config_revision"`
	Action    string  `json:"action"`
	SongID    *string `json:"song_id,omitempty"`
	Volume    *int    `json:"volume,omitempty"`
}

func DesktopCapabilities() []string {
	return []string{"play", "pause", "resume", "next", "previous", "playlist_read", "volume"}
}

func (r *Runtime) desktopResult(id string, result Result) {
	if r.observer != nil {
		r.observer(DesktopEvent{Type: "command_result", RequestID: id, Result: &result, At: time.Now().UTC()})
	}
}

// Called serially with server cycles. Local requests never enter the server journal.
func (r *Runtime) executeDesktop(request DesktopRequest) {
	if request.RequestID == "" || len(request.RequestID) > 128 {
		return
	}
	if result, exists := r.desktopResults[request.RequestID]; exists {
		r.desktopResult(request.RequestID, result)
		return
	}
	if r.desktopResults == nil {
		r.desktopResults = map[string]Result{}
	}
	if len(r.desktopResults) >= 4096 {
		r.desktopResult(request.RequestID, failed("session_limit", "Reinicie el motor para abrir otra sesion de control"))
		return
	}
	if request.Action == "volume" {
		result := r.setDesktopVolume(request)
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	if request.Volume != nil {
		result := failed("invalid_volume", "Solo volume admite el parametro volume")
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	if request.Action != "play" && request.Action != "pause" && request.Action != "resume" && request.Action != "next" && request.Action != "previous" {
		result := failed("unsupported_action", "Accion desktop no admitida")
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	cmd := Command{ID: "desktop:" + request.RequestID, ZoneID: request.ZoneID, Revision: request.Revision, Action: request.Action, SongID: request.SongID, ExpiresAt: time.Now().Add(30 * time.Second)}
	if err := r.supersede(cmd); err != nil {
		result := failed("persistence_failed", "No se pudo reemplazar la orden anterior")
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	result := r.execute(cmd)
	if result.Outcome == "pending" {
		r.mu.Lock()
		r.pending[cmd.ID].desktopRequestID = request.RequestID
		r.mu.Unlock()
	}
	r.desktopResults[request.RequestID] = result
	r.desktopResult(request.RequestID, result)
}

func (r *Runtime) setDesktopVolume(request DesktopRequest) Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	if request.Volume == nil || *request.Volume < 0 || *request.Volume > 100 || request.SongID != nil {
		return failed("invalid_volume", "El volumen debe ser un entero entre 0 y 100")
	}
	if request.Revision != r.config.Revision || r.configError != nil {
		return failed("config_revision_mismatch", "La configuracion cambio; actualice el estado")
	}
	if r.audio == nil {
		return failed("unsupported_action", "Motor sin salida de audio")
	}
	for _, zone := range r.config.Zones {
		if zone.ID != request.ZoneID {
			continue
		}
		player, err := r.audio.Zone(zone.ID)
		if err != nil {
			return failed("zone_not_assigned", "Zona no disponible")
		}
		if err := player.SetVolume(float64(*request.Volume) / 100); err != nil {
			return failed("volume_failed", "No se pudo aplicar el volumen")
		}
		if r.localVolumes == nil {
			r.localVolumes = map[string]int{}
		}
		r.localVolumes[zone.ID] = *request.Volume
		return Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
	}
	return failed("zone_not_assigned", "Zona no asignada")
}

// Caller holds r.mu. Overrides last only for the current server revision.
func (r *Runtime) zoneVolume(zone Zone) int {
	if volume, ok := r.localVolumes[zone.ID]; ok {
		return volume
	}
	return zone.Volume
}

// r.mu must be held; server results retain their durable journal semantics.
func (r *Runtime) completePending(id string, pending *pendingPlayback, result Result) error {
	if pending.desktopRequestID != "" {
		r.desktopResults[pending.desktopRequestID] = result
		r.desktopResult(pending.desktopRequestID, result)
		return nil
	}
	return r.journal.Record(id, &result)
}

func (r *Runtime) desktopSnapshot() DesktopEvent {
	report := r.report()
	r.mu.Lock()
	defer r.mu.Unlock()
	event := DesktopEvent{Type: "status", At: time.Now().UTC(), Report: &report, Zones: []DesktopZone{}}
	// Laravel's heartbeat contract validates configured volume against its revision.
	// Effective local volume is exposed only on the desktop observation channel.
	for i := range report.Zones {
		if volume, ok := r.localVolumes[report.Zones[i].ZoneID]; ok {
			report.Zones[i].Volume = volume
		}
	}
	for _, zone := range r.config.Zones {
		name := ""
		if zone.Playlist != nil {
			name = zone.Playlist.Name
		}
		songs := []string{}
		if zone.Playlist != nil {
			for _, song := range zone.Playlist.Songs {
				songs = append(songs, song.ID)
			}
		}
		event.Zones = append(event.Zones, DesktopZone{ID: zone.ID, Name: zone.Name, Playlist: name, Songs: songs})
	}
	return event
}
