package control

import "time"

func failed(code, message string) Result {
	return Result{"failed", time.Now().UTC(), &Failure{code, message}}
}

// execute verifica vencimiento, zona y revisión bajo r.mu; delega reproducción sin confirmar anticipadamente cargas pendientes.
func (r *Runtime) execute(cmd Command) Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cmd.ExpiresAt.IsZero() || !cmd.ExpiresAt.After(time.Now()) {
		return failed("command_expired", "Orden vencida")
	}
	var zone *Zone
	for i := range r.config.Zones {
		if r.config.Zones[i].ID == cmd.ZoneID {
			zone = &r.config.Zones[i]
			break
		}
	}
	if zone == nil {
		return failed("zone_not_assigned", "Zona no asignada")
	}
	if cmd.Revision != r.config.Revision {
		return failed("config_revision_mismatch", "Revisión distinta de la aplicada")
	}
	if r.audio != nil {
		if failure := validateSongSelection(cmd, zone); failure != nil {
			return failed(failure.Code, failure.Message)
		}
		return r.executeAudio(cmd, zone)
	}
	if cmd.Action != "stop" {
		return failed("unsupported_action", "Engine de configuración sin reproducción de audio")
	}
	return Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
}
