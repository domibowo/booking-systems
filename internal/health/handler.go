package health

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/domibowo/booking-systems/internal/platform/httpserver"
)

type Handler struct {
	db *gorm.DB
}

func NewHandler(db *gorm.DB) *Handler {
	return &Handler{db: db}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	r.GET("/health/live", h.Live)
	r.GET("/health/ready", h.Ready)
}

type statusResponse struct {
	Status string `json:"status" example:"ok"`
}

// Live godoc
// @Summary Liveness probe
// @Description Reports whether the process is up, without checking dependencies.
// @Tags health
// @Produce json
// @Success 200 {object} statusResponse
// @Router /health/live [get]
func (h *Handler) Live(c *gin.Context) {
	c.JSON(http.StatusOK, statusResponse{Status: "ok"})
}

// Ready godoc
// @Summary Readiness probe
// @Description Reports whether the service can serve traffic, including database connectivity.
// @Tags health
// @Produce json
// @Success 200 {object} statusResponse
// @Failure 503 {object} httpserver.ErrorResponse
// @Router /health/ready [get]
func (h *Handler) Ready(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, httpserver.ErrorResponse{Error: err.Error()})
		return
	}

	if err := sqlDB.PingContext(c.Request.Context()); err != nil {
		c.JSON(http.StatusServiceUnavailable, httpserver.ErrorResponse{Error: err.Error()})
		return
	}

	c.JSON(http.StatusOK, statusResponse{Status: "ok"})
}
