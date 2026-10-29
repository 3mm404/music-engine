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
	event.ServerControl = r.config.ServerControl
	event.Performance = r.telemetry.sample(time.Now())
	for _, playlist := range r.config.Playlists {
		entry := DesktopPlaylist{ID: playlist.ID, Name: playlist.Name, CoverPath: r.coverPaths[playlist.CoverURL], Songs: []DesktopSong{}}
		for _, song := range playlist.Songs {
			coverURL := song.CoverURL
			if coverURL == "" {
				coverURL = playlist.CoverURL
			}
			entry.Songs = append(entry.Songs, DesktopSong{ID: song.ID, Title: song.Title, Artist: song.Artist, CoverPath: r.coverPaths[coverURL]})
		}
		event.Playlists = append(event.Playlists, entry)
	}
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
		metadata := []DesktopSong{}
		if zone.Playlist != nil {
			for _, song := range zone.Playlist.Songs {
				songs = append(songs, song.ID)
				coverURL := song.CoverURL
				if coverURL == "" {
					coverURL = zone.Playlist.CoverURL
				}
				metadata = append(metadata, DesktopSong{ID: song.ID, Title: song.Title, Artist: song.Artist, CoverPath: r.coverPaths[coverURL]})
			}
		}
		desktopZone := DesktopZone{BackgroundPath: r.coverPaths[r.config.ZoneBackgrounds[zone.ID]], ID: zone.ID, Name: zone.Name, Playlist: name, Songs: songs,
			SongPreferences: r.config.SongPreferences[zone.ID],
			SongMetadata:    metadata, Shuffle: zone.Shuffle, RepeatMode: zone.RepeatMode, ContinuousPlayback: zone.ContinuousPlayback}
		if zone.Playlist != nil {
			desktopZone.CoverPath = r.coverPaths[zone.Playlist.CoverURL]
			desktopZone.PlaylistID = zone.Playlist.ID
		}
		if r.audio != nil {
			if player, err := r.audio.Zone(zone.ID); err == nil {
				state := player.GetState()
				desktopZone.PositionMS, desktopZone.DurationMS = state.PositionMS, state.DurationMS
			}
		}
		event.Zones = append(event.Zones, desktopZone)
	}
	return event
}
