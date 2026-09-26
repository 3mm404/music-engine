package player

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func awaitStatus(t *testing.T, p *Player, status string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if p.GetState().Status == status {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("expected %s, got %+v", status, p.GetState())
}

func TestRemoteCancellationAndFailure(t *testing.T) {
	started := make(chan struct{}, 4)
	canceled := make(chan struct{}, 4)
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			w.WriteHeader(403)
			return
		}
		started <- struct{}{}
		<-r.Context().Done()
		canceled <- struct{}{}
	}))
	defer s.Close()
	p := New()
	p.remote.Client = s.Client()
	defer p.Stop()
	for _, action := range []string{"switch", "stop"} {
		if err := p.Play(s.URL + "/slow?signature=secret"); err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("not started")
		}
		if state := p.GetState(); state.Status != "LOADING" || strings.Contains(state.Track, "secret") {
			t.Fatal(state)
		}
		if action == "switch" {
			p.Play(s.URL + "/fail")
			awaitStatus(t, p, "ERROR")
		} else {
			p.Stop()
			awaitStatus(t, p, "STOPPED")
		}
		select {
		case <-canceled:
		case <-time.After(time.Second):
			t.Fatal("not canceled")
		}
	}
}

func TestHTTPSPlaybackIntegration(t *testing.T) {
	path := os.Getenv("MUSIC_ENGINE_TEST_MP3")
	if path == "" {
		t.Skip("define MUSIC_ENGINE_TEST_MP3 para audio real")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count++
		if count == 1 {
			w.WriteHeader(503)
			return
		}
		if r.URL.Query().Get("signature") != "secret" {
			w.WriteHeader(403)
			return
		}
		w.Write(data)
	}))
	defer s.Close()
	p := New()
	p.remote.Client = s.Client()
	p.remote.Backoff = 200 * time.Millisecond
	defer p.Stop()
	p.Play(s.URL + "/audio?signature=secret")
	awaitStatus(t, p, "RECOVERING")
	awaitStatus(t, p, "PLAYING")
	if err := p.Pause(); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, p, "PAUSED")
	if err := p.Play(""); err != nil {
		t.Fatal(err)
	}
	awaitStatus(t, p, "PLAYING")
}
