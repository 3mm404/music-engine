package control

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"
	"time"

	"music-engine/internal/decoder"
	"music-engine/internal/engine"
)

const PlaybackProfile = "shared_stereo_mp3"
const MonoProfile = "shared_mono_mp3"

type pendingPlayback struct {
	command Command
	song    Song
	renewed bool
	failure *Failure
}

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

// Called only after the entire snapshot has passed validation, with r.mu held.
func (r *Runtime) applyAudio(c Config) error {
	configs := make([]engine.ZoneConfig, 0, len(c.Zones))
	for _, z := range c.Zones {
		config := engine.ZoneConfig{ID: z.ID}
		if !r.sharedStereo {
			config.OutputChannels = append([]int(nil), z.Output.Channels...)
		}
		configs = append(configs, config)
	}
	if err := r.audio.Reconfigure(configs); err != nil {
		return err
	}
	if r.selected == nil {
		r.selected = map[string]int{}
	}
	for _, z := range c.Zones {
		var old *Zone
		for i := range r.config.Zones {
			if r.config.Zones[i].ID == z.ID {
				old = &r.config.Zones[i]
				break
			}
		}
		p, _ := r.audio.Zone(z.ID)
		if old == nil || !samePlaylist(old.Playlist, z.Playlist) || !reflect.DeepEqual(old.Output, z.Output) || old.ChannelMode != z.ChannelMode {
			if !r.sharedStereo && z.Output != nil {
				log.Printf("Audio zone routing applied: zone=%q mode=%s device=%q output.channels=%v", z.ID, z.ChannelMode, z.Output.DeviceID, z.Output.Channels)
			} else {
				log.Printf("Zona %q: mezcla compartida Oto", z.ID)
			}
			p.Stop()
			r.selected[z.ID] = 0
			for _, pending := range r.pending {
				if pending.command.ZoneID == z.ID {
					pending.failure = &Failure{"config_revision_mismatch", "La playlist cambio durante la carga"}
				}
			}
		}
		p.SetVolume(float64(z.Volume) / 100)
	}
	for id := range r.selected {
		if _, err := r.audio.Zone(id); err != nil {
			delete(r.selected, id)
			for _, pending := range r.pending {
				if pending.command.ZoneID == id {
					pending.failure = &Failure{"zone_not_assigned", "Zona retirada durante la carga"}
				}
			}
		}
	}
	return nil
}

func validateSongSelection(cmd Command, z *Zone) *Failure {
	if cmd.SongID == nil {
		return nil
	}
	if cmd.Action != "play" {
		return &Failure{"invalid_song", "Solo play admite song_id"}
	}
	if z.Playlist != nil {
		for _, song := range z.Playlist.Songs {
			if song.ID == *cmd.SongID {
				return nil
			}
		}
	}
	return &Failure{"song_not_assigned", "Cancion fuera de la playlist de la zona"}
}

func (r *Runtime) executeAudio(cmd Command, z *Zone) Result {
	p, _ := r.audio.Zone(z.ID)
	var err error
	switch cmd.Action {
	case "play", "next", "previous":
		if z.Playlist == nil || len(z.Playlist.Songs) == 0 {
			return failed("empty_playlist", "La playlist esta vacia")
		}
		i := r.selected[z.ID]
		if cmd.SongID != nil {
			for index, song := range z.Playlist.Songs {
				if song.ID == *cmd.SongID {
					i = index
					break
				}
			}
		}
		if cmd.Action == "next" {
			i = (i + 1) % len(z.Playlist.Songs)
		}
		if cmd.Action == "previous" {
			i = (i + len(z.Playlist.Songs) - 1) % len(z.Playlist.Songs)
		}
		r.selected[z.ID] = i
		song := z.Playlist.Songs[i]
		if err = p.PlayVerified(song.URL, song.ContentVersion); err == nil {
			if r.pending == nil {
				r.pending = map[string]*pendingPlayback{}
			}
			r.pending[cmd.ID] = &pendingPlayback{command: cmd, song: song}
			return Result{Outcome: "pending"}
		}
	case "pause":
		err = p.Pause()
	case "resume":
		err = p.Resume()
	case "stop":
		err = p.Stop()
	default:
		return failed("unsupported_action", "Accion desconocida")
	}
	if err != nil {
		return failed("no_track", err.Error())
	}
	return Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
}

func (r *Runtime) supersede(cmd Command) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cmd.Revision != r.config.Revision || !cmd.ExpiresAt.After(time.Now()) {
		return nil
	}
	if cmd.Action != "play" && cmd.Action != "next" && cmd.Action != "previous" && cmd.Action != "stop" {
		return nil
	}
	for i := range r.config.Zones {
		if r.config.Zones[i].ID == cmd.ZoneID && validateSongSelection(cmd, &r.config.Zones[i]) != nil {
			return nil
		}
	}
	for id, pending := range r.pending {
		if pending.command.ZoneID != cmd.ZoneID {
			continue
		}
		result := failed("command_superseded", "La carga fue reemplazada por otra orden")
		if err := r.journal.Record(id, &result); err != nil {
			return err
		}
		delete(r.pending, id)
	}
	return nil
}

// Configuration has just been fetched by cycle. A 403 gets at most one replay,
// using that fresh URL only if the same song/content is still assigned.
func (r *Runtime) finishPending(ctx context.Context, fresh bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, pending := range r.pending {
		p, err := r.audio.Zone(pending.command.ZoneID)
		failure := pending.failure
		if err != nil {
			failure = &Failure{"zone_not_assigned", "Zona retirada"}
		}
		var result Result
		if failure != nil {
			result = failed(failure.Code, failure.Message)
		} else {
			state := p.GetState()
			if state.Status == "LOADING" || state.Status == "RECOVERING" {
				continue
			}
			var httpErr *decoder.HTTPError
			if fresh && !pending.renewed && errors.As(state.Error, &httpErr) && httpErr.Status == 403 {
				pending.renewed = true
				for _, z := range r.config.Zones {
					if z.ID != pending.command.ZoneID || z.Playlist == nil {
						continue
					}
					for _, song := range z.Playlist.Songs {
						if song.ID == pending.song.ID && song.ContentVersion == pending.song.ContentVersion && song.Format == pending.song.Format {
							if ctx.Err() == nil {
								err = p.PlayVerified(song.URL, song.ContentVersion)
								if err == nil {
									break
								}
							}
						}
					}
				}
				if p.GetState().Status == "LOADING" {
					continue
				}
			}
			if state.Error != nil {
				result = failed("audio_download_failed", state.Error.Error())
			} else {
				result = Result{Outcome: "succeeded", CompletedAt: time.Now().UTC()}
			}
		}
		if err := r.journal.Record(id, &result); err != nil {
			return err
		}
		delete(r.pending, id)
	}
	return nil
}

func (r *Runtime) audioReport(z Zone, state *ZoneState) {
	p, _ := r.audio.Zone(z.ID)
	s := p.GetState()
	state.State = strings.ToLower(s.Status)
	state.Output = z.Output
	if z.Playlist != nil && len(z.Playlist.Songs) > 0 {
		id := z.Playlist.Songs[r.selected[z.ID]].ID
		state.SongID = &id
	}
	if s.Error != nil {
		state.Error = &Failure{"audio_download_failed", s.Error.Error()}
	}
}
