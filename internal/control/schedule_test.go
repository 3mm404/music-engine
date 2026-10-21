package control

import (
	"encoding/json"
	"testing"
)

func testSchedule() *Schedule {
	volume := 40
	return &Schedule{ID: "s", Name: "Horario restaurante", Timezone: "America/Cancun", StopOutsideSlots: true,
		Slots: []ScheduleSlot{{ID: "night", Days: []int{5, 6}, StartsAt: "22:00", EndsAt: "02:00", CrossesMidnight: true, PlaylistID: "p", PlaylistName: "Lounge", Volume: &volume}}}
}

func TestScheduleIsDecodedAndExposedAsReadOnlyDesktopInformation(t *testing.T) {
	source := testConfig(1, 50)
	source.Zones[0].Schedule = testSchedule()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var received Config
	if err := json.Unmarshal(data, &received); err != nil {
		t.Fatal(err)
	}
	runtime := &Runtime{deviceID: "1"}
	if err := runtime.apply(received); err != nil {
		t.Fatal(err)
	}
	schedule := runtime.desktopSnapshot().Zones[0].Schedule
	if schedule.Name != "Horario restaurante" || schedule.Timezone != "America/Cancun" || schedule.Slots[0].PlaylistName != "Lounge" || !schedule.Slots[0].CrossesMidnight || *schedule.Slots[0].Volume != 40 {
		t.Fatal("schedule display information lost")
	}
	schedule.Slots[0].Days[0] = 1
	*schedule.Slots[0].Volume = 99
	if runtime.config.Zones[0].Schedule.Slots[0].Days[0] != 5 || *runtime.config.Zones[0].Schedule.Slots[0].Volume != 40 {
		t.Fatal("observer changed server configuration")
	}
	if runtime.report().Zones[0].State != "stopped" {
		t.Fatal("schedule information started playback locally")
	}
	if _, ok := findDesktopOperation("schedule_read"); ok {
		t.Fatal("read capability became an executable command")
	}
}

func TestScheduleValidationDoesNotReplaceLastGoodConfiguration(t *testing.T) {
	cases := map[string]func(*Schedule){
		"timezone":      func(s *Schedule) { s.Timezone = "Invalid/Zone" },
		"day":           func(s *Schedule) { s.Slots[0].Days = []int{0} },
		"duplicate day": func(s *Schedule) { s.Slots[0].Days = []int{1, 1} },
		"time":          func(s *Schedule) { s.Slots[0].StartsAt = "25:00" },
		"midnight":      func(s *Schedule) { s.Slots[0].CrossesMidnight = false },
		"volume":        func(s *Schedule) { v := 101; s.Slots[0].Volume = &v },
		"playlist":      func(s *Schedule) { s.Slots[0].PlaylistID = "" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			runtime := &Runtime{deviceID: "1"}
			if err := runtime.apply(testConfig(1, 50)); err != nil {
				t.Fatal(err)
			}
			next := testConfig(2, 50)
			next.Zones[0].Schedule = testSchedule()
			mutate(next.Zones[0].Schedule)
			if err := runtime.apply(next); err == nil {
				t.Fatal("invalid schedule accepted")
			}
			if runtime.config.Revision != 1 {
				t.Fatal("partially replaced valid configuration")
			}
		})
	}
}

func TestScheduleChangesPreserveAudioIdentityAndLegacyServers(t *testing.T) {
	legacy := testConfig(1, 50)
	runtime := &Runtime{deviceID: "1"}
	if err := runtime.apply(legacy); err != nil {
		t.Fatal(err)
	}
	if runtime.desktopSnapshot().Zones[0].Schedule != nil {
		t.Fatal("invented schedule")
	}
	updated := testConfig(2, 50)
	updated.Zones[0].Schedule = testSchedule()
	if sameSnapshot(legacy, updated) {
		t.Fatal("schedule change ignored")
	}
	if !samePlaylist(legacy.Zones[0].Playlist, updated.Zones[0].Playlist) {
		t.Fatal("schedule metadata would restart current audio")
	}
	updated.Zones[0].Schedule.Slots = []ScheduleSlot{}
	if err := runtime.apply(updated); err != nil {
		t.Fatalf("empty schedule rejected: %v", err)
	}
}
