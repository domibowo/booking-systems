package catalog

import (
	"context"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrBusinessNotFound = errors.New("catalog: business not found")
	ErrResourceNotFound = errors.New("catalog: resource not found")
	ErrServiceNotFound  = errors.New("catalog: service not found")
	ErrForbidden        = errors.New("catalog: forbidden")
)

// Repository interfaces are defined here, on the consumer side, per the
// project's dependency-inversion convention.
type BusinessRepository interface {
	Create(ctx context.Context, b *Business) error
	FindByID(ctx context.Context, id uuid.UUID) (*Business, error)
	List(ctx context.Context, limit, offset int) ([]Business, int64, error)
	Update(ctx context.Context, b *Business) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type ResourceRepository interface {
	Create(ctx context.Context, r *Resource) error
	FindByID(ctx context.Context, id uuid.UUID) (*Resource, error)
	ListByBusiness(ctx context.Context, businessID uuid.UUID, limit, offset int) ([]Resource, int64, error)
	Update(ctx context.Context, r *Resource) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type ServiceRepository interface {
	Create(ctx context.Context, s *Service) error
	FindByID(ctx context.Context, id uuid.UUID) (*Service, error)
	ListByResource(ctx context.Context, resourceID uuid.UUID, limit, offset int) ([]Service, int64, error)
	Update(ctx context.Context, s *Service) error
	Delete(ctx context.Context, id uuid.UUID) error
}

// Catalog is the business-logic orchestrator for businesses, resources, and
// services. Named Catalog (rather than "Service", the usual per-module
// convention) because the "Service" identifier is already taken by the
// Service domain model (a bookable offering/package).
type Catalog struct {
	businesses BusinessRepository
	resources  ResourceRepository
	services   ServiceRepository
}

func NewCatalog(businesses BusinessRepository, resources ResourceRepository, services ServiceRepository) *Catalog {
	return &Catalog{businesses: businesses, resources: resources, services: services}
}

type CreateBusinessInput struct {
	Name        string
	Address     string
	Phone       string
	Description string
}

type UpdateBusinessInput = CreateBusinessInput

type CreateResourceInput struct {
	Name        string
	Type        string
	Description string
}

type UpdateResourceInput = CreateResourceInput

type CreateServiceInput struct {
	Name            string
	DurationMinutes int
	PriceCents      int64
	Currency        string
	Description     string
}

type UpdateServiceInput = CreateServiceInput

// --- business ---

func (c *Catalog) CreateBusiness(ctx context.Context, ownerID uuid.UUID, in CreateBusinessInput) (*Business, error) {
	b := &Business{
		OwnerID:     ownerID,
		Name:        in.Name,
		Address:     in.Address,
		Phone:       in.Phone,
		Description: in.Description,
	}
	if err := c.businesses.Create(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (c *Catalog) GetBusiness(ctx context.Context, id uuid.UUID) (*Business, error) {
	b, err := c.businesses.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, ErrBusinessNotFound
	}
	return b, nil
}

func (c *Catalog) ListBusinesses(ctx context.Context, limit, offset int) ([]Business, int64, error) {
	return c.businesses.List(ctx, limit, offset)
}

func (c *Catalog) UpdateBusiness(ctx context.Context, id, userID uuid.UUID, isSuperadmin bool, in UpdateBusinessInput) (*Business, error) {
	b, err := c.ensureBusinessOwner(ctx, id, userID, isSuperadmin)
	if err != nil {
		return nil, err
	}
	b.Name = in.Name
	b.Address = in.Address
	b.Phone = in.Phone
	b.Description = in.Description
	if err := c.businesses.Update(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (c *Catalog) DeleteBusiness(ctx context.Context, id, userID uuid.UUID, isSuperadmin bool) error {
	if _, err := c.ensureBusinessOwner(ctx, id, userID, isSuperadmin); err != nil {
		return err
	}
	return c.businesses.Delete(ctx, id)
}

func (c *Catalog) ensureBusinessOwner(ctx context.Context, businessID, userID uuid.UUID, isSuperadmin bool) (*Business, error) {
	b, err := c.businesses.FindByID(ctx, businessID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, ErrBusinessNotFound
	}
	if !isSuperadmin && b.OwnerID != userID {
		return nil, ErrForbidden
	}
	return b, nil
}

// --- resource ---

func (c *Catalog) CreateResource(ctx context.Context, businessID, userID uuid.UUID, isSuperadmin bool, in CreateResourceInput) (*Resource, error) {
	if _, err := c.ensureBusinessOwner(ctx, businessID, userID, isSuperadmin); err != nil {
		return nil, err
	}
	r := &Resource{
		BusinessID:  businessID,
		Name:        in.Name,
		Type:        in.Type,
		Description: in.Description,
	}
	if err := c.resources.Create(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (c *Catalog) GetResource(ctx context.Context, id uuid.UUID) (*Resource, error) {
	r, err := c.resources.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrResourceNotFound
	}
	return r, nil
}

func (c *Catalog) ListResources(ctx context.Context, businessID uuid.UUID, limit, offset int) ([]Resource, int64, error) {
	return c.resources.ListByBusiness(ctx, businessID, limit, offset)
}

func (c *Catalog) UpdateResource(ctx context.Context, id, userID uuid.UUID, isSuperadmin bool, in UpdateResourceInput) (*Resource, error) {
	r, err := c.ensureResourceOwner(ctx, id, userID, isSuperadmin)
	if err != nil {
		return nil, err
	}
	r.Name = in.Name
	r.Type = in.Type
	r.Description = in.Description
	if err := c.resources.Update(ctx, r); err != nil {
		return nil, err
	}
	return r, nil
}

func (c *Catalog) DeleteResource(ctx context.Context, id, userID uuid.UUID, isSuperadmin bool) error {
	if _, err := c.ensureResourceOwner(ctx, id, userID, isSuperadmin); err != nil {
		return err
	}
	return c.resources.Delete(ctx, id)
}

func (c *Catalog) ensureResourceOwner(ctx context.Context, resourceID, userID uuid.UUID, isSuperadmin bool) (*Resource, error) {
	r, err := c.resources.FindByID(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, ErrResourceNotFound
	}
	if _, err := c.ensureBusinessOwner(ctx, r.BusinessID, userID, isSuperadmin); err != nil {
		return nil, err
	}
	return r, nil
}

// CheckResourceOwnership lets other modules (e.g. schedule, booking) reuse
// this module's ownership chain (resource -> business -> owner) without
// duplicating it.
func (c *Catalog) CheckResourceOwnership(ctx context.Context, resourceID, userID uuid.UUID, isSuperadmin bool) error {
	_, err := c.ensureResourceOwner(ctx, resourceID, userID, isSuperadmin)
	return err
}

// CheckBusinessOwnership is the business-level counterpart, for modules
// (e.g. booking's owner-scoped list) that are scoped by business rather
// than by resource.
func (c *Catalog) CheckBusinessOwnership(ctx context.Context, businessID, userID uuid.UUID, isSuperadmin bool) error {
	_, err := c.ensureBusinessOwner(ctx, businessID, userID, isSuperadmin)
	return err
}

// --- service (offering) ---

func (c *Catalog) CreateService(ctx context.Context, resourceID, userID uuid.UUID, isSuperadmin bool, in CreateServiceInput) (*Service, error) {
	if _, err := c.ensureResourceOwner(ctx, resourceID, userID, isSuperadmin); err != nil {
		return nil, err
	}
	s := &Service{
		ResourceID:      resourceID,
		Name:            in.Name,
		DurationMinutes: in.DurationMinutes,
		PriceCents:      in.PriceCents,
		Currency:        in.Currency,
		Description:     in.Description,
	}
	if err := c.services.Create(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (c *Catalog) GetService(ctx context.Context, id uuid.UUID) (*Service, error) {
	s, err := c.services.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, ErrServiceNotFound
	}
	return s, nil
}

func (c *Catalog) ListServices(ctx context.Context, resourceID uuid.UUID, limit, offset int) ([]Service, int64, error) {
	return c.services.ListByResource(ctx, resourceID, limit, offset)
}

func (c *Catalog) UpdateService(ctx context.Context, id, userID uuid.UUID, isSuperadmin bool, in UpdateServiceInput) (*Service, error) {
	s, err := c.ensureServiceOwner(ctx, id, userID, isSuperadmin)
	if err != nil {
		return nil, err
	}
	s.Name = in.Name
	s.DurationMinutes = in.DurationMinutes
	s.PriceCents = in.PriceCents
	s.Currency = in.Currency
	s.Description = in.Description
	if err := c.services.Update(ctx, s); err != nil {
		return nil, err
	}
	return s, nil
}

func (c *Catalog) DeleteService(ctx context.Context, id, userID uuid.UUID, isSuperadmin bool) error {
	if _, err := c.ensureServiceOwner(ctx, id, userID, isSuperadmin); err != nil {
		return err
	}
	return c.services.Delete(ctx, id)
}

func (c *Catalog) ensureServiceOwner(ctx context.Context, serviceID, userID uuid.UUID, isSuperadmin bool) (*Service, error) {
	s, err := c.services.FindByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, ErrServiceNotFound
	}
	if _, err := c.ensureResourceOwner(ctx, s.ResourceID, userID, isSuperadmin); err != nil {
		return nil, err
	}
	return s, nil
}
