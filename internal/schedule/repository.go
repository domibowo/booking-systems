package schedule

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type GormWeeklyHoursRepository struct {
	db *gorm.DB
}

func NewGormWeeklyHoursRepository(db *gorm.DB) *GormWeeklyHoursRepository {
	return &GormWeeklyHoursRepository{db: db}
}

// ReplaceForResource atomically swaps the full week's hours for a resource.
func (r *GormWeeklyHoursRepository) ReplaceForResource(ctx context.Context, resourceID uuid.UUID, hours []WeeklyHours) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("resource_id = ?", resourceID).Delete(&WeeklyHours{}).Error; err != nil {
			return err
		}
		if len(hours) == 0 {
			return nil
		}
		return tx.Create(&hours).Error
	})
}

func (r *GormWeeklyHoursRepository) ListByResource(ctx context.Context, resourceID uuid.UUID) ([]WeeklyHours, error) {
	var hours []WeeklyHours
	err := r.db.WithContext(ctx).
		Where("resource_id = ?", resourceID).
		Order("weekday asc").
		Find(&hours).Error
	if err != nil {
		return nil, err
	}
	return hours, nil
}

func (r *GormWeeklyHoursRepository) FindByResourceAndWeekday(ctx context.Context, resourceID uuid.UUID, weekday int) (*WeeklyHours, error) {
	var wh WeeklyHours
	err := r.db.WithContext(ctx).
		Where("resource_id = ? AND weekday = ?", resourceID, weekday).
		First(&wh).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &wh, nil
}

type GormExceptionRepository struct {
	db *gorm.DB
}

func NewGormExceptionRepository(db *gorm.DB) *GormExceptionRepository {
	return &GormExceptionRepository{db: db}
}

func (r *GormExceptionRepository) Create(ctx context.Context, e *ScheduleException) error {
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *GormExceptionRepository) FindByID(ctx context.Context, id uuid.UUID) (*ScheduleException, error) {
	var e ScheduleException
	err := r.db.WithContext(ctx).First(&e, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *GormExceptionRepository) ListByResource(ctx context.Context, resourceID uuid.UUID) ([]ScheduleException, error) {
	var exceptions []ScheduleException
	err := r.db.WithContext(ctx).
		Where("resource_id = ?", resourceID).
		Order("date asc").
		Find(&exceptions).Error
	if err != nil {
		return nil, err
	}
	return exceptions, nil
}

func (r *GormExceptionRepository) FindByResourceAndDate(ctx context.Context, resourceID uuid.UUID, date time.Time) (*ScheduleException, error) {
	var e ScheduleException
	err := r.db.WithContext(ctx).
		Where("resource_id = ? AND date = ?", resourceID, date.Format("2006-01-02")).
		First(&e).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *GormExceptionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&ScheduleException{}).Error
}

type GormSettingsRepository struct {
	db *gorm.DB
}

func NewGormSettingsRepository(db *gorm.DB) *GormSettingsRepository {
	return &GormSettingsRepository{db: db}
}

func (r *GormSettingsRepository) FindByResource(ctx context.Context, resourceID uuid.UUID) (*ScheduleSettings, error) {
	var s ScheduleSettings
	err := r.db.WithContext(ctx).First(&s, "resource_id = ?", resourceID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Upsert uses an explicit ON CONFLICT clause rather than GORM's Save,
// because ResourceID (the primary key) is always set by the caller — Save
// would assume the row already exists and emit an UPDATE that silently
// affects 0 rows on first write.
func (r *GormSettingsRepository) Upsert(ctx context.Context, s *ScheduleSettings) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "resource_id"}},
		UpdateAll: true,
	}).Create(s).Error
}
