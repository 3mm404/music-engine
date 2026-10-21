package control

import "time"

// Called serially with server cycles. Local requests never enter the server journal.
// executeDesktop deduplica por sesión y despacha solo capacidades registradas. Las órdenes locales no usan el diario durable del servidor.
func (r *Runtime) executeDesktop(request DesktopRequest) {
	if request.RequestID == "" || len(request.RequestID) > 128 {
		return
	}
	if result, exists := r.desktopResults[request.RequestID]; exists {
		r.desktopResult(request.RequestID, result)
		return
	}
	if r.desktopResults == nil {
		r.desktopResults = map[string]Result{}
	}
	if len(r.desktopResults) >= 4096 {
		r.desktopResult(request.RequestID, failed("session_limit", "Reinicie el motor para abrir otra sesion de control"))
		return
	}
	operation, registered := findDesktopOperation(request.Action)
	if !registered {
		result := failed("unsupported_action", "Accion desktop no admitida")
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	if (operation.handler != seekHandler && request.PositionMS != nil) ||
		(operation.handler != modesHandler && (request.Shuffle != nil || request.RepeatMode != nil || request.ContinuousPlayback != nil)) {
		result := failed("invalid_parameters", "Parametros ajenos a la operacion")
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	if operation.handler == volumeHandler {
		result := r.setDesktopVolume(request)
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	if operation.handler == seekHandler || operation.handler == modesHandler {
		var result Result
		if operation.handler == seekHandler {
			result = r.seekDesktop(request)
		} else {
			result = r.setDesktopModes(request)
		}
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	if request.Volume != nil {
		result := failed("invalid_volume", "Solo volume admite el parametro volume")
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	cmd := Command{ID: "desktop:" + request.RequestID, ZoneID: request.ZoneID, Revision: request.Revision, Action: request.Action, SongID: request.SongID, ExpiresAt: time.Now().Add(30 * time.Second)}
	if err := r.supersede(cmd); err != nil {
		result := failed("persistence_failed", "No se pudo reemplazar la orden anterior")
		r.desktopResults[request.RequestID] = result
		r.desktopResult(request.RequestID, result)
		return
	}
	result := r.execute(cmd)
	if result.Outcome == "pending" {
		r.mu.Lock()
		r.pending[cmd.ID].desktopRequestID = request.RequestID
		r.mu.Unlock()
	}
	r.desktopResults[request.RequestID] = result
	r.desktopResult(request.RequestID, result)
}
