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
}

type DesktopZone struct {
	ID       string `json:"zone_id"`
	Name     string `json:"name"`
	Playlist string `json:"playlist"`
}

func (r *Runtime) desktopSnapshot() DesktopEvent {
	report := r.report()
	r.mu.Lock()
	defer r.mu.Unlock()
	event := DesktopEvent{Type: "status", At: time.Now().UTC(), Report: &report, Zones: []DesktopZone{}}
	for _, zone := range r.config.Zones {
		name := ""
		if zone.Playlist != nil {
			name = zone.Playlist.Name
		}
		event.Zones = append(event.Zones, DesktopZone{zone.ID, zone.Name, name})
	}
	return event
}
