package schedule

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WeeklyHours is the recurring operating hours for a resource on one weekday
// (0=Sunday..6=Saturday, matching time.Weekday). OpenTime/CloseTime are
// "HH:MM" wall-clock strings interpreted in the resource's ScheduleSettings
// timezone — never UTC directly, since operating hours are inherently local.
type WeeklyHours struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	ResourceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_resource_weekday"`
	Weekday    int       `gorm:"not null;uniqueIndex:idx_resource_weekday"`
	IsClosed   bool      `gorm:"not null;default:false"`
	OpenTime   string    `gorm:"type:varchar(5)"`
	CloseTime  string    `gorm:"type:varchar(5)"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func (w *WeeklyHours) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}

// ScheduleException overrides WeeklyHours for one specific calendar date —
// a holiday, a one-off closure, or shortened hours.
type ScheduleException struct {
	ID         uuid.UUID `gorm:"type:uuid;primaryKey"`
	ResourceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_resource_date"`
	Date       time.Time `gorm:"type:date;not null;uniqueIndex:idx_resource_date"`
	IsClosed   bool      `gorm:"not null;default:true"`
	OpenTime   *string   `gorm:"type:varchar(5)"`
	CloseTime  *string   `gorm:"type:varchar(5)"`
	Reason     string
	CreatedAt  time.Time
}

func (e *ScheduleException) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// ScheduleSettings holds the per-resource knobs needed to turn operating
// hours into bookable slots: which IANA timezone the wall-clock times above
// are expressed in, and the gap required between consecutive bookings.
type ScheduleSettings struct {
	ResourceID    uuid.UUID `gorm:"type:uuid;primaryKey"`
	Timezone      string    `gorm:"not null;default:Asia/Jakarta"`
	BufferMinutes int       `gorm:"not null;default:0"`
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

const DefaultTimezone = "Asia/Jakarta"
