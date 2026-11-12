package schedule

import (
	"testing"
	"time"
)

func TestLocalWindow(t *testing.T) {
	jakarta, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	date := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		openTime     string
		closeTime    string
		wantErr      bool
		wantOpenUTC  string
		wantCloseUTC string
	}{
		{
			name:         "normal window",
			openTime:     "09:00",
			closeTime:    "17:00",
			wantOpenUTC:  "2026-11-15T02:00:00Z",
			wantCloseUTC: "2026-11-15T10:00:00Z",
		},
		{
			name:      "close before open",
			openTime:  "17:00",
			closeTime: "09:00",
			wantErr:   true,
		},
		{
			name:      "close equals open",
			openTime:  "09:00",
			closeTime: "09:00",
			wantErr:   true,
		},
		{
			name:      "invalid open time format",
			openTime:  "9am",
			closeTime: "17:00",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			openAt, closeAt, err := localWindow(date, jakarta, tt.openTime, tt.closeTime)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := openAt.Format(time.RFC3339); got != tt.wantOpenUTC {
				t.Errorf("openAt = %s, want %s", got, tt.wantOpenUTC)
			}
			if got := closeAt.Format(time.RFC3339); got != tt.wantCloseUTC {
				t.Errorf("closeAt = %s, want %s", got, tt.wantCloseUTC)
			}
		})
	}
}

func TestComputeSlots(t *testing.T) {
	open := time.Date(2026, 11, 15, 2, 0, 0, 0, time.UTC)    // 09:00 WIB
	close_ := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC) // 17:00 WIB

	tests := []struct {
		name          string
		openAt        time.Time
		closeAt       time.Time
		duration      time.Duration
		buffer        time.Duration
		wantCount     int
		wantFirstEnd  string
		wantLastStart string
	}{
		{
			name:          "60min slots with 15min buffer",
			openAt:        open,
			closeAt:       close_,
			duration:      60 * time.Minute,
			buffer:        15 * time.Minute,
			wantCount:     6,
			wantFirstEnd:  "2026-11-15T03:00:00Z",
			wantLastStart: "2026-11-15T08:15:00Z",
		},
		{
			name:          "back-to-back with no buffer",
			openAt:        open,
			closeAt:       close_,
			duration:      60 * time.Minute,
			buffer:        0,
			wantCount:     8,
			wantFirstEnd:  "2026-11-15T03:00:00Z",
			wantLastStart: "2026-11-15T09:00:00Z",
		},
		{
			name:      "zero duration yields no slots",
			openAt:    open,
			closeAt:   close_,
			duration:  0,
			buffer:    15 * time.Minute,
			wantCount: 0,
		},
		{
			name:      "window shorter than one slot yields no slots",
			openAt:    open,
			closeAt:   open.Add(30 * time.Minute),
			duration:  60 * time.Minute,
			buffer:    0,
			wantCount: 0,
		},
		{
			name:      "close before open yields no slots",
			openAt:    close_,
			closeAt:   open,
			duration:  60 * time.Minute,
			buffer:    0,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			slots := computeSlots(tt.openAt, tt.closeAt, tt.duration, tt.buffer)
			if len(slots) != tt.wantCount {
				t.Fatalf("got %d slots, want %d (%+v)", len(slots), tt.wantCount, slots)
			}
			if tt.wantCount == 0 {
				return
			}
			if got := slots[0].EndAt.Format(time.RFC3339); got != tt.wantFirstEnd {
				t.Errorf("first slot EndAt = %s, want %s", got, tt.wantFirstEnd)
			}
			last := slots[len(slots)-1]
			if got := last.StartAt.Format(time.RFC3339); got != tt.wantLastStart {
				t.Errorf("last slot StartAt = %s, want %s", got, tt.wantLastStart)
			}
			if last.EndAt.After(tt.closeAt) {
				t.Errorf("last slot EndAt %s is after closeAt %s", last.EndAt, tt.closeAt)
			}
		})
	}
}
