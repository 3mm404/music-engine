package control

import (
	"errors"
	"time"
	_ "time/tzdata" // Windows does not necessarily provide an IANA time zone database.
)

func validateSchedule(schedule *Schedule) error {
	if schedule == nil {
		return nil
	}
	if schedule.ID == "" || schedule.Name == "" || schedule.Slots == nil {
		return errors.New("programacion invalida")
	}
	if _, err := time.LoadLocation(schedule.Timezone); err != nil || schedule.Timezone == "" {
		return errors.New("zona horaria de programacion invalida")
	}
	seen := map[string]bool{}
	for _, slot := range schedule.Slots {
		if slot.ID == "" || seen[slot.ID] || slot.PlaylistID == "" || len(slot.Days) == 0 {
			return errors.New("franja de programacion invalida")
		}
		seen[slot.ID] = true
		days := map[int]bool{}
		for _, day := range slot.Days {
			if day < 1 || day > 7 || days[day] {
				return errors.New("dias de programacion invalidos")
			}
			days[day] = true
		}
		for _, clock := range []string{slot.StartsAt, slot.EndsAt} {
			if len(clock) != 5 {
				return errors.New("hora de programacion invalida")
			}
			if _, err := time.Parse("15:04", clock); err != nil {
				return errors.New("hora de programacion invalida")
			}
		}
		if slot.CrossesMidnight != (slot.EndsAt <= slot.StartsAt) {
			return errors.New("cruce de medianoche inconsistente")
		}
		if slot.Volume != nil && (*slot.Volume < 0 || *slot.Volume > 100) {
			return errors.New("volumen de programacion invalido")
		}
	}
	return nil
}

// A desktop observer cannot mutate the configuration through shared slices/pointers.
func cloneSchedule(source *Schedule) *Schedule {
	if source == nil {
		return nil
	}
	result := *source
	result.Slots = make([]ScheduleSlot, len(source.Slots))
	for i, slot := range source.Slots {
		result.Slots[i] = slot
		result.Slots[i].Days = append([]int{}, slot.Days...)
		if slot.Volume != nil {
			volume := *slot.Volume
			result.Slots[i].Volume = &volume
		}
	}
	return &result
}
