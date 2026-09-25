package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func testConfig(revision int64, volume int) Config {
	return Config{DeviceID: "1", Revision: revision, Zones: []Zone{{ID: "10", Name: "Lobby", Volume: volume, ChannelMode: "stereo", Playlist: &Playlist{ID: "20", Name: "Jazz", Songs: []Song{}}}}}
}

func TestAtomicConfigurationAndReports(t *testing.T) {
	r := &Runtime{deviceID: "1"}
	if err := r.apply(testConfig(1, 65)); err != nil {
		t.Fatal(err)
	}
	if err := r.apply(testConfig(2, 101)); err == nil {
		t.Fatal("accepted invalid volume")
	}
	report := r.report()
	if report.Revision != 1 || len(report.Zones) != 1 || report.Zones[0].Volume != 65 || report.Zones[0].State != "stopped" {
		t.Fatalf("partial config applied: %+v", report)
	}
	if err := r.apply(Config{DeviceID: "1", Revision: 2, Zones: []Zone{}}); err != nil {
		t.Fatal(err)
	}
	if len(r.report().Zones) != 0 {
		t.Fatal("removed zone retained")
	}
}

func TestJournalRecoversCompletedAndUnknownWithoutReplay(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	j, err := OpenJournal(dir, "1")
	if err != nil {
		t.Fatal(err)
	}
	done := Result{Outcome: "succeeded", CompletedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	if err = j.Record("done", &done); err != nil {
		t.Fatal(err)
	}
	if err = j.Record("interrupted", nil); err != nil {
		t.Fatal(err)
	}
	j.Close()
	j, err = OpenJournal(dir, "1")
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	results := map[string]Result{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v1/engine/config":
			json.NewEncoder(w).Encode(map[string]any{"data": testConfig(1, 65)})
		case "/api/v1/engine/commands":
			json.NewEncoder(w).Encode(map[string]any{"data": []Command{
				{ID: "done", Sequence: 1, ZoneID: "10", Revision: 1, Action: "next", ExpiresAt: time.Now().Add(time.Hour)},
				{ID: "interrupted", Sequence: 2, ZoneID: "10", Revision: 1, Action: "stop", ExpiresAt: time.Now().Add(time.Hour)},
			}})
		default:
			id := strings.Split(req.URL.Path, "/")[5]
			var result Result
			json.NewDecoder(req.Body).Decode(&result)
			results[id] = result
			w.Write([]byte(`{"data":{}}`))
		}
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, "secret")
	r := &Runtime{client: c, journal: j, deviceID: "1"}
	if err = r.cycle(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(results["done"], done) {
		t.Fatal("finished command was executed again")
	}
	if results["interrupted"].Error.Code != "execution_unknown" {
		t.Fatal("interrupted command was repeated")
	}
}

func TestLostAcknowledgmentReusesDurableResult(t *testing.T) {
	j, err := OpenJournal(t.TempDir(), "1")
	if err != nil {
		t.Fatal(err)
	}
	defer j.Close()
	var results []Result
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/api/v1/engine/config":
			json.NewEncoder(w).Encode(map[string]any{"data": testConfig(1, 10)})
		case "/api/v1/engine/commands":
			json.NewEncoder(w).Encode(map[string]any{"data": []Command{{ID: "cmd", Sequence: 1, ZoneID: "10", Revision: 1, Action: "stop", ExpiresAt: time.Now().Add(time.Minute)}}})
		default:
			var result Result
			json.NewDecoder(req.Body).Decode(&result)
			results = append(results, result)
			if len(results) == 1 {
				w.WriteHeader(503)
			} else {
				w.Write([]byte(`{"data":{}}`))
			}
		}
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, "secret")
	r := &Runtime{client: c, journal: j, deviceID: "1"}
	if r.cycle(context.Background()) == nil {
		t.Fatal("expected lost ACK")
	}
	if err := r.cycle(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || !reflect.DeepEqual(results[0], results[1]) {
		t.Fatal("result changed on redelivery")
	}
}

func TestJournalCorruptionAndExclusiveAccess(t *testing.T) {
	dir := t.TempDir()
	j, err := OpenJournal(dir, "1")
	if err != nil {
		t.Fatal(err)
	}
	other, err := OpenJournal(dir, "1")
	if err == nil {
		other.Close()
		t.Fatal("second process acquired journal")
	}
	name := j.file.Name()
	j.Close()
	if err := os.WriteFile(name, []byte(`{"command_id":"unfinished"`), 0600); err != nil {
		t.Fatal(err)
	}
	if j, err := OpenJournal(dir, "1"); err == nil {
		j.Close()
		t.Fatal("accepted corrupt journal")
	}
}

func TestCommandFailures(t *testing.T) {
	r := &Runtime{deviceID: "1"}
	_ = r.apply(testConfig(1, 65))
	for _, test := range []struct {
		name    string
		command Command
		code    string
	}{
		{"expired", Command{ZoneID: "10", Revision: 1, ExpiresAt: time.Now().Add(-time.Second)}, "command_expired"},
		{"removed", Command{ZoneID: "99", Revision: 1, ExpiresAt: time.Now().Add(time.Hour)}, "zone_not_assigned"},
		{"revision", Command{ZoneID: "10", Revision: 2, ExpiresAt: time.Now().Add(time.Hour)}, "config_revision_mismatch"},
		{"audio", Command{ZoneID: "10", Revision: 1, Action: "play", ExpiresAt: time.Now().Add(time.Hour)}, "unsupported_action"},
	} {
		t.Run(test.name, func(t *testing.T) {
			result := r.execute(test.command)
			if result.Error == nil || result.Error.Code != test.code {
				t.Fatalf("%+v", result)
			}
		})
	}
}

func TestClientRejectsUnsafeURLsAndRedirects(t *testing.T) {
	for _, url := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com?token=x"} {
		if _, err := NewClient(url, "token"); err == nil {
			t.Fatalf("accepted %s", url)
		}
	}
	var leaked atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Store(true) }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer server.Close()
	c, _ := NewClient(server.URL, "secret")
	_, err := c.Configuration(context.Background())
	if err == nil || leaked.Load() {
		t.Fatal("followed control redirect")
	}
}

func TestWebSocketPrivateAuthPingAndReconnect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	wake := make(chan struct{}, 10)
	pongs := make(chan struct{}, 10)
	fatal := make(chan error, 1)
	var connections atomic.Int32
	var authCalls atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/engine/broadcasting/auth" {
			if r.Header.Get("Authorization") != "Bearer secret" || r.Header.Get("X-Engine-Session") != "session" {
				t.Error("missing authentication")
			}
			authCalls.Add(1)
			w.Write([]byte(`{"auth":"key:signed"}`))
			return
		}
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		connections.Add(1)
		conn.WriteJSON(map[string]any{"event": "pusher:connection_established", "data": `{"socket_id":"123.456","activity_timeout":30}`})
		var sub struct {
			Event string
			Data  map[string]string
		}
		if conn.ReadJSON(&sub) != nil {
			return
		}
		if sub.Event != "pusher:subscribe" || sub.Data["auth"] != "key:signed" || sub.Data["channel"] != "private-engines.1" {
			t.Error("invalid private subscription")
		}
		conn.WriteJSON(map[string]any{"event": "pusher_internal:subscription_succeeded", "channel": "private-engines.1", "data": "{}"})
		conn.WriteJSON(map[string]any{"event": "pusher:ping", "data": "{}"})
		conn.SetReadDeadline(time.Now().Add(time.Second))
		var pong frame
		if conn.ReadJSON(&pong) != nil || pong.Event != "pusher:pong" {
			t.Error("missing pong")
			return
		}
		pongs <- struct{}{}
	}))
	defer server.Close()
	c, _ := NewClient(server.URL, "secret")
	c.session = "session"
	done := make(chan struct{})
	go func() {
		defer close(done)
		c.watch(ctx, WebSocketConfig{URL: strings.Replace(server.URL, "http:", "ws:", 1), Key: "key", Channel: "private-engines.1"}, wake, fatal)
	}()
	for count := 0; count < 2; count++ {
		select {
		case <-wake:
		case err := <-fatal:
			t.Fatal(err)
		case <-ctx.Done():
			t.Fatal("did not reconnect")
		}
		select {
		case <-pongs:
		case <-ctx.Done():
			t.Fatal("did not complete ping/pong before reconnect cancellation")
		}
	}
	cancel()
	<-done
	if connections.Load() < 2 || authCalls.Load() < 2 {
		t.Fatal("reconnection did not reauthorize")
	}
}

func TestRuntimeRestoresChangesAfterWebSocketReconnect(t *testing.T) {
	// Full run verifies config-before-commands and an event-triggered update,
	// with a poll interval too long to hide a broken reconnect implementation.
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	var revision atomic.Int64
	revision.Store(1)
	var connections atomic.Int32
	observed := make(chan Report, 20)
	upgrader := websocket.Upgrader{}
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/engine/sessions":
			json.NewEncoder(w).Encode(map[string]any{"data": Session{DeviceID: "1", SessionID: "session", HeartbeatSeconds: 1, PollSeconds: 60, WebSocket: &WebSocketConfig{URL: strings.Replace(server.URL, "http:", "ws:", 1), Key: "key", Channel: "private-engines.1"}}})
		case "/api/v1/engine/broadcasting/auth":
			w.Write([]byte(`{"auth":"key:signed"}`))
		case "/api/v1/engine/config":
			rev := revision.Load()
			json.NewEncoder(w).Encode(map[string]any{"data": testConfig(rev, int(rev)*30)})
		case "/api/v1/engine/commands":
			w.Write([]byte(`{"data":[]}`))
		case "/api/v1/engine/heartbeat":
			var report Report
			json.NewDecoder(r.Body).Decode(&report)
			observed <- report
			w.Write([]byte(`{"data":{}}`))
		default:
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				return
			}
			defer conn.Close()
			connections.Add(1)
			conn.WriteJSON(map[string]any{"event": "pusher:connection_established", "data": `{"socket_id":"1.2","activity_timeout":30}`})
			var subscription frame
			if conn.ReadJSON(&subscription) != nil {
				return
			}
			if connections.Load() > 1 {
				revision.Store(2)
			}
			conn.WriteJSON(map[string]any{"event": "pusher_internal:subscription_succeeded", "channel": "private-engines.1", "data": "{}"})
		}
	}))
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- Run(ctx, server.URL, "token", "test", filepath.Join(t.TempDir(), "state")) }()
	for {
		select {
		case report := <-observed:
			if report.Revision == 2 && len(report.Zones) == 1 && report.Zones[0].Volume == 60 {
				cancel()
				if err := <-done; err != nil {
					t.Fatal(err)
				}
				return
			}
		case err := <-done:
			t.Fatalf("ended before pending changes applied: %v", err)
		case <-ctx.Done():
			t.Fatal("missed reconnect update")
		}
	}
}
