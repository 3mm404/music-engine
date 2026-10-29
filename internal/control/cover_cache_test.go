package control

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestCoverCachePersistsWithoutCredentialsAndWorksOffline(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "" {
			t.Error("device credentials sent to artwork endpoint")
		}
		_, _ = w.Write([]byte("\x89PNG\r\n\x1a\nimage"))
	}))
	dir := t.TempDir()
	path, err := cacheCover(context.Background(), server.Client(), dir, server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	server.Close()
	again, err := cacheCover(context.Background(), server.Client(), dir, server.URL)
	if err != nil || again != path || calls != 1 {
		t.Fatalf("persistent cover not reused: %q %v calls=%d", again, err, calls)
	}
}

func TestCoverCacheRejectsUnsafeDownloads(t *testing.T) {
	for _, test := range []struct {
		name, body string
		redirect   bool
	}{
		{name: "html", body: "<html>not an image</html>"},
		{name: "oversize", body: "\x89PNG\r\n\x1a\n" + strings.Repeat("x", maxCoverBytes)},
		{name: "redirect", redirect: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if test.redirect {
					http.Redirect(w, r, "/other", http.StatusFound)
					return
				}
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			dir := t.TempDir()
			if _, err := cacheCover(context.Background(), server.Client(), dir, server.URL); err == nil {
				t.Fatal("invalid download accepted")
			}
			files, _ := os.ReadDir(dir)
			if len(files) != 0 {
				t.Fatal("invalid cover left on disk")
			}
		})
	}
}

func TestDesktopCoverMetadataDoesNotRestartAudioOrExposeURLs(t *testing.T) {
	first := &Playlist{ID: "p", CoverURL: "https://example.test/first.png", Songs: []Song{{ID: "s"}}}
	second := &Playlist{ID: "p", CoverURL: "https://example.test/second.png", Songs: []Song{{ID: "s"}}}
	if !samePlaylist(first, second) {
		t.Fatal("artwork change restarts audio")
	}
	if sameSnapshot(Config{Zones: []Zone{{Playlist: first}}}, Config{Zones: []Zone{{Playlist: second}}}) {
		t.Fatal("conflicting cover in same revision accepted")
	}
	r := &Runtime{config: Config{Zones: []Zone{{ID: "z", Playlist: first}}}, coverPaths: map[string]string{first.CoverURL: "local-cover.img"}}
	event := r.desktopSnapshot()
	if event.Zones[0].CoverPath != "local-cover.img" || event.Zones[0].SongMetadata[0].CoverPath != "local-cover.img" {
		t.Fatal("playlist artwork not inherited")
	}
}

func TestZoneBackgroundStaysOutsideAudioRevisionAndUsesLocalArtwork(t *testing.T) {
	first := Config{DeviceID: "e", Revision: 1, Zones: []Zone{{ID: "z", Name: "Cafe", Volume: 60, ChannelMode: "stereo"}}, ZoneBackgrounds: map[string]string{"z": "https://images.example.test/old.jpg"}}
	next := first
	next.ZoneBackgrounds = map[string]string{"z": "https://images.example.test/new.jpg"}
	if !sameSnapshot(first, next) {
		t.Fatal("background changed the audio snapshot")
	}
	r := &Runtime{deviceID: "e", config: first, localVolumes: map[string]int{"z": 37}, coverPaths: map[string]string{"https://images.example.test/new.jpg": "local-background.img"}}
	if err := r.apply(next); err != nil {
		t.Fatal(err)
	}
	if r.localVolumes["z"] != 37 {
		t.Fatal("background reset the local volume")
	}
	event := r.desktopSnapshot()
	if event.Zones[0].BackgroundPath != "local-background.img" || event.Zones[0].CoverPath != "" {
		t.Fatal("zone background mixed with song artwork")
	}
}
