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

// Live godoc
// @Summary Liveness probe
// @Description Reports whether the process is up, without checking dependencies.
// @Tags health
// @Produce json
// @Success 200 {object} httpserver.Response{data=object}
// @Router /health/live [get]
func (h *Handler) Live(c *gin.Context) {
	httpserver.JSON(c, http.StatusOK, "service is up", nil)
}

// Ready godoc
// @Summary Readiness probe
// @Description Reports whether the service can serve traffic, including database connectivity.
// @Tags health
// @Produce json
// @Success 200 {object} httpserver.Response{data=object}
// @Failure 503 {object} httpserver.Response{data=object}
// @Router /health/ready [get]
func (h *Handler) Ready(c *gin.Context) {
	sqlDB, err := h.db.DB()
	if err != nil {
		httpserver.JSON(c, http.StatusServiceUnavailable, err.Error(), nil)
		return
	}

	if err := sqlDB.PingContext(c.Request.Context()); err != nil {
		httpserver.JSON(c, http.StatusServiceUnavailable, err.Error(), nil)
		return
	}

	httpserver.JSON(c, http.StatusOK, "service is ready", nil)
}
