package control

import (
	"bytes"
	"context"
	"music-engine/internal/decoder"
	"music-engine/internal/engine"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDesktopPlaybackAndServerCommandsShareRuntime(t *testing.T) {
	frame := make([]byte, 417)
	copy(frame, []byte{0xff, 0xfb, 0x90, 0x00})
	data := bytes.Repeat(frame, 20)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) { w.Write(data) }))
	defer server.Close()
	manager, err := engine.NewRemoteWithOutput(&simulatedOutput{shared: true}, decoder.HTTPS{Client: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	journal, err := OpenJournal(t.TempDir(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer journal.Close()
	r := &Runtime{audio: manager, journal: journal, deviceID: "1", sharedStereo: true}
	events := []DesktopEvent{}
	r.observer = func(event DesktopEvent) { events = append(events, event) }
	config := audioConfig(server.URL+"/audio", data)
	second := config.Zones[0].Playlist.Songs[0]
	second.ID = "second"
	config.Zones[0].Playlist.Songs = append(config.Zones[0].Playlist.Songs, second)
	if err := r.apply(config); err != nil {
		t.Fatal(err)
	}
	request := func(id, action string, song *string) {
		t.Helper()
		r.executeDesktop(DesktopRequest{RequestID: id, ZoneID: "10", Revision: 1, Action: action, SongID: song})
	}
	complete := func(id, song string) {
		t.Helper()
		awaitAudio(t, r, "playing")
		if err := r.finishPending(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		if r.desktopResults[id].Outcome != "succeeded" {
			t.Fatalf("not confirmed: %+v", r.desktopResults[id])
		}
		if got := r.report().Zones[0].SongID; got == nil || *got != song {
			t.Fatalf("wrong song: %v", got)
		}
	}
	request("play", "play", &second.ID)
	complete("play", "second")
	request("pause", "pause", nil)
	if r.desktopResults["pause"].Outcome != "succeeded" || r.report().Zones[0].State != "paused" {
		t.Fatal("pause failed")
	}
	request("resume", "resume", nil)
	if r.report().Zones[0].State != "playing" {
		t.Fatal("resume failed")
	}
	request("next", "next", nil)
	complete("next", "song")
	request("next", "next", nil)
	if *r.report().Zones[0].SongID != "song" {
		t.Fatal("duplicate request executed twice")
	}
	request("previous", "previous", nil)
	complete("previous", "second")
	foreign := "foreign"
	request("foreign", "play", &foreign)
	if r.desktopResults["foreign"].Error.Code != "song_not_assigned" {
		t.Fatal("foreign song accepted")
	}
	r.executeDesktop(DesktopRequest{RequestID: "stale", ZoneID: "10", Revision: 2, Action: "next"})
	if r.desktopResults["stale"].Error.Code != "config_revision_mismatch" {
		t.Fatal("stale revision accepted")
	}
	r.executeDesktop(DesktopRequest{RequestID: "zone", ZoneID: "missing", Revision: 1, Action: "pause"})
	if r.desktopResults["zone"].Error.Code != "zone_not_assigned" {
		t.Fatal("foreign zone accepted")
	}
	if len(journal.results) != 0 {
		t.Fatal("desktop commands leaked into server journal")
	}
	// Server commands can supersede a local load and retain their own journal path.
	request("loading", "play", nil)
	if err := r.supersede(Command{ID: "server-stop", ZoneID: "10", Revision: 1, Action: "stop", ExpiresAt: time.Now().Add(time.Minute)}); err != nil {
		t.Fatal(err)
	}
	if r.desktopResults["loading"].Error.Code != "command_superseded" {
		t.Fatal("local supersession not reported")
	}
	if len(events) == 0 {
		t.Fatal("no command result events")
	}
}
