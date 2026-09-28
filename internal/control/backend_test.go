package control

import (
	"music-engine/internal/audio/oto"
	"music-engine/internal/decoder"
	"music-engine/internal/engine"
	"testing"
)

func TestOtoSharedZonesRejectPhysicalRoutes(t *testing.T) {
	c := audioConfig("https://example.test/music.mp3", nil)
	c.Zones[0].Playlist = nil
	second := c.Zones[0]
	second.ID = "second"
	c.Zones = append(c.Zones, second)
	if err := validateAudioForOutput(c, true); err != nil {
		t.Fatal(err)
	}
	if err := validateAudioForOutput(c, false); err == nil {
		t.Fatal("ASIO accepted shared physical channels")
	}
	m, err := engine.NewRemoteWithOutput(oto.NewOutput(), decoder.HTTPS{})
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	r := &Runtime{audio: m, sharedStereo: true}
	if err := r.applyAudio(c); err != nil {
		t.Fatal(err)
	}
	c.Zones[1].Output = &Output{DeviceID: "default", Channels: []int{3, 4}}
	if err := validateAudioForOutput(c, true); err == nil {
		t.Fatal("Oto accepted independent physical outputs")
	}
	c.Zones[1].Output = nil
	if err := validateAudioForOutput(c, true); err != nil {
		t.Fatal(err)
	}
	if err := r.applyAudio(c); err != nil {
		t.Fatal(err)
	}
}
