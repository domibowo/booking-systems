package booking

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/domibowo/booking-systems/internal/auth"
	"github.com/domibowo/booking-systems/internal/platform/httpserver"
)

const dateLayout = "2006-01-02"

type Handler struct {
	bookings *Bookings
}

func NewHandler(bookings *Bookings) *Handler {
	return &Handler{bookings: bookings}
}

// RegisterRoutes wires the booking-aware availability check as public, and
// everything else behind the given auth middleware — confirm/complete/
// no-show/the owner's list additionally require the owner role; cancel and
// the customer-facing endpoints only require being logged in, since who may
// act is data-dependent (customer-owns-the-booking vs business-owner) and
// gets checked inside the service layer.
func (h *Handler) RegisterRoutes(r *gin.Engine, requireAuth, requireOwnerRole gin.HandlerFunc) {
	r.GET("/resources/:resourceId/bookings/availability", h.GetAvailability)

	authed := r.Group("")
	authed.Use(requireAuth)
	authed.POST("/bookings", h.Create)
	authed.GET("/bookings/me", h.ListMine)
	authed.GET("/bookings/:bookingId", h.Get)
	authed.POST("/bookings/:bookingId/cancel", h.Cancel)

	owned := r.Group("")
	owned.Use(requireAuth, requireOwnerRole)
	owned.GET("/businesses/:businessId/bookings", h.ListForBusiness)
	owned.POST("/bookings/:bookingId/confirm", h.Confirm)
	owned.POST("/bookings/:bookingId/complete", h.Complete)
	owned.POST("/bookings/:bookingId/no-show", h.MarkNoShow)
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

func handleBookingError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrResourceNotFound), errors.Is(err, ErrServiceNotFound), errors.Is(err, ErrBookingNotFound):
		httpserver.JSON(c, http.StatusNotFound, err.Error(), nil)
	case errors.Is(err, ErrForbidden):
		httpserver.JSON(c, http.StatusForbidden, "you do not have access to this booking", nil)
	case errors.Is(err, ErrSlotInPast), errors.Is(err, ErrSlotNotAvailable), errors.Is(err, ErrInvalidTransition):
		httpserver.JSON(c, http.StatusBadRequest, err.Error(), nil)
	case errors.Is(err, ErrSlotConflict):
		httpserver.JSON(c, http.StatusConflict, err.Error(), nil)
	default:
		httpserver.JSON(c, http.StatusInternalServerError, "internal error", nil)
	}
}

// --- create / get / list ---

type createRequest struct {
	ResourceID string `json:"resource_id" binding:"required" example:"a8ba1de4-a497-42d1-8d70-63fd323505d5"`
	ServiceID  string `json:"service_id" binding:"required" example:"f22b9716-dd01-448f-8419-6c7051d431f5"`
	StartAt    string `json:"start_at" binding:"required" example:"2026-11-16T09:00:00Z"`
	Notes      string `json:"notes" example:"Tolong siapkan 2 raket"`
}

type bookingResponse struct {
	ID            string  `json:"id"`
	BusinessID    string  `json:"business_id"`
	ResourceID    string  `json:"resource_id"`
	ServiceID     string  `json:"service_id"`
	CustomerID    string  `json:"customer_id"`
	Status        string  `json:"status" example:"pending"`
	StartAt       string  `json:"start_at"`
	EndAt         string  `json:"end_at"`
	HoldExpiresAt *string `json:"hold_expires_at,omitempty"`
	Notes         string  `json:"notes"`
	CreatedAt     string  `json:"created_at"`
}

type bookingListResponse struct {
	Items    []bookingResponse `json:"items"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int64             `json:"total"`
}

func toBookingResponse(b *Booking) bookingResponse {
	resp := bookingResponse{
		ID:         b.ID.String(),
		BusinessID: b.BusinessID.String(),
		ResourceID: b.ResourceID.String(),
		ServiceID:  b.ServiceID.String(),
		CustomerID: b.CustomerID.String(),
		Status:     string(b.EffectiveStatus(time.Now())),
		StartAt:    b.StartAt.Format(time.RFC3339),
		EndAt:      b.EndAt.Format(time.RFC3339),
		Notes:      b.Notes,
		CreatedAt:  b.CreatedAt.Format(time.RFC3339),
	}
	if b.HoldExpiresAt != nil {
		s := b.HoldExpiresAt.Format(time.RFC3339)
		resp.HoldExpiresAt = &s
	}
	return resp
}

// Create godoc
// @Summary Create a booking (places a temporary hold pending payment)
// @Description Send an Idempotency-Key header to safely retry: a repeated request with the same key for the same customer returns the original booking instead of creating a duplicate.
// @Tags bookings
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param Idempotency-Key header string false "Client-generated key to dedupe retried requests"
// @Param request body createRequest true "Booking details"
// @Success 201 {object} httpserver.Response{data=bookingResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Failure 409 {object} httpserver.Response{data=object} "requested time overlaps an existing booking"
// @Router /bookings [post]
func (h *Handler) Create(c *gin.Context) {
	var req createRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httpserver.JSON(c, http.StatusBadRequest, err.Error(), nil)
		return
	}
	resourceID, err := uuid.Parse(req.ResourceID)
	if err != nil {
		httpserver.JSON(c, http.StatusBadRequest, "invalid resource_id", nil)
		return
	}
	serviceID, err := uuid.Parse(req.ServiceID)
	if err != nil {
		httpserver.JSON(c, http.StatusBadRequest, "invalid service_id", nil)
		return
	}
	startAt, err := time.Parse(time.RFC3339, req.StartAt)
	if err != nil {
		httpserver.JSON(c, http.StatusBadRequest, "invalid start_at, expected RFC3339", nil)
		return
	}

	var idempotencyKey *string
	if key := c.GetHeader("Idempotency-Key"); key != "" {
		idempotencyKey = &key
	}

	b, err := h.bookings.Create(c.Request.Context(), currentUserID(c), CreateInput{
		ResourceID: resourceID, ServiceID: serviceID, StartAt: startAt, Notes: req.Notes, IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		handleBookingError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusCreated, "booking created successfully", toBookingResponse(b))
}

// Get godoc
// @Summary Get a booking by ID
// @Description Visible to the booking's own customer or the resource's business owner/superadmin.
// @Tags bookings
// @Security BearerAuth
// @Produce json
// @Param bookingId path string true "Booking ID"
// @Success 200 {object} httpserver.Response{data=bookingResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /bookings/{bookingId} [get]
func (h *Handler) Get(c *gin.Context) {
	id, ok := parseUUIDParam(c, "bookingId")
	if !ok {
		return
	}
	b, err := h.bookings.Get(c.Request.Context(), id, currentUserID(c), isSuperadmin(c))
	if err != nil {
		handleBookingError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusOK, "booking retrieved successfully", toBookingResponse(b))
}

// ListMine godoc
// @Summary List the authenticated customer's own booking history
// @Tags bookings
// @Security BearerAuth
// @Produce json
// @Param status query string false "Filter by status" Enums(pending, confirmed, cancelled, completed, expired, no_show)
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page (max 100)" default(20)
// @Success 200 {object} httpserver.Response{data=bookingListResponse}
// @Failure 401 {object} httpserver.Response{data=object}
// @Router /bookings/me [get]
func (h *Handler) ListMine(c *gin.Context) {
	var status *Status
	if s := c.Query("status"); s != "" {
		st := Status(s)
		status = &st
	}
	p := httpserver.ParsePagination(c)
	items, total, err := h.bookings.ListMine(c.Request.Context(), currentUserID(c), status, p.Limit(), p.Offset())
	if err != nil {
		handleBookingError(c, err)
		return
	}
	data := make([]bookingResponse, 0, len(items))
	for i := range items {
		data = append(data, toBookingResponse(&items[i]))
	}
	httpserver.JSON(c, http.StatusOK, "bookings retrieved successfully", bookingListResponse{
		Items: data, Page: p.Page, PageSize: p.PageSize, Total: total,
	})
}

// ListForBusiness godoc
// @Summary List a business's bookings, with optional resource/status/date filters
// @Description Owner-only. Requires ownership of the business.
// @Tags bookings
// @Security BearerAuth
// @Produce json
// @Param businessId path string true "Business ID"
// @Param resource_id query string false "Filter by resource ID"
// @Param status query string false "Filter by status" Enums(pending, confirmed, cancelled, completed, expired, no_show)
// @Param from query string false "Start date filter (YYYY-MM-DD)"
// @Param to query string false "End date filter (YYYY-MM-DD, exclusive)"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page (max 100)" default(20)
// @Success 200 {object} httpserver.Response{data=bookingListResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Router /businesses/{businessId}/bookings [get]
func (h *Handler) ListForBusiness(c *gin.Context) {
	businessID, ok := parseUUIDParam(c, "businessId")
	if !ok {
		return
	}

	var filter ListFilter
	if rid := c.Query("resource_id"); rid != "" {
		id, err := uuid.Parse(rid)
		if err != nil {
			httpserver.JSON(c, http.StatusBadRequest, "invalid resource_id", nil)
			return
		}
		filter.ResourceID = &id
	}
	if s := c.Query("status"); s != "" {
		st := Status(s)
		filter.Status = &st
	}
	if from := c.Query("from"); from != "" {
		t, err := time.Parse(dateLayout, from)
		if err != nil {
			httpserver.JSON(c, http.StatusBadRequest, "invalid from, expected YYYY-MM-DD", nil)
			return
		}
		filter.From = &t
	}
	if to := c.Query("to"); to != "" {
		t, err := time.Parse(dateLayout, to)
		if err != nil {
			httpserver.JSON(c, http.StatusBadRequest, "invalid to, expected YYYY-MM-DD", nil)
			return
		}
		filter.To = &t
	}

	p := httpserver.ParsePagination(c)
	items, total, err := h.bookings.ListForBusiness(c.Request.Context(), businessID, currentUserID(c), isSuperadmin(c), filter, p.Limit(), p.Offset())
	if err != nil {
		handleBookingError(c, err)
		return
	}
	data := make([]bookingResponse, 0, len(items))
	for i := range items {
		data = append(data, toBookingResponse(&items[i]))
	}
	httpserver.JSON(c, http.StatusOK, "bookings retrieved successfully", bookingListResponse{
		Items: data, Page: p.Page, PageSize: p.PageSize, Total: total,
	})
}

// --- status transitions ---

// Confirm godoc
// @Summary Confirm a pending booking
// @Description Owner-only. Stands in for what a payment webhook will trigger once the payment module exists.
// @Tags bookings
// @Security BearerAuth
// @Produce json
// @Param bookingId path string true "Booking ID"
// @Success 200 {object} httpserver.Response{data=bookingResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /bookings/{bookingId}/confirm [post]
func (h *Handler) Confirm(c *gin.Context) {
	id, ok := parseUUIDParam(c, "bookingId")
	if !ok {
		return
	}
	b, err := h.bookings.Confirm(c.Request.Context(), id, currentUserID(c), isSuperadmin(c))
	if err != nil {
		handleBookingError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusOK, "booking confirmed successfully", toBookingResponse(b))
}

// Cancel godoc
// @Summary Cancel a pending or confirmed booking
// @Description Allowed for the booking's own customer or the resource's business owner/superadmin.
// @Tags bookings
// @Security BearerAuth
// @Produce json
// @Param bookingId path string true "Booking ID"
// @Success 200 {object} httpserver.Response{data=bookingResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /bookings/{bookingId}/cancel [post]
func (h *Handler) Cancel(c *gin.Context) {
	id, ok := parseUUIDParam(c, "bookingId")
	if !ok {
		return
	}
	b, err := h.bookings.Cancel(c.Request.Context(), id, currentUserID(c), isSuperadmin(c))
	if err != nil {
		handleBookingError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusOK, "booking cancelled successfully", toBookingResponse(b))
}

// Complete godoc
// @Summary Mark a confirmed booking as completed
// @Description Owner-only.
// @Tags bookings
// @Security BearerAuth
// @Produce json
// @Param bookingId path string true "Booking ID"
// @Success 200 {object} httpserver.Response{data=bookingResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /bookings/{bookingId}/complete [post]
func (h *Handler) Complete(c *gin.Context) {
	id, ok := parseUUIDParam(c, "bookingId")
	if !ok {
		return
	}
	b, err := h.bookings.Complete(c.Request.Context(), id, currentUserID(c), isSuperadmin(c))
	if err != nil {
		handleBookingError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusOK, "booking marked as completed", toBookingResponse(b))
}

// MarkNoShow godoc
// @Summary Mark a confirmed booking as a no-show
// @Description Owner-only.
// @Tags bookings
// @Security BearerAuth
// @Produce json
// @Param bookingId path string true "Booking ID"
// @Success 200 {object} httpserver.Response{data=bookingResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 401 {object} httpserver.Response{data=object}
// @Failure 403 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /bookings/{bookingId}/no-show [post]
func (h *Handler) MarkNoShow(c *gin.Context) {
	id, ok := parseUUIDParam(c, "bookingId")
	if !ok {
		return
	}
	b, err := h.bookings.MarkNoShow(c.Request.Context(), id, currentUserID(c), isSuperadmin(c))
	if err != nil {
		handleBookingError(c, err)
		return
	}
	httpserver.JSON(c, http.StatusOK, "booking marked as no-show", toBookingResponse(b))
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
// @Summary Check real-time available booking slots for a resource+service on a date
// @Description Unlike /resources/{resourceId}/availability (schedule module), this excludes times already held or booked.
// @Tags bookings
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Param service_id query string true "Service ID"
// @Param date query string true "Date (YYYY-MM-DD)"
// @Success 200 {object} httpserver.Response{data=availabilityResponse}
// @Failure 400 {object} httpserver.Response{data=object}
// @Failure 404 {object} httpserver.Response{data=object}
// @Router /resources/{resourceId}/bookings/availability [get]
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

	result, err := h.bookings.GetAvailableSlots(c.Request.Context(), resourceID, serviceID, date)
	if err != nil {
		handleBookingError(c, err)
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
