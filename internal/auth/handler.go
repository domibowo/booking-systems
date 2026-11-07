package auth

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/domibowo/booking-systems/internal/platform/httpserver"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) RegisterRoutes(r *gin.Engine) {
	g := r.Group("/auth")
	g.POST("/register", h.Register)
	g.POST("/login", h.Login)
	g.POST("/refresh", h.Refresh)
	g.POST("/logout", h.Logout)
}

type registerRequest struct {
	Email    string `json:"email" binding:"required,email" example:"customer@example.com"`
	Password string `json:"password" binding:"required,min=8" example:"password123"`
	FullName string `json:"full_name" binding:"required" example:"Jane Doe"`
}

type loginRequest struct {
	Email    string `json:"email" binding:"required,email" example:"customer@example.com"`
	Password string `json:"password" binding:"required" example:"password123"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

type authResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         userResponse `json:"user"`
}

type userResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Role     string `json:"role" example:"customer"`
}

func toUserResponse(u *User) userResponse {
	return userResponse{
		ID:       u.ID.String(),
		Email:    u.Email,
		FullName: u.FullName,
		Role:     string(u.Role),
	}
}

func toAuthResponse(result *AuthResult) authResponse {
	return authResponse{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		User:         toUserResponse(result.User),
	}
}

// Register godoc
// @Summary Register a new user
// @Description Creates a user with the "customer" role and returns an access/refresh token pair.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body registerRequest true "Registration details"
// @Success 201 {object} authResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 409 {object} httpserver.ErrorResponse "email already registered"
// @Router /auth/register [post]
func (h *Handler) Register(c *gin.Context) {
	var req registerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.service.Register(c.Request.Context(), req.Email, req.Password, req.FullName)
	if err != nil {
		if errors.Is(err, ErrEmailTaken) {
			c.JSON(http.StatusConflict, httpserver.ErrorResponse{Error: "email already registered"})
			return
		}
		c.JSON(http.StatusInternalServerError, httpserver.ErrorResponse{Error: "failed to register"})
		return
	}

	c.JSON(http.StatusCreated, toAuthResponse(result))
}

// Login godoc
// @Summary Log in with email and password
// @Tags auth
// @Accept json
// @Produce json
// @Param request body loginRequest true "Credentials"
// @Success 200 {object} authResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse "invalid email or password"
// @Router /auth/login [post]
func (h *Handler) Login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.service.Login(c.Request.Context(), req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, httpserver.ErrorResponse{Error: "invalid email or password"})
			return
		}
		c.JSON(http.StatusInternalServerError, httpserver.ErrorResponse{Error: "failed to login"})
		return
	}

	c.JSON(http.StatusOK, toAuthResponse(result))
}

// Refresh godoc
// @Summary Rotate a refresh token for a new access/refresh token pair
// @Description The supplied refresh token is revoked as part of rotation; reusing it afterwards fails.
// @Tags auth
// @Accept json
// @Produce json
// @Param request body refreshRequest true "Refresh token"
// @Success 200 {object} authResponse
// @Failure 400 {object} httpserver.ErrorResponse
// @Failure 401 {object} httpserver.ErrorResponse "invalid or expired refresh token"
// @Router /auth/refresh [post]
func (h *Handler) Refresh(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}

	result, err := h.service.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, ErrInvalidToken) {
			c.JSON(http.StatusUnauthorized, httpserver.ErrorResponse{Error: "invalid or expired refresh token"})
			return
		}
		c.JSON(http.StatusInternalServerError, httpserver.ErrorResponse{Error: "failed to refresh token"})
		return
	}

	c.JSON(http.StatusOK, toAuthResponse(result))
}

// Logout godoc
// @Summary Revoke a refresh token
// @Tags auth
// @Accept json
// @Produce json
// @Param request body refreshRequest true "Refresh token"
// @Success 204 "no content"
// @Failure 400 {object} httpserver.ErrorResponse
// @Router /auth/logout [post]
func (h *Handler) Logout(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, httpserver.ErrorResponse{Error: err.Error()})
		return
	}

	if err := h.service.Logout(c.Request.Context(), req.RefreshToken); err != nil {
		c.JSON(http.StatusInternalServerError, httpserver.ErrorResponse{Error: "failed to logout"})
		return
	}

	c.Status(http.StatusNoContent)
}
