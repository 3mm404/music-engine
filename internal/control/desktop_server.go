package control

import (
	"context"
	"net/url"
	"time"
)

// The backend owns song preferences and all selection decisions. This adapter
// only forwards a scoped request and applies the returned configuration.
func (r *Runtime) forwardDesktop(request DesktopRequest) Result {
	r.mu.Lock()
	valid := request.Revision == r.config.Revision && r.configError == nil
	assigned := false
	for _, zone := range r.config.Zones {
		if zone.ID == request.ZoneID {
			assigned = true
		}
	}
	r.mu.Unlock()
	if !valid {
		return failed("config_revision_mismatch", "Actualice el estado")
	}
	if !assigned {
		return failed("zone_not_assigned", "Zona no asignada")
	}
	if r.client == nil {
		return failed("server_unavailable", "Servidor no disponible")
	}
	path := "/zones/" + url.PathEscape(request.ZoneID) + "/transport"
	body := map[string]any{"config_revision": request.Revision, "action": request.Action}
	for _, state := range r.report().Zones {
		if state.ZoneID == request.ZoneID {
			body["current_song_id"] = state.SongID
			body["current_state"] = state.State
			break
		}
	}
	if request.Action == "song_preference" {
		path = "/zones/" + url.PathEscape(request.ZoneID) + "/song-preference"
		delete(body, "action")
		if request.Preference != nil {
			body["preference"] = *request.Preference
		}
	}
	if request.SongID != nil {
		body["song_id"] = *request.SongID
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var config Config
	if err := r.client.request(ctx, "PUT", path, body, &config, true); err != nil {
		return failed("server_request_failed", err.Error())
	}
	if err := r.apply(config); err != nil {
		return failed("unsupported_configuration", err.Error())
	}
	return Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
}
