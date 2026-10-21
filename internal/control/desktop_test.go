package control

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDesktopSnapshotIncludesStateWithoutSignedURLs(t *testing.T) {
	artist := "Artista"
	r := &Runtime{config: Config{Revision: 7, Zones: []Zone{{ID: "one", Name: "Terraza", Volume: 50, ChannelMode: "stereo",
		Shuffle: true, RepeatMode: "playlist", ContinuousPlayback: true,
		Playlist: &Playlist{ID: "p", Name: "Relax", Songs: []Song{{ID: "s", Title: "Título", Artist: &artist, URL: "https://example.test/audio?signature=secret"}}}}}}}
	event := r.desktopSnapshot()
	data, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "signature") || strings.Contains(string(data), "https://") {
		t.Fatal("private audio URL leaked")
	}
	if event.Report.Revision != 7 || len(event.Zones) != 1 || event.Zones[0].Name != "Terraza" || event.Zones[0].Playlist != "Relax" {
		t.Fatal("incomplete desktop snapshot")
	}
	if event.Report.Zones[0].State != "stopped" || event.Report.Zones[0].Volume != 50 {
		t.Fatal("invalid zone state")
	}
	if len(event.Zones[0].Songs) != 1 || event.Zones[0].Songs[0] != "s" {
		t.Fatal("assigned song IDs missing")
	}
	zone := event.Zones[0]
	if len(zone.SongMetadata) != 1 || zone.SongMetadata[0].Title != "Título" || *zone.SongMetadata[0].Artist != artist || !zone.Shuffle || zone.RepeatMode != "playlist" || !zone.ContinuousPlayback {
		t.Fatal("display metadata or server playback modes missing")
	}
}

func TestMetadataChangesDoNotChangeAudioIdentity(t *testing.T) {
	a := &Playlist{ID: "p", Songs: []Song{{ID: "s", Title: "Anterior"}}}
	b := &Playlist{ID: "p", Songs: []Song{{ID: "s", Title: "Nuevo"}}}
	if !samePlaylist(a, b) {
		t.Fatal("title update would restart audio")
	}
	one := Config{Zones: []Zone{{ID: "z", Playlist: a}}}
	two := Config{Zones: []Zone{{ID: "z", Playlist: b}}}
	if sameSnapshot(one, two) {
		t.Fatal("conflicting metadata in the same revision accepted")
	}
	b.Songs[0].Title = "Anterior"
	two.Zones[0].Shuffle = true
	if sameSnapshot(one, two) {
		t.Fatal("conflicting playback modes in the same revision accepted")
	}
}
