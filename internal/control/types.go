package control

import "time"

type Session struct {
	DeviceID         string           `json:"device_id"`
	SessionID        string           `json:"session_id"`
	HeartbeatSeconds int              `json:"heartbeat_interval_seconds"`
	PollSeconds      int              `json:"poll_interval_seconds"`
	WebSocket        *WebSocketConfig `json:"websocket"`
}

type WebSocketConfig struct {
	URL     string `json:"url"`
	Key     string `json:"key"`
	Channel string `json:"channel"`
}

type Config struct {
	Profile  string `json:"profile"`
	DeviceID string `json:"device_id"`
	Revision int64  `json:"config_revision"`
	Zones    []Zone `json:"zones"`
}

type Zone struct {
	ID                 string    `json:"zone_id"`
	Name               string    `json:"name"`
	Volume             int       `json:"volume"`
	ChannelMode        string    `json:"channel_mode"`
	Output             *Output   `json:"output"`
	Playlist           *Playlist `json:"playlist"`
	Shuffle            bool      `json:"shuffle"`
	RepeatMode         string    `json:"repeat_mode"`
	ContinuousPlayback bool      `json:"continuous_playback"`
	Schedule           *Schedule `json:"schedule"`
}

// Schedule is descriptive. Laravel remains the sole scheduler and command authority.
type Schedule struct {
	ID               string         `json:"schedule_id"`
	Name             string         `json:"name"`
	Timezone         string         `json:"timezone"`
	StopOutsideSlots bool           `json:"stop_outside_slots"`
	Slots            []ScheduleSlot `json:"slots"`
}

type ScheduleSlot struct {
	ID              string `json:"slot_id"`
	Days            []int  `json:"days"`
	StartsAt        string `json:"starts_at"`
	EndsAt          string `json:"ends_at"`
	CrossesMidnight bool   `json:"crosses_midnight"`
	PlaylistID      string `json:"playlist_id"`
	PlaylistName    string `json:"playlist_name"`
	Volume          *int   `json:"volume"`
}

type Output struct {
	DeviceID string `json:"device_id"`
	Channels []int  `json:"channels"`
}

type Playlist struct {
	ID    string `json:"playlist_id"`
	Name  string `json:"name"`
	Songs []Song `json:"songs"`
}

type Song struct {
	ID             string      `json:"song_id"`
	Title          string      `json:"title"`
	Artist         *string     `json:"artist"`
	URL            string      `json:"audio_url"`
	ContentVersion string      `json:"content_version"`
	Format         AudioFormat `json:"format"`
}

type AudioFormat struct {
	Container string `json:"container"`
	Codec     string `json:"codec"`
	Mime      string `json:"mime_type"`
	Rate      int    `json:"sample_rate_hz"`
	Channels  int    `json:"channels"`
}

type Command struct {
	ID        string    `json:"command_id"`
	Sequence  int64     `json:"sequence"`
	ZoneID    string    `json:"zone_id"`
	Revision  int64     `json:"config_revision"`
	Action    string    `json:"action"`
	SongID    *string   `json:"song_id,omitempty"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Result struct {
	Outcome     string    `json:"outcome"`
	CompletedAt time.Time `json:"completed_at"`
	Error       *Failure  `json:"error"`
}

type ConfigError struct {
	Revision int64  `json:"revision"`
	Code     string `json:"code"`
	Message  string `json:"message"`
}

type ZoneState struct {
	StopReason  string   `json:"stop_reason,omitempty"`
	ZoneID      string   `json:"zone_id"`
	State       string   `json:"state"`
	PlaylistID  *string  `json:"playlist_id"`
	SongID      *string  `json:"song_id"`
	PositionMS  *int     `json:"position_ms"`
	Volume      int      `json:"volume"`
	ChannelMode *string  `json:"channel_mode"`
	Output      *Output  `json:"output"`
	Error       *Failure `json:"error"`
}

type Report struct {
	Sequence    int64        `json:"report_sequence"`
	ObservedAt  time.Time    `json:"observed_at"`
	Revision    int64        `json:"applied_config_revision"`
	ConfigError *ConfigError `json:"config_error"`
	Zones       []ZoneState  `json:"zones"`
}
