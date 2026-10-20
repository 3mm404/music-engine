package control

import "time"

// setDesktopVolume valida y aplica 0–100 a audio 0–1 bajo r.mu. El override dura hasta una nueva revisión del servidor o cierre del proceso.
func (r *Runtime) setDesktopVolume(request DesktopRequest) Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	if request.Volume == nil || *request.Volume < 0 || *request.Volume > 100 || request.SongID != nil {
		return failed("invalid_volume", "El volumen debe ser un entero entre 0 y 100")
	}
	if request.Revision != r.config.Revision || r.configError != nil {
		return failed("config_revision_mismatch", "La configuracion cambio; actualice el estado")
	}
	if r.audio == nil {
		return failed("unsupported_action", "Motor sin salida de audio")
	}
	for _, zone := range r.config.Zones {
		if zone.ID != request.ZoneID {
			continue
		}
		player, err := r.audio.Zone(zone.ID)
		if err != nil {
			return failed("zone_not_assigned", "Zona no disponible")
		}
		if err := player.SetVolume(float64(*request.Volume) / 100); err != nil {
			return failed("volume_failed", "No se pudo aplicar el volumen")
		}
		if r.localVolumes == nil {
			r.localVolumes = map[string]int{}
		}
		r.localVolumes[zone.ID] = *request.Volume
		return Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
	}
	return failed("zone_not_assigned", "Zona no asignada")
}

// Caller holds r.mu. Overrides last only for the current server revision.
func (r *Runtime) zoneVolume(zone Zone) int {
	if volume, ok := r.localVolumes[zone.ID]; ok {
		return volume
	}
	return zone.Volume
}
