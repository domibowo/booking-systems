package booking

import (
	"testing"
	"time"

	"github.com/domibowo/booking-systems/internal/schedule"
)

func mustSlot(startOffset, endOffset time.Duration) schedule.TimeSlot {
	base := time.Date(2026, 11, 16, 2, 0, 0, 0, time.UTC)
	return schedule.TimeSlot{StartAt: base.Add(startOffset), EndAt: base.Add(endOffset)}
}

func mustBooking(startOffset, blockedUntilOffset time.Duration) Booking {
	base := time.Date(2026, 11, 16, 2, 0, 0, 0, time.UTC)
	return Booking{StartAt: base.Add(startOffset), BlockedUntil: base.Add(blockedUntilOffset)}
}

func TestFilterAvailableSlots(t *testing.T) {
	hour := time.Hour

	tests := []struct {
		name      string
		slots     []schedule.TimeSlot
		booked    []Booking
		wantCount int
	}{
		{
			name:      "no bookings keeps all slots",
			slots:     []schedule.TimeSlot{mustSlot(0, hour), mustSlot(hour, 2*hour)},
			booked:    nil,
			wantCount: 2,
		},
		{
			name:      "exact overlap removes the slot",
			slots:     []schedule.TimeSlot{mustSlot(0, hour), mustSlot(hour, 2*hour)},
			booked:    []Booking{mustBooking(0, hour)},
			wantCount: 1,
		},
		{
			name:      "partial overlap removes the slot",
			slots:     []schedule.TimeSlot{mustSlot(0, hour)},
			booked:    []Booking{mustBooking(30*time.Minute, 90*time.Minute)},
			wantCount: 0,
		},
		{
			name:      "booking exactly adjacent (touching, not overlapping) keeps the slot",
			slots:     []schedule.TimeSlot{mustSlot(0, hour)},
			booked:    []Booking{mustBooking(hour, 2*hour)},
			wantCount: 1,
		},
		{
			name:      "buffer extends the blocked range into the next slot",
			slots:     []schedule.TimeSlot{mustSlot(0, hour), mustSlot(hour+15*time.Minute, 2*hour+15*time.Minute)},
			booked:    []Booking{mustBooking(0, hour+15*time.Minute)},
			wantCount: 1,
		},
		{
			name:      "multiple bookings each remove their own slot",
			slots:     []schedule.TimeSlot{mustSlot(0, hour), mustSlot(hour, 2*hour), mustSlot(2*hour, 3*hour)},
			booked:    []Booking{mustBooking(0, hour), mustBooking(2*hour, 3*hour)},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterAvailableSlots(tt.slots, tt.booked)
			if len(got) != tt.wantCount {
				t.Fatalf("got %d slots, want %d (%+v)", len(got), tt.wantCount, got)
			}
		})
	}
}
