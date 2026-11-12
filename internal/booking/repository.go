package booking

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

const pgExclusionViolation = "23P01"

type GormRepository struct {
	db *gorm.DB
}

func NewGormRepository(db *gorm.DB) *GormRepository {
	return &GormRepository{db: db}
}

// Create sweeps stale pending holds for this resource before inserting, so
// a hold whose expiry has passed (but hasn't been written back as "expired"
// yet — there's no background job for that) doesn't wrongly block a new
// booking at the exclusion-constraint level. Both statements run in one
// transaction so the sweep is visible to the INSERT's constraint check.
func (r *GormRepository) Create(ctx context.Context, b *Booking) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Booking{}).
			Where("resource_id = ? AND status = ? AND hold_expires_at < ?", b.ResourceID, StatusPending, time.Now()).
			Update("status", StatusExpired).Error; err != nil {
			return err
		}

		if err := tx.Create(b).Error; err != nil {
			return mapCreateErr(err)
		}
		return nil
	})
}

func mapCreateErr(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgExclusionViolation {
		return ErrSlotConflict
	}
	return err
}

func (r *GormRepository) FindByID(ctx context.Context, id uuid.UUID) (*Booking, error) {
	var b Booking
	err := r.db.WithContext(ctx).First(&b, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *GormRepository) FindByCustomerAndIdempotencyKey(ctx context.Context, customerID uuid.UUID, key string) (*Booking, error) {
	var b Booking
	err := r.db.WithContext(ctx).
		Where("customer_id = ? AND idempotency_key = ?", customerID, key).
		First(&b).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *GormRepository) ListByCustomer(ctx context.Context, customerID uuid.UUID, status *Status, limit, offset int) ([]Booking, int64, error) {
	q := r.db.WithContext(ctx).Model(&Booking{}).Where("customer_id = ?", customerID)
	if status != nil {
		q = q.Where("status = ?", *status)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var bookings []Booking
	if err := q.Order("start_at desc").Limit(limit).Offset(offset).Find(&bookings).Error; err != nil {
		return nil, 0, err
	}
	return bookings, total, nil
}

func (r *GormRepository) ListByBusiness(ctx context.Context, businessID uuid.UUID, filter ListFilter, limit, offset int) ([]Booking, int64, error) {
	q := r.db.WithContext(ctx).Model(&Booking{}).Where("business_id = ?", businessID)
	if filter.ResourceID != nil {
		q = q.Where("resource_id = ?", *filter.ResourceID)
	}
	if filter.Status != nil {
		q = q.Where("status = ?", *filter.Status)
	}
	if filter.From != nil {
		q = q.Where("start_at >= ?", *filter.From)
	}
	if filter.To != nil {
		q = q.Where("start_at < ?", *filter.To)
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var bookings []Booking
	if err := q.Order("start_at desc").Limit(limit).Offset(offset).Find(&bookings).Error; err != nil {
		return nil, 0, err
	}
	return bookings, total, nil
}

// ListActiveOverlapping returns bookings for resourceID that are pending
// (with a still-live hold) or confirmed, and whose [start_at, blocked_until)
// overlaps [rangeStart, rangeEnd). Used to filter booking-aware
// availability — this is a plain query-time filter (not an index
// predicate), so unlike the exclusion constraint it can safely compare
// against the current time directly.
func (r *GormRepository) ListActiveOverlapping(ctx context.Context, resourceID uuid.UUID, rangeStart, rangeEnd time.Time) ([]Booking, error) {
	var bookings []Booking
	now := time.Now()
	err := r.db.WithContext(ctx).
		Where("resource_id = ?", resourceID).
		Where("status IN ?", []Status{StatusPending, StatusConfirmed}).
		Where("NOT (status = ? AND hold_expires_at < ?)", StatusPending, now).
		Where("start_at < ? AND blocked_until > ?", rangeEnd, rangeStart).
		Find(&bookings).Error
	if err != nil {
		return nil, err
	}
	return bookings, nil
}

func (r *GormRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status Status) error {
	return r.db.WithContext(ctx).
		Model(&Booking{}).
		Where("id = ?", id).
		Update("status", status).Error
}
