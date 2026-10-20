package control

import "time"

// report construye el heartbeat bajo r.mu. Conserva el volumen configurado que valida Laravel; Desktop expone el volumen efectivo por otro snapshot.
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
