package control

import "errors"

// apply valida la instantánea completa antes de cambiar audio y configuración. La revisión nueva invalida el volumen local; no persiste datos de Flutter.
func (r *Runtime) apply(c Config) error {
	if c.DeviceID != r.deviceID || c.Revision < 1 || c.Zones == nil {
		return errors.New("configuración inválida o equipo incorrecto")
	}
	seen := map[string]bool{}
	for _, z := range c.Zones {
		if err := validateSchedule(z.Schedule); err != nil {
			return err
		}
		if z.RepeatMode != "" && z.RepeatMode != "off" && z.RepeatMode != "track" && z.RepeatMode != "playlist" {
			return errors.New("repeat_mode inválido")
		}
		if z.ID == "" || seen[z.ID] || z.Volume < 0 || z.Volume > 100 || (z.ChannelMode != "mono" && z.ChannelMode != "stereo") {
			return errors.New("zona inválida")
		}
		if r.audio == nil && z.Output != nil {
			return errors.New("esta etapa no admite salidas de audio; configure output=null")
		}
		if z.Playlist != nil && (z.Playlist.ID == "" || z.Playlist.Songs == nil) {
			return errors.New("playlist inválida")
		}
		seen[z.ID] = true
	}
	if r.audio != nil {
		if r.client != nil && r.client.profile != "" && c.Profile != r.client.profile {
			return errors.New("perfil diferente al negociado")
		}
		if err := validateAudioForOutput(c, r.sharedStereo); err != nil {
			return err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.Revision < r.config.Revision {
		return errors.New("revisión regresiva")
	}
	if r.audio != nil {
		if c.Revision == r.config.Revision && !sameSnapshot(c, r.config) {
			return errors.New("contenido diferente para la misma revision")
		}
		if err := r.applyAudio(c); err != nil {
			return err
		}
	}
	if c.Revision != r.config.Revision {
		r.localVolumes = nil
	}
	r.config = c
	r.configError = nil
	return nil
}
