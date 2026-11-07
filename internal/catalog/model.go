package catalog

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Business struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	OwnerID     uuid.UUID `gorm:"type:uuid;not null;index"`
	Name        string    `gorm:"not null"`
	Address     string
	Phone       string
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (b *Business) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

// Resource is a bookable unit within a business (a court, a room, a studio,
// a doctor) — the thing a booking is ultimately placed against.
type Resource struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	BusinessID  uuid.UUID `gorm:"type:uuid;not null;index"`
	Name        string    `gorm:"not null"`
	Type        string    `gorm:"not null"`
	Description string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r *Resource) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

// Service is a bookable offering/package on a resource (duration + price) —
// what a customer actually selects when booking.
type Service struct {
	ID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	ResourceID      uuid.UUID `gorm:"type:uuid;not null;index"`
	Name            string    `gorm:"not null"`
	DurationMinutes int       `gorm:"not null"`
	PriceCents      int64     `gorm:"not null"`
	Currency        string    `gorm:"type:varchar(3);not null;default:IDR"`
	Description     string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

func (s *Service) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}
