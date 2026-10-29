package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDesktopPreferencesAndNextAreOnlyForwardedToBackend(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		var body map[string]any
		_ = json.NewDecoder(req.Body).Decode(&body)
		if req.Header.Get("Authorization") != "Bearer token" || body["config_revision"] != float64(1) {
			t.Error("missing server authorization or revision")
		}
		if calls == 1 {
			if req.URL.Path != "/api/v1/engine/zones/10/song-preference" || body["preference"] != "like" || body["song_id"] != "s" {
				t.Error("incorrect preference relay")
			}
		} else if req.URL.Path != "/api/v1/engine/zones/10/transport" || body["action"] != "next" {
			t.Error("selection was not delegated to server")
		}
		config := testConfig(1, 50)
		config.ServerControl = true
		config.SongPreferences = map[string]map[string]string{"10": {"s": "like"}}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": config})
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "token")
	config := testConfig(1, 50)
	config.ServerControl = true
	r := &Runtime{deviceID: "1", client: client, config: config}
	song, preference := "s", "like"
	request := DesktopRequest{RequestID: "preference", ZoneID: "10", Revision: 1, Action: "song_preference", SongID: &song, Preference: &preference}
	r.executeDesktop(request)
	r.executeDesktop(request)
	if calls != 1 || r.desktopResults["preference"].Outcome != "succeeded" {
		t.Fatal("preference relay was not deduplicated")
	}
	if r.desktopSnapshot().Zones[0].SongPreferences["s"] != "like" {
		t.Fatal("server preference missing from desktop snapshot")
	}
	r.executeDesktop(DesktopRequest{RequestID: "next", ZoneID: "10", Revision: 1, Action: "next"})
	if calls != 2 || r.desktopResults["next"].Outcome != "succeeded" {
		t.Fatal("next was handled locally")
	}
	if r.selected != nil {
		t.Fatal("relay made a local song selection")
	}
}

func TestBackendPreferenceErrorIsVisibleToDesktop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"code":"validation_failed","message":"Datos invalidos","details":[{"message":"Cancion excluida en esta zona"}]}}`))
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "token")
	r := &Runtime{deviceID: "1", client: client, config: testConfig(1, 50)}
	result := r.forwardDesktop(DesktopRequest{ZoneID: "10", Revision: 1, Action: "play"})
	if result.Outcome != "failed" || !strings.Contains(result.Error.Message, "Cancion excluida") {
		t.Fatal("server error was hidden")
	}
	if result := r.forwardDesktop(DesktopRequest{ZoneID: "foreign", Revision: 1, Action: "next"}); result.Error.Code != "zone_not_assigned" {
		t.Fatal("foreign zone accepted")
	}
}
