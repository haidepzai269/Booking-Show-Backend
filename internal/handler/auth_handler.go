package handler

import (
	"net/http"

	"github.com/booking-show/booking-show-api/config"
	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/booking-show/booking-show-api/pkg/oauth"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"encoding/json"
	"fmt"
	"context"
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

func (h *AuthHandler) CheckAvailability(c *gin.Context) {
	value := c.Query("value")
	fieldType := c.Query("type") // "email" or "fullname"

	if fieldType != "email" && fieldType != "fullname" && fieldType != "username" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Type must be email, fullname or username"})
		return
	}

	available, err := h.AuthService.CheckAvailability(value, fieldType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Internal server error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"available": available,
		},
	})
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
				"theme":     user.Theme,
				"language":  user.Language,
			},
		},
	})
}

func (h *AuthHandler) Logout(c *gin.Context) {
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
				"theme":     user.Theme,
				"language":  user.Language,
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

// ─── OAuth Google ────────────────────────────────────────────────────────

func (h *AuthHandler) GoogleLogin(c *gin.Context) {
	url := oauth.GoogleOauthConfig.AuthCodeURL("state")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	code := c.Query("code")
	token, err := oauth.GoogleOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, h.AuthService.Cfg.FrontendURL+"/login?error=google_auth_failed")
		return
	}

	resp, err := http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + token.AccessToken)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, h.AuthService.Cfg.FrontendURL+"/login?error=google_fetch_failed")
		return
	}
	defer resp.Body.Close()

	data, _ := ioutil.ReadAll(resp.Body)
	var userInfo struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		ID    string `json:"id"`
	}
	json.Unmarshal(data, &userInfo)

	tokens, _, err := h.AuthService.OAuthLoginUser(userInfo.Email, userInfo.Name, "google", userInfo.ID)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, h.AuthService.Cfg.FrontendURL+"/login?error=auth_internal_error")
		return
	}

	c.SetCookie("refresh_token", tokens.RefreshToken, 7*24*3600, "/", "", false, true)
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/login?token=%s", h.AuthService.Cfg.FrontendURL, tokens.AccessToken))
}

// ─── OAuth Facebook ──────────────────────────────────────────────────────

func (h *AuthHandler) FacebookLogin(c *gin.Context) {
	url := oauth.FacebookOauthConfig.AuthCodeURL("state")
	c.Redirect(http.StatusTemporaryRedirect, url)
}

func (h *AuthHandler) FacebookCallback(c *gin.Context) {
	code := c.Query("code")
	token, err := oauth.FacebookOauthConfig.Exchange(context.Background(), code)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, h.AuthService.Cfg.FrontendURL+"/login?error=facebook_auth_failed")
		return
	}

	resp, err := http.Get("https://graph.facebook.com/me?fields=id,name,email&access_token=" + token.AccessToken)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, h.AuthService.Cfg.FrontendURL+"/login?error=facebook_fetch_failed")
		return
	}
	defer resp.Body.Close()

	data, _ := ioutil.ReadAll(resp.Body)
	var userInfo struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		ID    string `json:"id"`
	}
	json.Unmarshal(data, &userInfo)

	// Trường hợp FB không trả về email (hiếm nhưng có thể xảy ra nếu user không cấp quyền)
	if userInfo.Email == "" {
		userInfo.Email = fmt.Sprintf("%s@facebook.com", userInfo.ID)
	}

	tokens, _, err := h.AuthService.OAuthLoginUser(userInfo.Email, userInfo.Name, "facebook", userInfo.ID)
	if err != nil {
		c.Redirect(http.StatusTemporaryRedirect, h.AuthService.Cfg.FrontendURL+"/login?error=auth_internal_error")
		return
	}

	c.SetCookie("refresh_token", tokens.RefreshToken, 7*24*3600, "/", "", false, true)
	c.Redirect(http.StatusTemporaryRedirect, fmt.Sprintf("%s/login?token=%s", h.AuthService.Cfg.FrontendURL, tokens.AccessToken))
}
