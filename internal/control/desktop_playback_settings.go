package control

import (
	"context"
	"net/url"
	"time"
)

func (r *Runtime) seekDesktop(request DesktopRequest) Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	if request.PositionMS == nil || request.Volume != nil || request.SongID != nil || request.Shuffle != nil || request.RepeatMode != nil || request.ContinuousPlayback != nil {
		return failed("invalid_position", "seek requiere position_ms")
	}
	if request.Revision != r.config.Revision || r.configError != nil {
		return failed("config_revision_mismatch", "Actualice el estado")
	}
	if r.audio == nil {
		return failed("unsupported_action", "Motor sin audio")
	}
	p, err := r.audio.Zone(request.ZoneID)
	if err != nil {
		return failed("zone_not_assigned", "Zona no asignada")
	}
	if err := p.SeekMS(*request.PositionMS); err != nil {
		return failed("seek_failed", err.Error())
	}
	return Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
}

// Laravel persists and decides playback order; local modes are never overridden optimistically.
func (r *Runtime) setDesktopModes(request DesktopRequest) Result {
	if request.Volume != nil || request.SongID != nil || request.PositionMS != nil || (request.Shuffle == nil && request.RepeatMode == nil && request.ContinuousPlayback == nil) {
		return failed("invalid_modes", "Indique los modos de reproduccion")
	}
	if request.RepeatMode != nil && *request.RepeatMode != "off" && *request.RepeatMode != "track" && *request.RepeatMode != "playlist" {
		return failed("invalid_modes", "Repeticion invalida")
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
	body := map[string]any{"config_revision": request.Revision}
	if request.Shuffle != nil {
		body["shuffle"] = *request.Shuffle
	}
	if request.RepeatMode != nil {
		body["repeat_mode"] = *request.RepeatMode
	}
	if request.ContinuousPlayback != nil {
		body["continuous_playback"] = *request.ContinuousPlayback
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var config Config
	if err := r.client.request(ctx, "PUT", "/zones/"+url.PathEscape(request.ZoneID)+"/playback-modes", body, &config, true); err != nil {
		return failed("modes_update_failed", err.Error())
	}
	if err := r.apply(config); err != nil {
		return failed("unsupported_configuration", err.Error())
	}
	return Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
}
