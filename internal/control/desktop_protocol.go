package control

import "time"

// DesktopEvent deliberately excludes tokens, signed URLs and session secrets.
type DesktopEvent struct {
	ServerControl bool                `json:"server_control"`
	Performance   *DesktopPerformance `json:"performance,omitempty"`
	Playlists     []DesktopPlaylist   `json:"playlists,omitempty"`
	Type          string              `json:"type"`
	Channel       string              `json:"channel,omitempty"`
	Connected     bool                `json:"connected"`
	Message       string              `json:"message,omitempty"`
	At            time.Time           `json:"at"`
	Report        *Report             `json:"report,omitempty"`
	Zones         []DesktopZone       `json:"zones,omitempty"`
	RequestID     string              `json:"request_id,omitempty"`
	Result        *Result             `json:"result,omitempty"`
}

type DesktopZone struct {
	BackgroundPath     string            `json:"background_path,omitempty"`
	SongPreferences    map[string]string `json:"song_preferences,omitempty"`
	PlaylistID         string            `json:"playlist_id,omitempty"`
	CoverPath          string            `json:"cover_path,omitempty"`
	ID                 string            `json:"zone_id"`
	Name               string            `json:"name"`
	Playlist           string            `json:"playlist"`
	Songs              []string          `json:"songs"`
	PositionMS         *int              `json:"position_ms,omitempty"`
	DurationMS         *int              `json:"duration_ms,omitempty"`
	SongMetadata       []DesktopSong     `json:"song_metadata"`
	Shuffle            bool              `json:"shuffle"`
	RepeatMode         string            `json:"repeat_mode"`
	ContinuousPlayback bool              `json:"continuous_playback"`
}

// DesktopSong exposes display metadata without private audio URLs or grants.
type DesktopSong struct {
	CoverPath string  `json:"cover_path,omitempty"`
	ID        string  `json:"song_id"`
	Title     string  `json:"title"`
	Artist    *string `json:"artist"`
}

// DesktopRequest is scoped to this process and the currently applied revision.
type DesktopRequest struct {
	Preference         *string `json:"preference,omitempty"`
	PlaylistID         *string `json:"playlist_id,omitempty"`
	RequestID          string  `json:"request_id"`
	ZoneID             string  `json:"zone_id"`
	Revision           int64   `json:"config_revision"`
	Action             string  `json:"action"`
	SongID             *string `json:"song_id,omitempty"`
	Volume             *int    `json:"volume,omitempty"`
	PositionMS         *int    `json:"position_ms,omitempty"`
	Shuffle            *bool   `json:"shuffle,omitempty"`
	RepeatMode         *string `json:"repeat_mode,omitempty"`
	ContinuousPlayback *bool   `json:"continuous_playback,omitempty"`
}

type DesktopPlaylist struct {
	ID        string        `json:"playlist_id"`
	Name      string        `json:"name"`
	CoverPath string        `json:"cover_path,omitempty"`
	Songs     []DesktopSong `json:"songs"`
}
