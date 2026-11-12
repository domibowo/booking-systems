package schedule

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/domibowo/booking-systems/internal/auth"
	"github.com/domibowo/booking-systems/internal/catalog"
	"github.com/domibowo/booking-systems/internal/platform/httpserver"
)

const dateLayout = "2006-01-02"

type Handler struct {
	schedule *Schedule
}

func NewHandler(schedule *Schedule) *Handler {
	return &Handler{schedule: schedule}
}

func (h *Handler) RegisterRoutes(r *gin.Engine, requireAuth, requireOwnerRole gin.HandlerFunc) {
	r.GET("/resources/:resourceId/schedule/weekly-hours", h.GetWeeklyHours)
	r.GET("/resources/:resourceId/schedule/exceptions", h.ListExceptions)
	r.GET("/resources/:resourceId/schedule/settings", h.GetSettings)
	r.GET("/resources/:resourceId/availability", h.GetAvailability)

	authed := r.Group("")
	authed.Use(requireAuth, requireOwnerRole)
	authed.PUT("/resources/:resourceId/schedule/weekly-hours", h.ReplaceWeeklyHours)
	authed.POST("/resources/:resourceId/schedule/exceptions", h.CreateException)
	authed.DELETE("/schedule/exceptions/:exceptionId", h.DeleteException)
	authed.PUT("/resources/:resourceId/schedule/settings", h.UpsertSettings)
}

func currentUserID(c *gin.Context) uuid.UUID {
	v, _ := c.Get(auth.ContextUserIDKey)
	id, _ := v.(uuid.UUID)
	return id
}

func isSuperadmin(c *gin.Context) bool {
	v, _ := c.Get(auth.ContextUserRoleKey)
	role, _ := v.(auth.Role)
	return role == auth.RoleSuperadmin
}

func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		httpserver.JSON(c, http.StatusBadRequest, "invalid "+name, nil)
		return uuid.Nil, false
	}
	return id, true
}

func handleScheduleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrResourceNotFound), errors.Is(err, ErrServiceNotFound), errors.Is(err, ErrExceptionNotFound):
		httpserver.JSON(c, http.StatusNotFound, err.Error(), nil)
	case errors.Is(err, catalog.ErrForbidden):
		httpserver.JSON(c, http.StatusForbidden, "you do not own this business", nil)
	case errors.Is(err, ErrInvalidWeekdays), errors.Is(err, ErrInvalidInput):
		httpserver.JSON(c, http.StatusBadRequest, err.Error(), nil)
	default:
		httpserver.JSON(c, http.StatusInternalServerError, "internal error", nil)
	}
}

// --- weekly hours ---

type dayInput struct {
	Weekday   int    `json:"weekday" binding:"min=0,max=6" example:"1"`
	IsClosed  bool   `json:"is_closed"`
	OpenTime  string `json:"open_time" example:"09:00"`
	CloseTime string `json:"close_time" example:"17:00"`
}

type weeklyHoursRequest struct {
	Days []dayInput `json:"days" binding:"required,len=7"`
}

type weeklyHoursResponse struct {
	Weekday   int    `json:"weekday"`
	IsClosed  bool   `json:"is_closed"`
	OpenTime  string `json:"open_time"`
	CloseTime string `json:"close_time"`
}

type weeklyHoursListResponse struct {
	Items []weeklyHoursResponse `json:"items"`
}

func toWeeklyHoursResponse(w *WeeklyHours) weeklyHoursResponse {
	return weeklyHoursResponse{Weekday: w.Weekday, IsClosed: w.IsClosed, OpenTime: w.OpenTime, CloseTime: w.CloseTime}
}

// ReplaceWeeklyHours godoc
// @Summary Replace a resource's weekly operating hours
// @Description Requires exactly one entry per weekday (0=Sunday..6=Saturday). Replaces the full week atomically.
// @Tags schedule
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Param request body weeklyHoursRequest true "Full week of operating hours"
// @Success 200 {object} httpserver.Response{data=weeklyHoursListResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /resources/{resourceId}/schedule/weekly-hours [put]
func (h *Handler) ReplaceWeeklyHours(c *gin.Context) {
	resourceID, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	var req weeklyHoursRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpserver.JSON(c, http.StatusBadRequest, err.Error(), nil)
		return
	}

	days := make([]WeeklyHoursInput, 0, len(req.Days))
	for _, d := range req.Days {
		days = append(days, WeeklyHoursInput{Weekday: d.Weekday, IsClosed: d.IsClosed, OpenTime: d.OpenTime, CloseTime: d.CloseTime})
	}

	rows, err := h.schedule.ReplaceWeeklyHours(c.Request.Context(), resourceID, currentUserID(c), isSuperadmin(c), days)
	if err != nil {
		handleScheduleError(c, err)
		return
	}

	data := make([]weeklyHoursResponse, 0, len(rows))
	for i := range rows {
		data = append(data, toWeeklyHoursResponse(&rows[i]))
	}
	httpserver.JSON(c, http.StatusOK, "weekly hours updated successfully", weeklyHoursListResponse{Items: data})
}

// GetWeeklyHours godoc
// @Summary Get a resource's weekly operating hours
// @Tags schedule
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Success 200 {object} httpserver.Response{data=weeklyHoursListResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /resources/{resourceId}/schedule/weekly-hours [get]
func (h *Handler) GetWeeklyHours(c *gin.Context) {
	resourceID, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	rows, err := h.schedule.GetWeeklyHours(c.Request.Context(), resourceID)
	if err != nil {
		handleScheduleError(c, err)
		return
	}
	data := make([]weeklyHoursResponse, 0, len(rows))
	for i := range rows {
		data = append(data, toWeeklyHoursResponse(&rows[i]))
	}
	httpserver.JSON(c, http.StatusOK, "weekly hours retrieved successfully", weeklyHoursListResponse{Items: data})
}

// --- exceptions ---

type exceptionRequest struct {
	Date      string `json:"date" binding:"required" example:"2026-12-25"`
	IsClosed  bool   `json:"is_closed" example:"true"`
	OpenTime  string `json:"open_time" example:"09:00"`
	CloseTime string `json:"close_time" example:"13:00"`
	Reason    string `json:"reason" example:"Libur Nasional"`
}

type exceptionResponse struct {
	ID        string `json:"id"`
	Date      string `json:"date"`
	IsClosed  bool   `json:"is_closed"`
	OpenTime  string `json:"open_time,omitempty"`
	CloseTime string `json:"close_time,omitempty"`
	Reason    string `json:"reason"`
}

type exceptionListResponse struct {
	Items []exceptionResponse `json:"items"`
}

func toExceptionResponse(e *ScheduleException) exceptionResponse {
	resp := exceptionResponse{
		ID:       e.ID.String(),
		Date:     e.Date.Format(dateLayout),
		IsClosed: e.IsClosed,
		Reason:   e.Reason,
	}
	if e.OpenTime != nil {
		resp.OpenTime = *e.OpenTime
	}
	if e.CloseTime != nil {
		resp.CloseTime = *e.CloseTime
	}
	return resp
}

// CreateException godoc
// @Summary Add a schedule exception (holiday, closure, or shortened hours) for a specific date
// @Tags schedule
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Param request body exceptionRequest true "Exception details"
// @Success 201 {object} httpserver.Response{data=exceptionResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /resources/{resourceId}/schedule/exceptions [post]
func (h *Handler) CreateException(c *gin.Context) {
	resourceID, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	var req exceptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpserver.JSON(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	date, err := time.Parse(dateLayout, req.Date)
	if err != nil {
		httpserver.JSON(c, http.StatusBadRequest, "invalid date, expected YYYY-MM-DD", nil)
		return
	}

	e, err := h.schedule.CreateException(c.Request.Context(), resourceID, currentUserID(c), isSuperadmin(c), ExceptionInput{
		Date: date, IsClosed: req.IsClosed, OpenTime: req.OpenTime, CloseTime: req.CloseTime, Reason: req.Reason,
	})
	if err != nil {
		handleScheduleError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusCreated, "exception created successfully", toExceptionResponse(e))
}

// ListExceptions godoc
// @Summary List a resource's schedule exceptions
// @Tags schedule
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Success 200 {object} httpserver.Response{data=exceptionListResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /resources/{resourceId}/schedule/exceptions [get]
func (h *Handler) ListExceptions(c *gin.Context) {
	resourceID, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	items, err := h.schedule.ListExceptions(c.Request.Context(), resourceID)
	if err != nil {
		handleScheduleError(c, err)
		return
	}
	data := make([]exceptionResponse, 0, len(items))
	for i := range items {
		data = append(data, toExceptionResponse(&items[i]))
	}
	httpserver.JSON(c, http.StatusOK, "exceptions retrieved successfully", exceptionListResponse{Items: data})
}

// DeleteException godoc
// @Summary Delete a schedule exception
// @Tags schedule
// @Security BearerAuth
// @Produce json
// @Param exceptionId path string true "Exception ID"
// @Success 200 {object} httpserver.Response{data=object}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /schedule/exceptions/{exceptionId} [delete]
func (h *Handler) DeleteException(c *gin.Context) {
	id, ok := parseUUIDParam(c, "exceptionId")
	if !ok {
		return
	}
	if err := h.schedule.DeleteException(c.Request.Context(), id, currentUserID(c), isSuperadmin(c)); err != nil {
		handleScheduleError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusOK, "exception deleted successfully", nil)
}

// --- settings ---

type settingsRequest struct {
	Timezone      string `json:"timezone" binding:"required" example:"Asia/Jakarta"`
	BufferMinutes int    `json:"buffer_minutes" binding:"min=0" example:"15"`
}

type settingsResponse struct {
	ResourceID    string `json:"resource_id"`
	Timezone      string `json:"timezone"`
	BufferMinutes int    `json:"buffer_minutes"`
}

func toSettingsResponse(s *ScheduleSettings) settingsResponse {
	return settingsResponse{ResourceID: s.ResourceID.String(), Timezone: s.Timezone, BufferMinutes: s.BufferMinutes}
}

// UpsertSettings godoc
// @Summary Set a resource's scheduling settings (timezone, buffer between bookings)
// @Tags schedule
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Param request body settingsRequest true "Settings"
// @Success 200 {object} httpserver.Response{data=settingsResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /resources/{resourceId}/schedule/settings [put]
func (h *Handler) UpsertSettings(c *gin.Context) {
	resourceID, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	var req settingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpserver.JSON(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	s, err := h.schedule.UpsertSettings(c.Request.Context(), resourceID, currentUserID(c), isSuperadmin(c), SettingsInput{
		Timezone: req.Timezone, BufferMinutes: req.BufferMinutes,
	})
	if err != nil {
		handleScheduleError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusOK, "settings updated successfully", toSettingsResponse(s))
}

// GetSettings godoc
// @Summary Get a resource's scheduling settings
// @Description Returns defaults (Asia/Jakarta, no buffer) if never configured.
// @Tags schedule
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Success 200 {object} httpserver.Response{data=settingsResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /resources/{resourceId}/schedule/settings [get]
func (h *Handler) GetSettings(c *gin.Context) {
	resourceID, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	s, err := h.schedule.GetSettings(c.Request.Context(), resourceID)
	if err != nil {
		handleScheduleError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusOK, "settings retrieved successfully", toSettingsResponse(s))
}

// --- availability ---

type slotResponse struct {
	StartAt string `json:"start_at"`
	EndAt   string `json:"end_at"`
}

type availabilityResponse struct {
	ResourceID string         `json:"resource_id"`
	ServiceID  string         `json:"service_id"`
	Date       string         `json:"date"`
	Timezone   string         `json:"timezone"`
	Slots      []slotResponse `json:"slots"`
}

// GetAvailability godoc
// @Summary Check available booking slots for a resource+service on a date
// @Description Computed from operating hours, exceptions, and the service's duration/buffer. Does not yet exclude existing bookings (no booking module yet).
// @Tags schedule
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Param service_id query string true "Service ID"
// @Param date query string true "Date (YYYY-MM-DD)"
// @Success 200 {object} httpserver.Response{data=availabilityResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /resources/{resourceId}/availability [get]
func (h *Handler) GetAvailability(c *gin.Context) {
	resourceID, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	serviceID, err := uuid.Parse(c.Query("service_id"))
	if err != nil {
		httpserver.JSON(c, http.StatusBadRequest, "invalid or missing service_id", nil)
		return
	}
	date, err := time.Parse(dateLayout, c.Query("date"))
	if err != nil {
		httpserver.JSON(c, http.StatusBadRequest, "invalid or missing date, expected YYYY-MM-DD", nil)
		return
	}

	result, err := h.schedule.GetAvailability(c.Request.Context(), resourceID, serviceID, date)
	if err != nil {
		handleScheduleError(c, err)
		return
	}

	slots := make([]slotResponse, 0, len(result.Slots))
	for _, s := range result.Slots {
		slots = append(slots, slotResponse{StartAt: s.StartAt.Format(time.RFC3339), EndAt: s.EndAt.Format(time.RFC3339)})
	}
	httpserver.JSON(c, http.StatusOK, "availability retrieved successfully", availabilityResponse{
		ResourceID: result.ResourceID.String(),
		ServiceID:  result.ServiceID.String(),
		Date:       result.Date.Format(dateLayout),
		Timezone:   result.Timezone,
		Slots:      slots,
	})
}
