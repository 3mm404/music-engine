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
