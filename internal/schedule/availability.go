package schedule

import (
	"fmt"
	"time"
)

type TimeSlot struct {
	StartAt time.Time
	EndAt   time.Time
}

// localWindow resolves "HH:MM" open/close wall-clock strings on the given
// calendar date into a [start, end) window expressed in UTC.
func localWindow(date time.Time, loc *time.Location, openTime, closeTime string) (time.Time, time.Time, error) {
	open, err := time.ParseInLocation("15:04", openTime, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("schedule: invalid open_time %q: %w", openTime, err)
	}
	close_, err := time.ParseInLocation("15:04", closeTime, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("schedule: invalid close_time %q: %w", closeTime, err)
	}

	openAt := time.Date(date.Year(), date.Month(), date.Day(), open.Hour(), open.Minute(), 0, 0, loc)
	closeAt := time.Date(date.Year(), date.Month(), date.Day(), close_.Hour(), close_.Minute(), 0, 0, loc)
	if !closeAt.After(openAt) {
		return time.Time{}, time.Time{}, fmt.Errorf("schedule: close_time %q must be after open_time %q", closeTime, openTime)
	}
	return openAt.UTC(), closeAt.UTC(), nil
}

// computeSlots lays out consecutive, non-overlapping bookable slots within
// [openAt, closeAt), each exactly slotDuration long and spaced apart by
// slotDuration+buffer. It does not know about existing bookings — that
// filtering happens once the booking module exists.
func computeSlots(openAt, closeAt time.Time, slotDuration, buffer time.Duration) []TimeSlot {
	if slotDuration <= 0 || !closeAt.After(openAt) {
		return nil
	}

	step := slotDuration + buffer
	var slots []TimeSlot
	for start := openAt; !start.Add(slotDuration).After(closeAt); start = start.Add(step) {
		slots = append(slots, TimeSlot{StartAt: start, EndAt: start.Add(slotDuration)})
	}
	return slots
}
