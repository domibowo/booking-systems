package auth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/domibowo/booking-systems/internal/platform/httpserver"
)

const (
	ContextUserIDKey   = "auth_user_id"
	ContextUserRoleKey = "auth_user_role"
)

// RequireAuth validates the bearer access token and stores the authenticated
// user's ID and role in the Gin context for downstream handlers.
func (h *Handler) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			httpserver.JSON(c, http.StatusUnauthorized, "missing bearer token", nil)
			c.Abort()
			return
		}

		claims, err := h.service.ParseAccessToken(token)
		if err != nil {
			httpserver.JSON(c, http.StatusUnauthorized, "invalid or expired token", nil)
			c.Abort()
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Set(ContextUserRoleKey, claims.Role)
		c.Next()
	}
}

// RequireRole must run after RequireAuth. It rejects the request unless the
// authenticated user's role is one of the allowed roles.
func RequireRole(roles ...Role) gin.HandlerFunc {
	allowed := make(map[Role]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}

	return func(c *gin.Context) {
		role, _ := c.Get(ContextUserRoleKey)
		r, _ := role.(Role)
		if _, ok := allowed[r]; !ok {
			httpserver.JSON(c, http.StatusForbidden, "insufficient permissions", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}
