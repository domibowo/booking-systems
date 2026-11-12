package schedule

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/domibowo/booking-systems/internal/catalog"
)

var (
	ErrResourceNotFound  = errors.New("schedule: resource not found")
	ErrServiceNotFound   = errors.New("schedule: service not found")
	ErrExceptionNotFound = errors.New("schedule: exception not found")
	ErrInvalidWeekdays   = errors.New("schedule: must specify exactly one entry for each weekday 0-6")
	// ErrInvalidInput wraps validation failures (bad time format, close
	// before open, unknown timezone) so handlers can map them to 400
	// instead of falling through to a generic 500.
	ErrInvalidInput = errors.New("schedule: invalid input")
)

// Repository interfaces are defined here, on the consumer side, per the
// project's dependency-inversion convention.
type WeeklyHoursRepository interface {
	ReplaceForResource(ctx context.Context, resourceID uuid.UUID, hours []WeeklyHours) error
	ListByResource(ctx context.Context, resourceID uuid.UUID) ([]WeeklyHours, error)
	FindByResourceAndWeekday(ctx context.Context, resourceID uuid.UUID, weekday int) (*WeeklyHours, error)
}

type ExceptionRepository interface {
	Create(ctx context.Context, e *ScheduleException) error
	FindByID(ctx context.Context, id uuid.UUID) (*ScheduleException, error)
	ListByResource(ctx context.Context, resourceID uuid.UUID) ([]ScheduleException, error)
	FindByResourceAndDate(ctx context.Context, resourceID uuid.UUID, date time.Time) (*ScheduleException, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type SettingsRepository interface {
	FindByResource(ctx context.Context, resourceID uuid.UUID) (*ScheduleSettings, error)
	Upsert(ctx context.Context, s *ScheduleSettings) error
}

// ResourceOwnershipChecker reuses the catalog module's resource -> business
// -> owner chain instead of duplicating authorization logic here.
type ResourceOwnershipChecker interface {
	CheckResourceOwnership(ctx context.Context, resourceID, userID uuid.UUID, isSuperadmin bool) error
}

type ResourceReader interface {
	GetResource(ctx context.Context, id uuid.UUID) (*catalog.Resource, error)
}

type ServiceReader interface {
	GetService(ctx context.Context, id uuid.UUID) (*catalog.Service, error)
}

type Schedule struct {
	weeklyHours WeeklyHoursRepository
	exceptions  ExceptionRepository
	settings    SettingsRepository
	ownership   ResourceOwnershipChecker
	resources   ResourceReader
	services    ServiceReader
}

func NewSchedule(
	weeklyHours WeeklyHoursRepository,
	exceptions ExceptionRepository,
	settings SettingsRepository,
	ownership ResourceOwnershipChecker,
	resources ResourceReader,
	services ServiceReader,
) *Schedule {
	return &Schedule{
		weeklyHours: weeklyHours,
		exceptions:  exceptions,
		settings:    settings,
		ownership:   ownership,
		resources:   resources,
		services:    services,
	}
}

type WeeklyHoursInput struct {
	Weekday   int
	IsClosed  bool
	OpenTime  string
	CloseTime string
}

// ReplaceWeeklyHours overwrites the full week in one call — callers must
// supply exactly one entry per weekday (0=Sunday..6=Saturday).
func (s *Schedule) ReplaceWeeklyHours(ctx context.Context, resourceID, userID uuid.UUID, isSuperadmin bool, days []WeeklyHoursInput) ([]WeeklyHours, error) {
	if err := s.ownership.CheckResourceOwnership(ctx, resourceID, userID, isSuperadmin); err != nil {
		return nil, mapCatalogErr(err)
	}

	seen := make(map[int]bool, 7)
	rows := make([]WeeklyHours, 0, len(days))
	for _, d := range days {
		if d.Weekday < 0 || d.Weekday > 6 || seen[d.Weekday] {
			return nil, ErrInvalidWeekdays
		}
		seen[d.Weekday] = true

		row := WeeklyHours{ResourceID: resourceID, Weekday: d.Weekday, IsClosed: d.IsClosed}
		if !d.IsClosed {
			if _, _, err := localWindow(time.Now(), time.UTC, d.OpenTime, d.CloseTime); err != nil {
				return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
			}
			row.OpenTime = d.OpenTime
			row.CloseTime = d.CloseTime
		}
		rows = append(rows, row)
	}
	if len(seen) != 7 {
		return nil, ErrInvalidWeekdays
	}

	if err := s.weeklyHours.ReplaceForResource(ctx, resourceID, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *Schedule) GetWeeklyHours(ctx context.Context, resourceID uuid.UUID) ([]WeeklyHours, error) {
	resource, err := s.resources.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, ErrResourceNotFound
	}
	return s.weeklyHours.ListByResource(ctx, resourceID)
}

type ExceptionInput struct {
	Date      time.Time
	IsClosed  bool
	OpenTime  string
	CloseTime string
	Reason    string
}

func (s *Schedule) CreateException(ctx context.Context, resourceID, userID uuid.UUID, isSuperadmin bool, in ExceptionInput) (*ScheduleException, error) {
	if err := s.ownership.CheckResourceOwnership(ctx, resourceID, userID, isSuperadmin); err != nil {
		return nil, mapCatalogErr(err)
	}

	e := &ScheduleException{
		ResourceID: resourceID,
		Date:       in.Date,
		IsClosed:   in.IsClosed,
		Reason:     in.Reason,
	}
	if !in.IsClosed {
		if _, _, err := localWindow(time.Now(), time.UTC, in.OpenTime, in.CloseTime); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		e.OpenTime = &in.OpenTime
		e.CloseTime = &in.CloseTime
	}

	if err := s.exceptions.Create(ctx, e); err != nil {
		return nil, err
	}
	return e, nil
}

func (s *Schedule) ListExceptions(ctx context.Context, resourceID uuid.UUID) ([]ScheduleException, error) {
	resource, err := s.resources.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, ErrResourceNotFound
	}
	return s.exceptions.ListByResource(ctx, resourceID)
}

func (s *Schedule) DeleteException(ctx context.Context, exceptionID, userID uuid.UUID, isSuperadmin bool) error {
	e, err := s.exceptions.FindByID(ctx, exceptionID)
	if err != nil {
		return err
	}
	if e == nil {
		return ErrExceptionNotFound
	}
	if err := s.ownership.CheckResourceOwnership(ctx, e.ResourceID, userID, isSuperadmin); err != nil {
		return mapCatalogErr(err)
	}
	return s.exceptions.Delete(ctx, exceptionID)
}

func (s *Schedule) GetSettings(ctx context.Context, resourceID uuid.UUID) (*ScheduleSettings, error) {
	resource, err := s.resources.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, ErrResourceNotFound
	}

	settings, err := s.settings.FindByResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		settings = &ScheduleSettings{ResourceID: resourceID, Timezone: DefaultTimezone, BufferMinutes: 0}
	}
	return settings, nil
}

type SettingsInput struct {
	Timezone      string
	BufferMinutes int
}

func (s *Schedule) UpsertSettings(ctx context.Context, resourceID, userID uuid.UUID, isSuperadmin bool, in SettingsInput) (*ScheduleSettings, error) {
	if err := s.ownership.CheckResourceOwnership(ctx, resourceID, userID, isSuperadmin); err != nil {
		return nil, mapCatalogErr(err)
	}
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return nil, fmt.Errorf("%w: invalid timezone %q", ErrInvalidInput, in.Timezone)
	}

	settings := &ScheduleSettings{ResourceID: resourceID, Timezone: in.Timezone, BufferMinutes: in.BufferMinutes}
	if err := s.settings.Upsert(ctx, settings); err != nil {
		return nil, err
	}
	return settings, nil
}

type AvailabilityResult struct {
	ResourceID uuid.UUID
	ServiceID  uuid.UUID
	Date       time.Time
	Timezone   string
	Slots      []TimeSlot
}

// GetAvailability computes bookable slots for a resource+service on a given
// calendar date, from operating hours, exceptions, and the service's
// duration/buffer. It does not yet exclude already-booked times — there is
// no booking module yet — so results reflect capacity, not real-time
// occupancy.
func (s *Schedule) GetAvailability(ctx context.Context, resourceID, serviceID uuid.UUID, date time.Time) (*AvailabilityResult, error) {
	resource, err := s.resources.GetResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	if resource == nil {
		return nil, ErrResourceNotFound
	}

	service, err := s.services.GetService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if service == nil || service.ResourceID != resourceID {
		return nil, ErrServiceNotFound
	}

	settings, err := s.settings.FindByResource(ctx, resourceID)
	if err != nil {
		return nil, err
	}
	tzName := DefaultTimezone
	bufferMinutes := 0
	if settings != nil {
		tzName = settings.Timezone
		bufferMinutes = settings.BufferMinutes
	}

	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return nil, err
	}
	localDate := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc)

	openTime, closeTime, closed, err := s.resolveHours(ctx, resourceID, localDate)
	if err != nil {
		return nil, err
	}
	if closed {
		return &AvailabilityResult{ResourceID: resourceID, ServiceID: serviceID, Date: localDate, Timezone: tzName}, nil
	}

	openAt, closeAt, err := localWindow(localDate, loc, openTime, closeTime)
	if err != nil {
		return nil, err
	}

	slots := computeSlots(openAt, closeAt, time.Duration(service.DurationMinutes)*time.Minute, time.Duration(bufferMinutes)*time.Minute)
	return &AvailabilityResult{ResourceID: resourceID, ServiceID: serviceID, Date: localDate, Timezone: tzName, Slots: slots}, nil
}

// IsWithinOperatingHours reports whether [startAt, endAt) (UTC instants)
// falls entirely inside the resource's operating hours for that date,
// after applying exceptions. Used by the booking module to validate a
// requested time before attempting to reserve it.
func (s *Schedule) IsWithinOperatingHours(ctx context.Context, resourceID uuid.UUID, startAt, endAt time.Time) (bool, error) {
	tzName := DefaultTimezone
	settings, err := s.settings.FindByResource(ctx, resourceID)
	if err != nil {
		return false, err
	}
	if settings != nil {
		tzName = settings.Timezone
	}

	loc, err := time.LoadLocation(tzName)
	if err != nil {
		return false, err
	}
	localStart := startAt.In(loc)
	localDate := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, loc)

	openTime, closeTime, closed, err := s.resolveHours(ctx, resourceID, localDate)
	if err != nil {
		return false, err
	}
	if closed {
		return false, nil
	}

	openAt, closeAt, err := localWindow(localDate, loc, openTime, closeTime)
	if err != nil {
		return false, err
	}
	return !startAt.Before(openAt) && !endAt.After(closeAt), nil
}

// resolveHours applies: exception (closed or override) > weekly hours > closed.
func (s *Schedule) resolveHours(ctx context.Context, resourceID uuid.UUID, localDate time.Time) (openTime, closeTime string, closed bool, err error) {
	exception, err := s.exceptions.FindByResourceAndDate(ctx, resourceID, localDate)
	if err != nil {
		return "", "", false, err
	}
	if exception != nil {
		if exception.IsClosed {
			return "", "", true, nil
		}
		if exception.OpenTime != nil && exception.CloseTime != nil {
			return *exception.OpenTime, *exception.CloseTime, false, nil
		}
	}

	weekday := int(localDate.Weekday())
	wh, err := s.weeklyHours.FindByResourceAndWeekday(ctx, resourceID, weekday)
	if err != nil {
		return "", "", false, err
	}
	if wh == nil || wh.IsClosed {
		return "", "", true, nil
	}
	return wh.OpenTime, wh.CloseTime, false, nil
}

func mapCatalogErr(err error) error {
	if errors.Is(err, catalog.ErrResourceNotFound) {
		return ErrResourceNotFound
	}
	return err
}
