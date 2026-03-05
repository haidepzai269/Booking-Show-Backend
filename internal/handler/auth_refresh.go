package handler

// Đây là bổ sung thêm hàm RefreshToken vào file auth_handler.go

import (
	"net/http"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	bookingjwt "github.com/booking-show/booking-show-api/pkg/jwt"
	"github.com/gin-gonic/gin"
)

type RefreshReq struct {
	RefreshToken string `json:"refresh_token"`
}

func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// 1. Lấy token từ cookie nếu có, không thì từ body
	tokenString, err := c.Cookie("refresh_token")
	if err != nil {
		var req RefreshReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Refresh token is missing", "code": "INVALID_INPUT"})
			return
		}
		tokenString = req.RefreshToken
	}

	// 2. Validate refresh token
	claims, err := bookingjwt.ValidateToken(tokenString, h.AuthService.Cfg.JWTSecret)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "Invalid or expired refresh token", "code": "UNAUTHORIZED"})
		return
	}

	// 3. (Tuỳ chọn) Check xem UserID có còn active trong DB không
	var user model.User
	if err := repository.DB.Select("id", "role", "is_active").First(&user, claims.UserID).Error; err != nil || !user.IsActive {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": "User blocked or not found", "code": "UNAUTHORIZED"})
		return
	}

	accessExp, _ := time.ParseDuration(h.AuthService.Cfg.JWTAccessExpiration)

	// Sinh access token mới - không renew refresh token ở đây (theo yêu cầu)
	newAccessToken, _, err := bookingjwt.GenerateTokens(user.ID, string(user.Role), h.AuthService.Cfg.JWTSecret, accessExp, 0)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Failed to generate token", "code": "SERVER_ERROR"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"access_token": newAccessToken,
		},
	})
}
