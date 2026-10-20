package control

import "time"

func (r *Runtime) desktopResult(id string, result Result) {
	if r.observer != nil {
		r.observer(DesktopEvent{Type: "command_result", RequestID: id, Result: &result, At: time.Now().UTC()})
	}
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
