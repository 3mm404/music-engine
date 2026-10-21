package control

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDesktopModesArePersistedByServerAndDeduplicated(t *testing.T) {
	calls := 0
	config := testConfig(2, 50)
	config.Zones[0].Shuffle = true
	config.Zones[0].RepeatMode = "track"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls++
		if req.Method != "PUT" || req.URL.Path != "/api/v1/engine/zones/10/playback-modes" || req.Header.Get("Authorization") != "Bearer token" {
			t.Error("invalid mode request")
		}
		var body map[string]any
		json.NewDecoder(req.Body).Decode(&body)
		if body["shuffle"] != true || body["config_revision"] != float64(1) {
			t.Error("invalid mode payload")
		}
		json.NewEncoder(w).Encode(map[string]any{"data": config})
	}))
	defer server.Close()
	client, _ := NewClient(server.URL, "token")
	r := &Runtime{deviceID: "1", client: client, config: testConfig(1, 50)}
	shuffle := true
	req := DesktopRequest{RequestID: "modes", ZoneID: "10", Revision: 1, Action: "playback_modes", Shuffle: &shuffle}
	r.executeDesktop(req)
	r.executeDesktop(req)
	if calls != 1 || r.config.Revision != 2 || !r.config.Zones[0].Shuffle || r.desktopResults["modes"].Outcome != "succeeded" {
		t.Fatal("modes not confirmed by server")
	}
	req.RequestID = "stale"
	r.executeDesktop(req)
	if calls != 1 || r.desktopResults["stale"].Error.Code != "config_revision_mismatch" {
		t.Fatal("stale mode request accepted")
	}
}
