package booking

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/domibowo/booking-systems/internal/catalog"
	"github.com/domibowo/booking-systems/internal/schedule"
)

var (
	ErrResourceNotFound  = errors.New("booking: resource not found")
	ErrServiceNotFound   = errors.New("booking: service not found")
	ErrBookingNotFound   = errors.New("booking: booking not found")
	ErrForbidden         = errors.New("booking: forbidden")
	ErrSlotInPast        = errors.New("booking: start time is in the past")
	ErrSlotNotAvailable  = errors.New("booking: requested time is outside operating hours")
	ErrSlotConflict      = errors.New("booking: requested time overlaps an existing booking")
	ErrInvalidTransition = errors.New("booking: invalid status transition")
)

// Repository interfaces are defined here, on the consumer side, per the
// project's dependency-inversion convention.
type Repository interface {
	Create(ctx context.Context, b *Booking) error
	FindByID(ctx context.Context, id uuid.UUID) (*Booking, error)
	FindByCustomerAndIdempotencyKey(ctx context.Context, customerID uuid.UUID, key string) (*Booking, error)
	ListByCustomer(ctx context.Context, customerID uuid.UUID, status *Status, limit, offset int) ([]Booking, int64, error)
	ListByBusiness(ctx context.Context, businessID uuid.UUID, filter ListFilter, limit, offset int) ([]Booking, int64, error)
	ListActiveOverlapping(ctx context.Context, resourceID uuid.UUID, rangeStart, rangeEnd time.Time) ([]Booking, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error
}

type ResourceReader interface {
	GetResource(ctx context.Context, id uuid.UUID) (*catalog.Resource, error)
}

type ServiceReader interface {
	GetService(ctx context.Context, id uuid.UUID) (*catalog.Service, error)
}

type ResourceOwnershipChecker interface {
	CheckResourceOwnership(ctx context.Context, resourceID, userID uuid.UUID, isSuperadmin bool) error
}

type BusinessOwnershipChecker interface {
	CheckBusinessOwnership(ctx context.Context, businessID, userID uuid.UUID, isSuperadmin bool) error
}

type AvailabilityChecker interface {
	IsWithinOperatingHours(ctx context.Context, resourceID uuid.UUID, startAt, endAt time.Time) (bool, error)
	GetAvailability(ctx context.Context, resourceID, serviceID uuid.UUID, date time.Time) (*schedule.AvailabilityResult, error)
	GetSettings(ctx context.Context, resourceID uuid.UUID) (*schedule.ScheduleSettings, error)
}

type Bookings struct {
	repo          Repository
	resources     ResourceReader
	services      ServiceReader
	resourceOwner ResourceOwnershipChecker
	businessOwner BusinessOwnershipChecker
	availability  AvailabilityChecker
	holdDuration  time.Duration
}

func NewBookings(
	repo Repository,
	resources ResourceReader,
	services ServiceReader,
	resourceOwner ResourceOwnershipChecker,
	businessOwner BusinessOwnershipChecker,
	availability AvailabilityChecker,
	holdDuration time.Duration,
) *Bookings {
	return &Bookings{
		repo:          repo,
		resources:     resources,
		services:      services,
		resourceOwner: resourceOwner,
		businessOwner: businessOwner,
		availability:  availability,
		holdDuration:  holdDuration,
	}
}

type CreateInput struct {
	ResourceID     uuid.UUID
	ServiceID      uuid.UUID
	StartAt        time.Time
	Notes          string
	IdempotencyKey *string
}

func (b *Bookings) Create(ctx context.Context, customerID uuid.UUID, in CreateInput) (*Booking, error) {
	if in.IdempotencyKey != nil {
		existing, err := b.repo.FindByCustomerAndIdempotencyKey(ctx, customerID, *in.IdempotencyKey)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return existing, nil
		}
	}

	if in.StartAt.Before(time.Now()) {
		return nil, ErrSlotInPast
	}

	resource, err := b.resources.GetResource(ctx, in.ResourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, ErrResourceNotFound
	}

	service, err := b.services.GetService(ctx, in.ServiceID)
	if err != nil {
		return nil, err
	}
	if service == nil || service.ResourceID != in.ResourceID {
		return nil, ErrServiceNotFound
	}

	endAt := in.StartAt.Add(time.Duration(service.DurationMinutes) * time.Minute)

	open, err := b.availability.IsWithinOperatingHours(ctx, in.ResourceID, in.StartAt, endAt)
	if err != nil {
		return nil, err
	}
	if !open {
		return nil, ErrSlotNotAvailable
	}

	settings, err := b.availability.GetSettings(ctx, in.ResourceID)
	if err != nil {
		return nil, err
	}
	bufferMinutes := 0
	if settings != nil {
		bufferMinutes = settings.BufferMinutes
	}

	holdExpiresAt := time.Now().Add(b.holdDuration)
	newBooking := &Booking{
		BusinessID:     resource.BusinessID,
		ResourceID:     in.ResourceID,
		ServiceID:      in.ServiceID,
		CustomerID:     customerID,
		Status:         StatusPending,
		StartAt:        in.StartAt,
		EndAt:          endAt,
		BlockedUntil:   endAt.Add(time.Duration(bufferMinutes) * time.Minute),
		HoldExpiresAt:  &holdExpiresAt,
		IdempotencyKey: in.IdempotencyKey,
		Notes:          in.Notes,
	}

	if err := b.repo.Create(ctx, newBooking); err != nil {
		return nil, err
	}
	return newBooking, nil
}

func (b *Bookings) Get(ctx context.Context, bookingID, userID uuid.UUID, isSuperadmin bool) (*Booking, error) {
	bk, err := b.repo.FindByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if bk == nil {
		return nil, ErrBookingNotFound
	}
	if bk.CustomerID == userID || isSuperadmin {
		return bk, nil
	}
	if err := b.resourceOwner.CheckResourceOwnership(ctx, bk.ResourceID, userID, isSuperadmin); err != nil {
		return nil, ErrForbidden
	}
	return bk, nil
}

func (b *Bookings) ListMine(ctx context.Context, customerID uuid.UUID, status *Status, limit, offset int) ([]Booking, int64, error) {
	return b.repo.ListByCustomer(ctx, customerID, status, limit, offset)
}

type ListFilter struct {
	ResourceID *uuid.UUID
	Status     *Status
	From       *time.Time
	To         *time.Time
}

func (b *Bookings) ListForBusiness(ctx context.Context, businessID, userID uuid.UUID, isSuperadmin bool, filter ListFilter, limit, offset int) ([]Booking, int64, error) {
	if err := b.businessOwner.CheckBusinessOwnership(ctx, businessID, userID, isSuperadmin); err != nil {
		return nil, 0, ErrForbidden
	}
	return b.repo.ListByBusiness(ctx, businessID, filter, limit, offset)
}

func (b *Bookings) Confirm(ctx context.Context, bookingID, userID uuid.UUID, isSuperadmin bool) (*Booking, error) {
	bk, err := b.ownedByResourceOwner(ctx, bookingID, userID, isSuperadmin)
	if err != nil {
		return nil, err
	}
	if bk.EffectiveStatus(time.Now()) != StatusPending {
		return nil, ErrInvalidTransition
	}
	if err := b.repo.UpdateStatus(ctx, bk.ID, StatusConfirmed); err != nil {
		return nil, err
	}
	bk.Status = StatusConfirmed
	return bk, nil
}

// Cancel is allowed for the booking's own customer or the resource's
// business owner/superadmin — unlike the other transitions, which are
// owner-only.
func (b *Bookings) Cancel(ctx context.Context, bookingID, userID uuid.UUID, isSuperadmin bool) (*Booking, error) {
	bk, err := b.repo.FindByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if bk == nil {
		return nil, ErrBookingNotFound
	}
	if bk.CustomerID != userID && !isSuperadmin {
		if err := b.resourceOwner.CheckResourceOwnership(ctx, bk.ResourceID, userID, isSuperadmin); err != nil {
			return nil, ErrForbidden
		}
	}

	switch bk.EffectiveStatus(time.Now()) {
	case StatusPending, StatusConfirmed:
	default:
		return nil, ErrInvalidTransition
	}

	if err := b.repo.UpdateStatus(ctx, bk.ID, StatusCancelled); err != nil {
		return nil, err
	}
	bk.Status = StatusCancelled
	return bk, nil
}

func (b *Bookings) Complete(ctx context.Context, bookingID, userID uuid.UUID, isSuperadmin bool) (*Booking, error) {
	bk, err := b.ownedByResourceOwner(ctx, bookingID, userID, isSuperadmin)
	if err != nil {
		return nil, err
	}
	if bk.Status != StatusConfirmed {
		return nil, ErrInvalidTransition
	}
	if err := b.repo.UpdateStatus(ctx, bk.ID, StatusCompleted); err != nil {
		return nil, err
	}
	bk.Status = StatusCompleted
	return bk, nil
}

func (b *Bookings) MarkNoShow(ctx context.Context, bookingID, userID uuid.UUID, isSuperadmin bool) (*Booking, error) {
	bk, err := b.ownedByResourceOwner(ctx, bookingID, userID, isSuperadmin)
	if err != nil {
		return nil, err
	}
	if bk.Status != StatusConfirmed {
		return nil, ErrInvalidTransition
	}
	if err := b.repo.UpdateStatus(ctx, bk.ID, StatusNoShow); err != nil {
		return nil, err
	}
	bk.Status = StatusNoShow
	return bk, nil
}

func (b *Bookings) ownedByResourceOwner(ctx context.Context, bookingID, userID uuid.UUID, isSuperadmin bool) (*Booking, error) {
	bk, err := b.repo.FindByID(ctx, bookingID)
	if err != nil {
		return nil, err
	}
	if bk == nil {
		return nil, ErrBookingNotFound
	}
	if err := b.resourceOwner.CheckResourceOwnership(ctx, bk.ResourceID, userID, isSuperadmin); err != nil {
		return nil, ErrForbidden
	}
	return bk, nil
}

// GetAvailableSlots composes the schedule module's raw capacity (operating
// hours minus exceptions) with this module's own booking records, so the
// result reflects real-time occupancy — the gap schedule.GetAvailability
// explicitly leaves open since it has no knowledge of bookings.
func (b *Bookings) GetAvailableSlots(ctx context.Context, resourceID, serviceID uuid.UUID, date time.Time) (*schedule.AvailabilityResult, error) {
	raw, err := b.availability.GetAvailability(ctx, resourceID, serviceID, date)
	if err != nil {
		return nil, mapScheduleErr(err)
	}
	if len(raw.Slots) == 0 {
		return raw, nil
	}

	rangeStart := raw.Slots[0].StartAt
	rangeEnd := raw.Slots[len(raw.Slots)-1].EndAt
	booked, err := b.repo.ListActiveOverlapping(ctx, resourceID, rangeStart, rangeEnd)
	if err != nil {
		return nil, err
	}

	raw.Slots = filterAvailableSlots(raw.Slots, booked)
	return raw, nil
}

func filterAvailableSlots(slots []schedule.TimeSlot, booked []Booking) []schedule.TimeSlot {
	if len(booked) == 0 {
		return slots
	}
	result := make([]schedule.TimeSlot, 0, len(slots))
	for _, s := range slots {
		conflict := false
		for _, bk := range booked {
			if s.StartAt.Before(bk.BlockedUntil) && bk.StartAt.Before(s.EndAt) {
				conflict = true
				break
			}
		}
		if !conflict {
			result = append(result, s)
		}
	}
	return result
}

func mapScheduleErr(err error) error {
	if errors.Is(err, schedule.ErrResourceNotFound) {
		return ErrResourceNotFound
	}
	if errors.Is(err, schedule.ErrServiceNotFound) {
		return ErrServiceNotFound
	}
	return err
}
