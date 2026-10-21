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
	ID                 string        `json:"zone_id"`
	Name               string        `json:"name"`
	Playlist           string        `json:"playlist"`
	Songs              []string      `json:"songs"`
	PositionMS         *int          `json:"position_ms,omitempty"`
	DurationMS         *int          `json:"duration_ms,omitempty"`
	SongMetadata       []DesktopSong `json:"song_metadata"`
	Shuffle            bool          `json:"shuffle"`
	RepeatMode         string        `json:"repeat_mode"`
	ContinuousPlayback bool          `json:"continuous_playback"`
	Schedule           *Schedule     `json:"schedule"`
}

// DesktopSong exposes display metadata without private audio URLs or grants.
type DesktopSong struct {
	ID     string  `json:"song_id"`
	Title  string  `json:"title"`
	Artist *string `json:"artist"`
}

// DesktopRequest is scoped to this process and the currently applied revision.
type DesktopRequest struct {
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
