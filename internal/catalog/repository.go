package catalog

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type GormBusinessRepository struct {
	db *gorm.DB
}

func NewGormBusinessRepository(db *gorm.DB) *GormBusinessRepository {
	return &GormBusinessRepository{db: db}
}

func (r *GormBusinessRepository) Create(ctx context.Context, b *Business) error {
	return r.db.WithContext(ctx).Create(b).Error
}

func (r *GormBusinessRepository) FindByID(ctx context.Context, id uuid.UUID) (*Business, error) {
	var b Business
	err := r.db.WithContext(ctx).First(&b, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *GormBusinessRepository) List(ctx context.Context, limit, offset int) ([]Business, int64, error) {
	var businesses []Business
	var total int64

	if err := r.db.WithContext(ctx).Model(&Business{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := r.db.WithContext(ctx).Order("created_at desc").Limit(limit).Offset(offset).Find(&businesses).Error; err != nil {
		return nil, 0, err
	}
	return businesses, total, nil
}

func (r *GormBusinessRepository) Update(ctx context.Context, b *Business) error {
	return r.db.WithContext(ctx).Save(b).Error
}

// Delete removes the business along with its resources and their services.
// There's no DB-level foreign key (models stay decoupled, repository-pattern
// style), so the cascade is done explicitly in one transaction.
func (r *GormBusinessRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resourceIDs := tx.Model(&Resource{}).Select("id").Where("business_id = ?", id)
		if err := tx.Where("resource_id IN (?)", resourceIDs).Delete(&Service{}).Error; err != nil {
			return err
		}
		if err := tx.Where("business_id = ?", id).Delete(&Resource{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&Business{}).Error
	})
}

type GormResourceRepository struct {
	db *gorm.DB
}

func NewGormResourceRepository(db *gorm.DB) *GormResourceRepository {
	return &GormResourceRepository{db: db}
}

func (r *GormResourceRepository) Create(ctx context.Context, res *Resource) error {
	return r.db.WithContext(ctx).Create(res).Error
}

func (r *GormResourceRepository) FindByID(ctx context.Context, id uuid.UUID) (*Resource, error) {
	var res Resource
	err := r.db.WithContext(ctx).First(&res, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &res, nil
}

func (r *GormResourceRepository) ListByBusiness(ctx context.Context, businessID uuid.UUID, limit, offset int) ([]Resource, int64, error) {
	var resources []Resource
	var total int64

	q := r.db.WithContext(ctx).Model(&Resource{}).Where("business_id = ?", businessID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at desc").Limit(limit).Offset(offset).Find(&resources).Error; err != nil {
		return nil, 0, err
	}
	return resources, total, nil
}

func (r *GormResourceRepository) Update(ctx context.Context, res *Resource) error {
	return r.db.WithContext(ctx).Save(res).Error
}

func (r *GormResourceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("resource_id = ?", id).Delete(&Service{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", id).Delete(&Resource{}).Error
	})
}

type GormServiceRepository struct {
	db *gorm.DB
}

func NewGormServiceRepository(db *gorm.DB) *GormServiceRepository {
	return &GormServiceRepository{db: db}
}

func (r *GormServiceRepository) Create(ctx context.Context, s *Service) error {
	return r.db.WithContext(ctx).Create(s).Error
}

func (r *GormServiceRepository) FindByID(ctx context.Context, id uuid.UUID) (*Service, error) {
	var s Service
	err := r.db.WithContext(ctx).First(&s, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *GormServiceRepository) ListByResource(ctx context.Context, resourceID uuid.UUID, limit, offset int) ([]Service, int64, error) {
	var services []Service
	var total int64

	q := r.db.WithContext(ctx).Model(&Service{}).Where("resource_id = ?", resourceID)
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	if err := q.Order("created_at desc").Limit(limit).Offset(offset).Find(&services).Error; err != nil {
		return nil, 0, err
	}
	return services, total, nil
}

func (r *GormServiceRepository) Update(ctx context.Context, s *Service) error {
	return r.db.WithContext(ctx).Save(s).Error
}

func (r *GormServiceRepository) Delete(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&Service{}).Error
}
