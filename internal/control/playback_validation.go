package control

import (
	"encoding/hex"
	"errors"
	"fmt"
	"music-engine/internal/decoder"
	"reflect"
)

// samePlaylist compara identidad y contenido, no firmas temporales de las URLs.
func samePlaylist(a, b *Playlist) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.ID != b.ID || len(a.Songs) != len(b.Songs) {
		return false
	}
	for i, s := range a.Songs {
		t := b.Songs[i]
		if s.ID != t.ID || s.ContentVersion != t.ContentVersion || s.Format != t.Format {
			return false
		}
	}
	return true
}

func sameSnapshot(a, b Config) bool {
	if a.Profile != b.Profile || len(a.Zones) != len(b.Zones) {
		return false
	}
	for i, z := range a.Zones {
		other := b.Zones[i]
		if z.ID != other.ID || z.Name != other.Name || z.Volume != other.Volume || z.ChannelMode != other.ChannelMode || !reflect.DeepEqual(z.Output, other.Output) || !samePlaylist(z.Playlist, other.Playlist) {
			return false
		}
	}
	return true
}

func validateAudio(c Config) error { return validateAudioForOutput(c, false) }

// validateAudioForOutput rechaza formatos y rutas incompatibles antes de tocar reproductores. shared distingue mezcla Oto de canales físicos independientes.
func validateAudioForOutput(c Config, shared bool) error {
	if c.Profile != PlaybackProfile && c.Profile != MonoProfile {
		return errors.New("perfil de reproduccion no compatible")
	}
	assigned := map[int]string{}
	for _, z := range c.Zones {
		if z.ChannelMode != "mono" && z.ChannelMode != "stereo" {
			return errors.New("unsupported_output: channel_mode debe ser mono o stereo")
		}
		if c.Profile == MonoProfile && z.ChannelMode != "mono" {
			return errors.New("unsupported_output: el perfil mono solo admite zonas mono")
		}
		channelCount := 2
		if z.ChannelMode == "mono" {
			channelCount = 1
		}
		if shared {
			if z.Output != nil && (z.Output.DeviceID != "default" || (z.ChannelMode == "stereo" && !reflect.DeepEqual(z.Output.Channels, []int{1, 2})) || (z.ChannelMode == "mono" && !reflect.DeepEqual(z.Output.Channels, []int{1}))) {
				return fmt.Errorf("unsupported_output: Oto mezcla todas las zonas en la salida del sistema; zona %q no puede asignar canales fisicos independientes", z.ID)
			}
		} else {
			if z.Output == nil || z.Output.DeviceID != "default" || len(z.Output.Channels) != channelCount {
				return fmt.Errorf("unsupported_output: zona %q requiere salida default con %d canal(es)", z.ID, channelCount)
			}
			for _, channel := range z.Output.Channels {
				if channel < 1 {
					return fmt.Errorf("unsupported_output: canal invalido %d en zona %q", channel, z.ID)
				}
				if owner, exists := assigned[channel]; exists {
					return fmt.Errorf("unsupported_output: canal %d compartido por zonas %q y %q", channel, owner, z.ID)
				}
				assigned[channel] = z.ID
			}
		}
		if z.Playlist == nil {
			continue
		}
		seen := map[string]bool{}
		for _, s := range z.Playlist.Songs {
			hash, hashErr := hex.DecodeString(s.ContentVersion)
			if hashErr != nil || len(hash) != 32 {
				return errors.New("version de contenido SHA256 invalida")
			}
			if s.ID == "" || seen[s.ID] || s.ContentVersion == "" || !decoder.ValidURL(s.URL) {
				return errors.New("cancion o URL HTTPS invalida")
			}
			seen[s.ID] = true
			if s.Format.Container != "mp3" || s.Format.Codec != "mp3" || s.Format.Mime != "audio/mpeg" || s.Format.Rate != 44100 || (s.Format.Channels != 1 && s.Format.Channels != 2) {
				return errors.New("unsupported_format: requiere MP3 44100 Hz")
			}
		}
	}
	return nil
}
