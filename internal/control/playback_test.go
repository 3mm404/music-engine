package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"music-engine/internal/decoder"
	"music-engine/internal/engine"
)

func audioConfig(raw string, data []byte) Config {
	sum := sha256.Sum256(data)
	c := testConfig(1, 25)
	c.Profile = PlaybackProfile
	c.Zones[0].Output = &Output{DeviceID: "default", Channels: []int{1, 2}}
	c.Zones[0].Playlist.Songs = []Song{{ID: "song", URL: raw, ContentVersion: hex.EncodeToString(sum[:]), Format: AudioFormat{Container: "mp3", Codec: "mp3", Mime: "audio/mpeg", Rate: 44100, Channels: 2}}}
	return c
}

func TestValidateAudioAcceptsDistinctStereoAndMonoRoutes(t *testing.T) {
	c := audioConfig("https://audio.example.test/song.mp3", nil)
	c.Zones[0].Playlist = nil
	second := c.Zones[0]
	second.ID, second.Name = "lobby", "Lobby"
	second.Output = &Output{DeviceID: "default", Channels: []int{3, 4}}
	third := c.Zones[0]
	third.ID, third.Name, third.ChannelMode = "restaurant", "Restaurant", "mono"
	third.Output = &Output{DeviceID: "default", Channels: []int{5}}
	c.Zones = append(c.Zones, second, third)
	if err := validateAudio(c); err != nil {
		t.Fatalf("valid mixed routes rejected: %v", err)
	}

	c.Zones[2].Output.Channels = []int{4}
	if err := validateAudio(c); err == nil {
		t.Fatal("overlapping output channel accepted")
	}
	c.Zones[2].Output.Channels = []int{5, 6}
	if err := validateAudio(c); err == nil {
		t.Fatal("stereo-sized route accepted for mono zone")
	}
}

func TestApplyAudioPropagatesZoneChannelAssignments(t *testing.T) {
	manager, err := engine.NewRemoteWithOutput(nil, decoder.HTTPS{})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	r := &Runtime{audio: manager}
	c := Config{Zones: []Zone{
		{ID: "pool", ChannelMode: "stereo", Output: &Output{DeviceID: "default", Channels: []int{1, 2}}},
		{ID: "lobby", ChannelMode: "stereo", Output: &Output{DeviceID: "default", Channels: []int{2, 3}}},
	}}
	if err := r.applyAudio(c); err == nil {
		t.Fatal("Manager accepted overlapping routes forwarded by applyAudio")
	}
}

func TestSpecificSongSelectionAndInvalidSelectionPreservesPending(t *testing.T) {
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		<-req.Context().Done()
	}))
	defer s.Close()
	r := audioRuntime(t, s)
	c := audioConfig(s.URL+"/first", nil)
	second := c.Zones[0].Playlist.Songs[0]
	second.ID, second.URL = "second", s.URL+"/second"
	c.Zones[0].Playlist.Songs = append(c.Zones[0].Playlist.Songs, second)
	if err := r.apply(c); err != nil {
		t.Fatal(err)
	}
	cmd := Command{ID: "specific", Sequence: 1, ZoneID: "10", Revision: 1, Action: "play", SongID: &second.ID, ExpiresAt: time.Now().Add(time.Minute)}
	if result := r.execute(cmd); result.Outcome != "pending" {
		t.Fatalf("specific play: %+v", result)
	}
	if got := r.report().Zones[0]; got.SongID == nil || *got.SongID != "second" || got.State != "loading" {
		t.Fatalf("wrong selection: %+v", got)
	}
	foreign := "foreign"
	bad := cmd
	bad.ID, bad.SongID = "invalid", &foreign
	if err := r.supersede(bad); err != nil {
		t.Fatal(err)
	}
	if result := r.execute(bad); result.Error == nil || result.Error.Code != "song_not_assigned" {
		t.Fatalf("invalid selection accepted: %+v", result)
	}
	if r.pending[cmd.ID] == nil || r.selected["10"] != 1 {
		t.Fatal("invalid command replaced valid pending selection")
	}
	bad.Action = "pause"
	if result := r.execute(bad); result.Error == nil || result.Error.Code != "invalid_song" {
		t.Fatalf("song_id accepted for pause: %+v", result)
	}
	cmd.ID, cmd.Action, cmd.SongID = "next", "next", nil
	r.execute(cmd)
	if r.selected["10"] != 0 {
		t.Fatal("next did not advance from specifically selected song")
	}
	r.audio.Close()
}

func audioRuntime(t *testing.T, s *httptest.Server) *Runtime {
	t.Helper()
	c, _ := NewClient(s.URL, "secret")
	c.http = s.Client()
	c.profile = PlaybackProfile
	m, err := engine.NewRemote(decoder.HTTPS{Client: s.Client(), Backoff: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { m.Close() })
	j, err := OpenJournal(t.TempDir(), s.URL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { j.Close() })
	return &Runtime{client: c, journal: j, deviceID: "1", audio: m}
}

func awaitAudio(t *testing.T, r *Runtime, status string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if r.report().Zones[0].State == status {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %s: %+v", status, r.report())
}

func TestPlayback403RenewsOnlyOnceAndPersistsFailure(t *testing.T) {
	var downloads, configs atomic.Int32
	var s *httptest.Server
	cmd := Command{ID: "play", Sequence: 1, ZoneID: "10", Revision: 1, Action: "play", ExpiresAt: time.Now().Add(time.Minute)}
	s = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v1/engine/config":
			configs.Add(1)
			json.NewEncoder(w).Encode(map[string]any{"data": audioConfig(s.URL+"/audio?signature=secret", nil)})
		case "/api/v1/engine/commands":
			json.NewEncoder(w).Encode(map[string]any{"data": []Command{cmd}})
		case "/audio":
			if req.Header.Get("Authorization") != "" {
				t.Error("leaked bearer")
			}
			downloads.Add(1)
			w.WriteHeader(403)
		default:
			w.Write([]byte(`{"data":{}}`))
		}
	}))
	defer s.Close()
	r := audioRuntime(t, s)
	for i := 0; i < 2; i++ {
		if err := r.cycle(context.Background()); err != nil {
			t.Fatal(err)
		}
		awaitAudio(t, r, "error")
	}
	if err := r.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	result := r.journal.results["play"]
	if result == nil || result.Error == nil || result.Error.Code != "audio_download_failed" || downloads.Load() != 2 || configs.Load() < 2 {
		t.Fatalf("downloads=%d result=%+v", downloads.Load(), result)
	}
	r.cycle(context.Background())
	if downloads.Load() != 2 {
		t.Fatal("redelivery replayed audio")
	}
}

func TestPendingDownloadCanBeStoppedByNextCommand(t *testing.T) {
	started := make(chan struct{}, 1)
	canceled := make(chan struct{}, 1)
	var addStop atomic.Bool
	var s *httptest.Server
	s = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v1/engine/config":
			json.NewEncoder(w).Encode(map[string]any{"data": audioConfig(s.URL+"/audio", nil)})
		case "/api/v1/engine/commands":
			commands := []Command{{ID: "play", Sequence: 1, ZoneID: "10", Revision: 1, Action: "play", ExpiresAt: time.Now().Add(time.Minute)}}
			if addStop.Load() {
				commands = append(commands, Command{ID: "stop", Sequence: 2, ZoneID: "10", Revision: 1, Action: "stop", ExpiresAt: time.Now().Add(time.Minute)})
			}
			json.NewEncoder(w).Encode(map[string]any{"data": commands})
		case "/audio":
			started <- struct{}{}
			<-req.Context().Done()
			canceled <- struct{}{}
		default:
			w.Write([]byte(`{"data":{}}`))
		}
	}))
	defer s.Close()
	r := audioRuntime(t, s)
	if err := r.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("download did not start")
	}
	addStop.Store(true)
	if err := r.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-canceled:
	case <-time.After(time.Second):
		t.Fatal("download not canceled")
	}
	if r.report().Zones[0].State != "stopped" || r.journal.results["play"].Error.Code != "command_superseded" || r.journal.results["stop"].Outcome != "succeeded" {
		t.Fatal("incorrect cancellation results")
	}
}

func TestAudioConfigurationIsAtomicAndRemovesZones(t *testing.T) {
	s := httptest.NewTLSServer(http.NotFoundHandler())
	defer s.Close()
	r := audioRuntime(t, s)
	c := audioConfig(s.URL, nil)
	if err := r.apply(c); err != nil {
		t.Fatal(err)
	}
	conflict := audioConfig(s.URL, nil)
	conflict.Zones[0].Volume = 80
	if r.apply(conflict) == nil || r.report().Zones[0].Volume != 25 {
		t.Fatal("same revision accepted changed content")
	}
	bad := audioConfig(s.URL, nil)
	bad.Revision = 2
	bad.Zones[0].Playlist.Songs[0].Format.Rate = 48000
	if r.apply(bad) == nil || r.report().Revision != 1 {
		t.Fatal("unsupported config partially applied")
	}
	if err := r.apply(Config{DeviceID: "1", Revision: 2, Profile: PlaybackProfile, Zones: []Zone{}}); err != nil {
		t.Fatal(err)
	}
	if len(r.audio.IDs()) != 0 || len(r.report().Zones) != 0 {
		t.Fatal("zone retained")
	}
}

func TestPlaybackRecoveryAndTransportIntegration(t *testing.T) {
	path := os.Getenv("MUSIC_ENGINE_TEST_MP3")
	if path == "" {
		t.Skip("set MUSIC_ENGINE_TEST_MP3")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var downloads, configs atomic.Int32
	var s *httptest.Server
	s = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v1/engine/config":
			url := s.URL + "/audio?signature=renewed"
			if configs.Add(1) == 1 {
				url = s.URL + "/audio?signature=expired"
			}
			json.NewEncoder(w).Encode(map[string]any{"data": audioConfig(url, data)})
		case "/api/v1/engine/commands":
			json.NewEncoder(w).Encode(map[string]any{"data": []Command{{ID: "play", Sequence: 1, ZoneID: "10", Revision: 1, Action: "play", ExpiresAt: time.Now().Add(time.Minute)}}})
		case "/audio":
			downloads.Add(1)
			if req.URL.Query().Get("signature") == "expired" {
				w.WriteHeader(403)
			} else {
				w.Write(data)
			}
		default:
			w.Write([]byte(`{"data":{}}`))
		}
	}))
	defer s.Close()
	r := audioRuntime(t, s)
	if err = r.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	awaitAudio(t, r, "error")
	if err = r.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	awaitAudio(t, r, "playing")
	if err = r.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if r.journal.results["play"].Outcome != "succeeded" || downloads.Load() != 2 {
		t.Fatal("renewal failed")
	}
	c := audioConfig(s.URL+"/audio?signature=new", data)
	c.Revision = 2
	c.Zones[0].Volume = 60
	if err = r.apply(c); err != nil {
		t.Fatal(err)
	}
	if r.report().Zones[0].State != "playing" || downloads.Load() != 2 {
		t.Fatal("volume/URL restarted track")
	}
	for _, action := range []string{"pause", "resume", "stop"} {
		result := r.execute(Command{ZoneID: "10", Revision: 2, Action: action, ExpiresAt: time.Now().Add(time.Minute)})
		if result.Outcome != "succeeded" {
			t.Fatal(result)
		}
	}
	awaitAudio(t, r, "stopped")
}
