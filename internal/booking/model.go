package booking

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusConfirmed Status = "confirmed"
	StatusCancelled Status = "cancelled"
	StatusCompleted Status = "completed"
	StatusExpired   Status = "expired"
	StatusNoShow    Status = "no_show"
)

// Booking is the core record: a customer holding (and, once confirmed,
// occupying) a resource for [StartAt, EndAt).
//
// BlockedUntil = EndAt + the resource's buffer-between-bookings, and is what
// the DB-level exclusion constraint (see schema.go) actually checks for
// overlap — baking the buffer requirement into the same guarantee that
// prevents double-booking, rather than enforcing it only in application code.
//
// BusinessID is denormalized from the resource at creation time so the
// owner's booking list can filter by business without joining into the
// catalog module's tables.
type Booking struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	BusinessID     uuid.UUID `gorm:"type:uuid;not null;index"`
	ResourceID     uuid.UUID `gorm:"type:uuid;not null;index"`
	ServiceID      uuid.UUID `gorm:"type:uuid;not null"`
	CustomerID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Status         Status    `gorm:"type:varchar(20);not null;index"`
	StartAt        time.Time `gorm:"not null"`
	EndAt          time.Time `gorm:"not null"`
	BlockedUntil   time.Time `gorm:"not null"`
	HoldExpiresAt  *time.Time
	IdempotencyKey *string `gorm:"type:text"`
	Notes          string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (b *Booking) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

// EffectiveStatus is what should be shown to API consumers: a pending hold
// whose expiry has passed reads as "expired" even before the (not yet
// built) background job writes that back to the row. See GetAvailability's
// query-time filtering for why conflict-prevention doesn't depend on this
// write-back happening promptly.
func (b *Booking) EffectiveStatus(now time.Time) Status {
	if b.Status == StatusPending && b.HoldExpiresAt != nil && b.HoldExpiresAt.Before(now) {
		return StatusExpired
	}
	return b.Status
}
