package control

import (
	"context"
	"net/url"
	"time"
)

// Assignment and the durable play command are committed together by Laravel.
func (r *Runtime) playDesktopPlaylist(request DesktopRequest) Result {
	if request.PlaylistID == nil || *request.PlaylistID == "" || (request.SongID != nil && *request.SongID == "") || request.Volume != nil {
		return failed("invalid_playlist", "Seleccione una playlist")
	}
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
		return failed("unsupported_action", "Servidor no disponible")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var config Config
	body := map[string]any{"config_revision": request.Revision, "playlist_id": *request.PlaylistID}
	if request.SongID != nil {
		body["song_id"] = *request.SongID
	}
	if err := r.client.request(ctx, "PUT", "/zones/"+url.PathEscape(request.ZoneID)+"/playlist", body, &config, true); err != nil {
		return failed("playlist_update_failed", err.Error())
	}
	if err := r.apply(config); err != nil {
		return failed("unsupported_configuration", err.Error())
	}
	return Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
}
