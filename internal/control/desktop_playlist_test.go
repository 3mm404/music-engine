package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDesktopPlaylistAssignmentIsServerConfirmedAndDeduplicated(t *testing.T) {
	calls := 0
	config := testConfig(2, 50)
	config.Zones[0].Playlist = &Playlist{ID: "p", Name: "Ambient", Songs: []Song{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		if req.Method != "PUT" || req.URL.Path != "/api/v1/engine/zones/10/playlist" || req.Header.Get("Authorization") != "Bearer token" {
			t.Error("incorrect assignment request")
		}
		var body map[string]any
		_ = json.NewDecoder(req.Body).Decode(&body)
		if body["playlist_id"] != "p" || body["config_revision"] != float64(1) {
			t.Error("incorrect playlist or revision")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": config})
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "token")
	r := &Runtime{deviceID: "1", client: client, config: testConfig(1, 50)}
	id := "p"
	req := DesktopRequest{RequestID: "playlist", ZoneID: "10", Revision: 1, Action: "play_playlist", PlaylistID: &id}
	r.executeDesktop(req)
	r.executeDesktop(req)
	if calls != 1 || r.config.Revision != 2 || r.config.Zones[0].Playlist.ID != "p" || r.desktopResults[req.RequestID].Outcome != "succeeded" {
		t.Fatal("assignment not confirmed")
	}
	req.RequestID = "stale"
	r.executeDesktop(req)
	if calls != 1 || r.desktopResults[req.RequestID].Error.Code != "config_revision_mismatch" {
		t.Fatal("stale revision accepted")
	}
}

func TestDesktopPlaylistRejectionKeepsCurrentAssignment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { w.WriteHeader(http.StatusNotFound) }))
	defer server.Close()
	client, _ := NewClient(server.URL, "token")
	config := testConfig(1, 50)
	r := &Runtime{deviceID: "1", client: client, config: config}
	id := "foreign"
	result := r.playDesktopPlaylist(DesktopRequest{ZoneID: "10", Revision: 1, PlaylistID: &id})
	if result.Outcome != "failed" || r.config.Revision != 1 {
		t.Fatal("rejected assignment changed state")
	}
	if result := r.playDesktopPlaylist(DesktopRequest{ZoneID: "missing", Revision: 1, PlaylistID: &id}); result.Error.Code != "zone_not_assigned" {
		t.Fatal("unassigned zone accepted")
	}
	if result := r.playDesktopPlaylist(DesktopRequest{ZoneID: "10", Revision: 1}); result.Error.Code != "invalid_playlist" {
		t.Fatal("missing playlist accepted")
	}
}

func TestDesktopPlaylistCarriesSelectedSong(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(req.Body).Decode(&body)
		if body["song_id"] != "second" || body["playlist_id"] != "p" {
			t.Error("chosen song was lost")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": testConfig(2, 50)})
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "token")
	r := &Runtime{deviceID: "1", client: client, config: testConfig(1, 50)}
	playlist, song := "p", "second"
	r.executeDesktop(DesktopRequest{RequestID: "chosen", ZoneID: "10", Revision: 1, Action: "play_playlist", PlaylistID: &playlist, SongID: &song})
	if r.desktopResults["chosen"].Outcome != "succeeded" {
		t.Fatal("selected song request rejected")
	}
}

func TestDesktopCatalogExposesSongsAndLocalCoversWithoutAudioGrants(t *testing.T) {
	r := &Runtime{config: Config{Playlists: []Playlist{{ID: "p", Name: "Lounge", CoverURL: "https://example.test/cover", Songs: []Song{{ID: "s", Title: "Tema", URL: "https://example.test/audio?signature=secret"}}}}}, coverPaths: map[string]string{"https://example.test/cover": "local.img"}}
	event := r.desktopSnapshot()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "https://") || strings.Contains(string(data), "secret") {
		t.Fatal("private media URLs leaked")
	}
	if len(event.Playlists) != 1 || event.Playlists[0].Songs[0].Title != "Tema" || event.Playlists[0].Songs[0].CoverPath != "local.img" {
		t.Fatal("incomplete playlist catalog")
	}
}
