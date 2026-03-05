package middleware

import (
	"net/http"
	"strings"

	"github.com/booking-show/booking-show-api/config"
	bookingjwt "github.com/booking-show/booking-show-api/pkg/jwt"
	"github.com/gin-gonic/gin"
)

// AuthMiddleware kiểm tra token JWT
func AuthMiddleware(cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")

		// Fallback: đọc token từ query param (dùng cho SSE EventSource)
		var tokenString string
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid authorization header format", "code": "UNAUTHORIZED"})
				c.Abort()
				return
			}
			tokenString = parts[1]
		} else if qToken := c.Query("token"); qToken != "" {
			tokenString = qToken
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Authorization required", "code": "UNAUTHORIZED"})
			c.Abort()
			return
		}

		claims, err := bookingjwt.ValidateToken(tokenString, cfg.JWTSecret)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid or expired token", "code": "UNAUTHORIZED"})
			c.Abort()
			return
		}

		// Lưu thông tin user vào context
		c.Set("userID", claims.UserID)
		c.Set("userRole", claims.Role)
		c.Next()
	}
}

// RequireRole chặn quyền truy cập dựa trên role
func RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userRole, exists := c.Get("userRole")
		if !exists {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "Role not found", "code": "FORBIDDEN"})
			c.Abort()
			return
		}

		allowed := false
		for _, role := range roles {
			if userRole.(string) == role {
				allowed = true
				break
			}
		}

		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{"success": false, "error": "You don't have permission to access this resource", "code": "FORBIDDEN"})
			c.Abort()
			return
		}

		c.Next()
	}
}
