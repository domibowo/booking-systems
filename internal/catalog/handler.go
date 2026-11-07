package catalog

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/domibowo/booking-systems/internal/auth"
	"github.com/domibowo/booking-systems/internal/platform/httpserver"
)

type Handler struct {
	catalog *Catalog
}

func NewHandler(catalog *Catalog) *Handler {
	return &Handler{catalog: catalog}
}

// RegisterRoutes wires read endpoints as public and mutations behind the
// given auth middleware, so this package doesn't need to import the auth
// package's handler type directly.
func (h *Handler) RegisterRoutes(r *gin.Engine, requireAuth, requireOwnerRole gin.HandlerFunc) {
	r.GET("/businesses", h.ListBusinesses)
	r.GET("/businesses/:businessId", h.GetBusiness)
	r.GET("/businesses/:businessId/resources", h.ListResources)
	r.GET("/resources/:resourceId", h.GetResource)
	r.GET("/resources/:resourceId/services", h.ListServices)
	r.GET("/services/:serviceId", h.GetService)

	authed := r.Group("")
	authed.Use(requireAuth, requireOwnerRole)
	authed.POST("/businesses", h.CreateBusiness)
	authed.PUT("/businesses/:businessId", h.UpdateBusiness)
	authed.DELETE("/businesses/:businessId", h.DeleteBusiness)
	authed.POST("/businesses/:businessId/resources", h.CreateResource)
	authed.PUT("/resources/:resourceId", h.UpdateResource)
	authed.DELETE("/resources/:resourceId", h.DeleteResource)
	authed.POST("/resources/:resourceId/services", h.CreateService)
	authed.PUT("/services/:serviceId", h.UpdateService)
	authed.DELETE("/services/:serviceId", h.DeleteService)
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
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: "invalid " + name})
		return uuid.Nil, false
	}
	return id, true
}

func handleCatalogError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrBusinessNotFound), errors.Is(err, ErrResourceNotFound), errors.Is(err, ErrServiceNotFound):
		c.JSON(http.StatusNotFound, httpserver.ErrorResponse{Error: err.Error()})
	case errors.Is(err, ErrForbidden):
		c.JSON(http.StatusForbidden, httpserver.ErrorResponse{Error: "you do not own this business"})
	default:
		c.JSON(http.StatusInternalServerError, httpserver.ErrorResponse{Error: "internal error"})
	}
}

// --- business ---

type businessRequest struct {
	Name        string `json:"name" binding:"required" example:"Lapangan Jaya"`
	Address     string `json:"address" example:"Jl. Merdeka 1"`
	Phone       string `json:"phone" example:"08123456789"`
	Description string `json:"description" example:"Lapangan futsal indoor"`
}

type businessResponse struct {
	ID          string `json:"id"`
	OwnerID     string `json:"owner_id"`
	Name        string `json:"name"`
	Address     string `json:"address"`
	Phone       string `json:"phone"`
	Description string `json:"description"`
}

type businessListResponse struct {
	Data     []businessResponse `json:"data"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int64              `json:"total"`
}

func toBusinessResponse(b *Business) businessResponse {
	return businessResponse{
		ID:          b.ID.String(),
		OwnerID:     b.OwnerID.String(),
		Name:        b.Name,
		Address:     b.Address,
		Phone:       b.Phone,
		Description: b.Description,
	}
}

// CreateBusiness godoc
// @Summary Create a business (venue)
// @Description Requires the "owner" or "superadmin" role. The authenticated user becomes the owner.
// @Tags businesses
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param request body businessRequest true "Business details"
// @Success 201 {object} businessResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse
// @Failure 403 {object} httpserver.ErrorResponse
// @Router /businesses [post]
func (h *Handler) CreateBusiness(c *gin.Context) {
	var req businessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}

	b, err := h.catalog.CreateBusiness(c.Request.Context(), currentUserID(c), CreateBusinessInput{
		Name: req.Name, Address: req.Address, Phone: req.Phone, Description: req.Description,
	})
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toBusinessResponse(b))
}

// GetBusiness godoc
// @Summary Get a business by ID
// @Tags businesses
// @Produce json
// @Param businessId path string true "Business ID"
// @Success 200 {object} businessResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /businesses/{businessId} [get]
func (h *Handler) GetBusiness(c *gin.Context) {
	id, ok := parseUUIDParam(c, "businessId")
	if !ok {
		return
	}
	b, err := h.catalog.GetBusiness(c.Request.Context(), id)
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, toBusinessResponse(b))
}

// ListBusinesses godoc
// @Summary List businesses
// @Tags businesses
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page (max 100)" default(20)
// @Success 200 {object} businessListResponse
// @Router /businesses [get]
func (h *Handler) ListBusinesses(c *gin.Context) {
	p := httpserver.ParsePagination(c)
	items, total, err := h.catalog.ListBusinesses(c.Request.Context(), p.Limit(), p.Offset())
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	data := make([]businessResponse, 0, len(items))
	for i := range items {
		data = append(data, toBusinessResponse(&items[i]))
	}
	c.JSON(http.StatusOK, businessListResponse{Data: data, Page: p.Page, PageSize: p.PageSize, Total: total})
}

// UpdateBusiness godoc
// @Summary Update a business
// @Description Only the business owner or a superadmin may update it.
// @Tags businesses
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param businessId path string true "Business ID"
// @Param request body businessRequest true "Business details"
// @Success 200 {object} businessResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse
// @Failure 403 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /businesses/{businessId} [put]
func (h *Handler) UpdateBusiness(c *gin.Context) {
	id, ok := parseUUIDParam(c, "businessId")
	if !ok {
		return
	}
	var req businessRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}
	b, err := h.catalog.UpdateBusiness(c.Request.Context(), id, currentUserID(c), isSuperadmin(c), UpdateBusinessInput{
		Name: req.Name, Address: req.Address, Phone: req.Phone, Description: req.Description,
	})
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, toBusinessResponse(b))
}

// DeleteBusiness godoc
// @Summary Delete a business
// @Description Cascades to delete its resources and services. Only the owner or a superadmin may delete it.
// @Tags businesses
// @Security BearerAuth
// @Param businessId path string true "Business ID"
// @Success 204 "no content"
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse
// @Failure 403 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /businesses/{businessId} [delete]
func (h *Handler) DeleteBusiness(c *gin.Context) {
	id, ok := parseUUIDParam(c, "businessId")
	if !ok {
		return
	}
	if err := h.catalog.DeleteBusiness(c.Request.Context(), id, currentUserID(c), isSuperadmin(c)); err != nil {
		handleCatalogError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// --- resource ---

type resourceRequest struct {
	Name        string `json:"name" binding:"required" example:"Lapangan A"`
	Type        string `json:"type" binding:"required" example:"lapangan"`
	Description string `json:"description" example:"Lapangan indoor dengan lantai vinyl"`
}

type resourceResponse struct {
	ID          string `json:"id"`
	BusinessID  string `json:"business_id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

type resourceListResponse struct {
	Data     []resourceResponse `json:"data"`
	Page     int                `json:"page"`
	PageSize int                `json:"page_size"`
	Total    int64              `json:"total"`
}

func toResourceResponse(r *Resource) resourceResponse {
	return resourceResponse{
		ID:          r.ID.String(),
		BusinessID:  r.BusinessID.String(),
		Name:        r.Name,
		Type:        r.Type,
		Description: r.Description,
	}
}

// CreateResource godoc
// @Summary Create a resource under a business
// @Description Requires ownership of the parent business (or superadmin).
// @Tags resources
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param businessId path string true "Business ID"
// @Param request body resourceRequest true "Resource details"
// @Success 201 {object} resourceResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse
// @Failure 403 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /businesses/{businessId}/resources [post]
func (h *Handler) CreateResource(c *gin.Context) {
	businessID, ok := parseUUIDParam(c, "businessId")
	if !ok {
		return
	}
	var req resourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}
	r, err := h.catalog.CreateResource(c.Request.Context(), businessID, currentUserID(c), isSuperadmin(c), CreateResourceInput{
		Name: req.Name, Type: req.Type, Description: req.Description,
	})
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toResourceResponse(r))
}

// GetResource godoc
// @Summary Get a resource by ID
// @Tags resources
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Success 200 {object} resourceResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /resources/{resourceId} [get]
func (h *Handler) GetResource(c *gin.Context) {
	id, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	r, err := h.catalog.GetResource(c.Request.Context(), id)
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResourceResponse(r))
}

// ListResources godoc
// @Summary List resources under a business
// @Tags resources
// @Produce json
// @Param businessId path string true "Business ID"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page (max 100)" default(20)
// @Success 200 {object} resourceListResponse
// @Router /businesses/{businessId}/resources [get]
func (h *Handler) ListResources(c *gin.Context) {
	businessID, ok := parseUUIDParam(c, "businessId")
	if !ok {
		return
	}
	p := httpserver.ParsePagination(c)
	items, total, err := h.catalog.ListResources(c.Request.Context(), businessID, p.Limit(), p.Offset())
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	data := make([]resourceResponse, 0, len(items))
	for i := range items {
		data = append(data, toResourceResponse(&items[i]))
	}
	c.JSON(http.StatusOK, resourceListResponse{Data: data, Page: p.Page, PageSize: p.PageSize, Total: total})
}

// UpdateResource godoc
// @Summary Update a resource
// @Description Requires ownership of the parent business (or superadmin).
// @Tags resources
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Param request body resourceRequest true "Resource details"
// @Success 200 {object} resourceResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse
// @Failure 403 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /resources/{resourceId} [put]
func (h *Handler) UpdateResource(c *gin.Context) {
	id, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	var req resourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}
	r, err := h.catalog.UpdateResource(c.Request.Context(), id, currentUserID(c), isSuperadmin(c), UpdateResourceInput{
		Name: req.Name, Type: req.Type, Description: req.Description,
	})
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, toResourceResponse(r))
}

// DeleteResource godoc
// @Summary Delete a resource
// @Description Cascades to delete its services. Requires ownership of the parent business (or superadmin).
// @Tags resources
// @Security BearerAuth
// @Param resourceId path string true "Resource ID"
// @Success 204 "no content"
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse
// @Failure 403 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /resources/{resourceId} [delete]
func (h *Handler) DeleteResource(c *gin.Context) {
	id, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	if err := h.catalog.DeleteResource(c.Request.Context(), id, currentUserID(c), isSuperadmin(c)); err != nil {
		handleCatalogError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// --- service (offering) ---

type serviceRequest struct {
	Name            string `json:"name" binding:"required" example:"Sewa 1 jam"`
	DurationMinutes int    `json:"duration_minutes" binding:"required,min=1" example:"60"`
	PriceCents      int64  `json:"price_cents" binding:"min=0" example:"15000000"`
	Currency        string `json:"currency" example:"IDR"`
	Description     string `json:"description" example:"Sewa lapangan selama 1 jam"`
}

type serviceResponse struct {
	ID              string `json:"id"`
	ResourceID      string `json:"resource_id"`
	Name            string `json:"name"`
	DurationMinutes int    `json:"duration_minutes"`
	PriceCents      int64  `json:"price_cents"`
	Currency        string `json:"currency"`
	Description     string `json:"description"`
}

type serviceListResponse struct {
	Data     []serviceResponse `json:"data"`
	Page     int               `json:"page"`
	PageSize int               `json:"page_size"`
	Total    int64             `json:"total"`
}

func toServiceResponse(s *Service) serviceResponse {
	return serviceResponse{
		ID:              s.ID.String(),
		ResourceID:      s.ResourceID.String(),
		Name:            s.Name,
		DurationMinutes: s.DurationMinutes,
		PriceCents:      s.PriceCents,
		Currency:        s.Currency,
		Description:     s.Description,
	}
}

// CreateService godoc
// @Summary Create a service (bookable offering) under a resource
// @Description Price is in integer cents to avoid floating point issues. Requires ownership of the parent business (or superadmin).
// @Tags services
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Param request body serviceRequest true "Service details"
// @Success 201 {object} serviceResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse
// @Failure 403 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /resources/{resourceId}/services [post]
func (h *Handler) CreateService(c *gin.Context) {
	resourceID, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	var req serviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}
	if req.Currency == "" {
		req.Currency = "IDR"
	}
	s, err := h.catalog.CreateService(c.Request.Context(), resourceID, currentUserID(c), isSuperadmin(c), CreateServiceInput{
		Name: req.Name, DurationMinutes: req.DurationMinutes, PriceCents: req.PriceCents,
		Currency: req.Currency, Description: req.Description,
	})
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toServiceResponse(s))
}

// GetService godoc
// @Summary Get a service by ID
// @Tags services
// @Produce json
// @Param serviceId path string true "Service ID"
// @Success 200 {object} serviceResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /services/{serviceId} [get]
func (h *Handler) GetService(c *gin.Context) {
	id, ok := parseUUIDParam(c, "serviceId")
	if !ok {
		return
	}
	s, err := h.catalog.GetService(c.Request.Context(), id)
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, toServiceResponse(s))
}

// ListServices godoc
// @Summary List services under a resource
// @Tags services
// @Produce json
// @Param resourceId path string true "Resource ID"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Items per page (max 100)" default(20)
// @Success 200 {object} serviceListResponse
// @Router /resources/{resourceId}/services [get]
func (h *Handler) ListServices(c *gin.Context) {
	resourceID, ok := parseUUIDParam(c, "resourceId")
	if !ok {
		return
	}
	p := httpserver.ParsePagination(c)
	items, total, err := h.catalog.ListServices(c.Request.Context(), resourceID, p.Limit(), p.Offset())
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	data := make([]serviceResponse, 0, len(items))
	for i := range items {
		data = append(data, toServiceResponse(&items[i]))
	}
	c.JSON(http.StatusOK, serviceListResponse{Data: data, Page: p.Page, PageSize: p.PageSize, Total: total})
}

// UpdateService godoc
// @Summary Update a service
// @Description Requires ownership of the parent business (or superadmin).
// @Tags services
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param serviceId path string true "Service ID"
// @Param request body serviceRequest true "Service details"
// @Success 200 {object} serviceResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse
// @Failure 403 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /services/{serviceId} [put]
func (h *Handler) UpdateService(c *gin.Context) {
	id, ok := parseUUIDParam(c, "serviceId")
	if !ok {
		return
	}
	var req serviceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}
	if req.Currency == "" {
		req.Currency = "IDR"
	}
	s, err := h.catalog.UpdateService(c.Request.Context(), id, currentUserID(c), isSuperadmin(c), UpdateServiceInput{
		Name: req.Name, DurationMinutes: req.DurationMinutes, PriceCents: req.PriceCents,
		Currency: req.Currency, Description: req.Description,
	})
	if err != nil {
		handleCatalogError(c, err)
		return
	}
	c.JSON(http.StatusOK, toServiceResponse(s))
}

// DeleteService godoc
// @Summary Delete a service
// @Description Requires ownership of the parent business (or superadmin).
// @Tags services
// @Security BearerAuth
// @Param serviceId path string true "Service ID"
// @Success 204 "no content"
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse
// @Failure 403 {object} httpserver.ErrorResponse
// @Failure 404 {object} httpserver.ErrorResponse
// @Router /services/{serviceId} [delete]
func (h *Handler) DeleteService(c *gin.Context) {
	id, ok := parseUUIDParam(c, "serviceId")
	if !ok {
		return
	}
	if err := h.catalog.DeleteService(c.Request.Context(), id, currentUserID(c), isSuperadmin(c)); err != nil {
		handleCatalogError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
