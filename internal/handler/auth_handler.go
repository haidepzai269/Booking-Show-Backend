package handler

import (
	"net/http"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	AuthService *service.AuthService
}

func NewAuthHandler(cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		AuthService: &service.AuthService{
			Cfg:          cfg,
			EmailService: service.NewEmailService(cfg),
		},
	}
}

type MagicLinkReq struct {
	Email string `json:"email" binding:"required,email"`
}

type MagicLinkVerifyReq struct {
	Token string `json:"token" binding:"required"`
}

type ResetPasswordReq struct {
	Token       string `json:"token" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req service.RegisterReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error(), "code": "INVALID_INPUT"})
		return
	}

	if err := h.AuthService.RegisterUser(req); err != nil {
		c.JSON(http.StatusConflict, gin.H{"success": false, "error": err.Error(), "code": "REGISTER_FAILED"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": "User registered successfully"})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req service.LoginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error(), "code": "INVALID_INPUT"})
		return
	}

	tokens, user, err := h.AuthService.LoginUser(req)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": err.Error(), "code": "UNAUTHORIZED"})
		return
	}

	// Set refresh token in httpOnly cookie
	c.SetCookie("refresh_token", tokens.RefreshToken, 7*24*3600, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"access_token": tokens.AccessToken,
			"user": gin.H{
				"id":        user.ID,
				"email":     user.Email,
				"full_name": user.FullName,
				"role":      user.Role,
				"theme":     user.ThemePreference,
			},
		},
	})
}

// Logout — POST /auth/logout: xóa refresh_token cookie
func (h *AuthHandler) Logout(c *gin.Context) {
	// Xóa httpOnly cookie refresh_token
	c.SetCookie("refresh_token", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Đăng xuất thành công."})
}

func (h *AuthHandler) RequestMagicLink(c *gin.Context) {
	var req MagicLinkReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Email không hợp lệ", "code": "INVALID_INPUT"})
		return
	}

	if err := h.AuthService.RequestMagicLink(req.Email); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error(), "code": "MAGIC_LINK_FAILED"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Một liên kết đăng nhập đã được gửi đến email của bạn."})
}

func (h *AuthHandler) VerifyMagicLink(c *gin.Context) {
	var req MagicLinkVerifyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Token không hợp lệ", "code": "INVALID_INPUT"})
		return
	}

	tokens, user, err := h.AuthService.VerifyMagicLink(req.Token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "error": err.Error(), "code": "UNAUTHORIZED"})
		return
	}

	// Set refresh token in httpOnly cookie
	c.SetCookie("refresh_token", tokens.RefreshToken, 7*24*3600, "/", "", false, true)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"access_token": tokens.AccessToken,
			"user": gin.H{
				"id":        user.ID,
				"email":     user.Email,
				"full_name": user.FullName,
				"role":      user.Role,
				"theme":     user.ThemePreference,
			},
		},
	})
}

func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Dữ liệu không hợp lệ", "code": "INVALID_INPUT"})
		return
	}

	if err := h.AuthService.ResetPassword(req.Token, req.NewPassword); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error(), "code": "RESET_FAILED"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Mật khẩu của bạn đã được cập nhật thành công."})
}
