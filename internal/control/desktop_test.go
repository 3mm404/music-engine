package control

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDesktopSnapshotIncludesStateWithoutSignedURLs(t *testing.T) {
	r := &Runtime{config: Config{Revision: 7, Zones: []Zone{{ID: "one", Name: "Terraza", Volume: 50, ChannelMode: "stereo",
		Playlist: &Playlist{ID: "p", Name: "Relax", Songs: []Song{{ID: "s", URL: "https://example.test/audio?signature=secret"}}}}}}}
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
}
