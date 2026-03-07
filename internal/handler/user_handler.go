package handler

import (
	"net/http"

	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/gin-gonic/gin"
)

type UserHandler struct{}

func NewUserHandler() *UserHandler {
	return &UserHandler{}
}

// GetMe — GET /api/v1/users/me
// Trả về thông tin profile của user đang đăng nhập
func (h *UserHandler) GetMe(c *gin.Context) {
	userID, _ := c.Get("userID")

	type UserProfile struct {
		ID              int    `json:"id"`
		FullName        string `json:"full_name"`
		Email           string `json:"email"`
		Phone           string `json:"phone"`
		Role            string `json:"role"`
		ThemePreference string `json:"theme_preference"`
		CreatedAt       string `json:"created_at"`
	}

	var profile UserProfile
	if err := repository.DB.
		Table("users").
		Select("id, full_name, email, phone, role, theme_preference, created_at").
		Where("id = ?", userID).
		First(&profile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Không tìm thấy người dùng"})
		return
	}

	// Thống kê đơn hàng
	var totalOrders int64
	repository.DB.Table("orders").Where("user_id = ? AND status = 'COMPLETED'", userID).Count(&totalOrders)

	var totalTickets int64
	repository.DB.Table("tickets").
		Joins("JOIN orders ON orders.id = tickets.order_id").
		Where("orders.user_id = ? AND orders.status = 'COMPLETED'", userID).
		Count(&totalTickets)

	var totalSpent int64
	repository.DB.Table("orders").
		Where("user_id = ? AND status = 'COMPLETED'", userID).
		Select("COALESCE(SUM(final_amount), 0)").
		Scan(&totalSpent)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user":          profile,
			"total_orders":  totalOrders,
			"total_tickets": totalTickets,
			"total_spent":   totalSpent,
		},
	})
}

// UpdateMe — PUT /api/v1/users/me
// Cập nhật họ tên và số điện thoại
func (h *UserHandler) UpdateMe(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req struct {
		FullName string `json:"full_name" binding:"required"`
		Phone    string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Dữ liệu không hợp lệ"})
		return
	}

	if err := repository.DB.Table("users").
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"full_name": req.FullName,
			"phone":     req.Phone,
		}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Không thể cập nhật thông tin"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Cập nhật thông tin thành công"})
}

// UpdateTheme — PATCH /api/v1/users/theme
func (h *UserHandler) UpdateTheme(c *gin.Context) {
	userID, _ := c.Get("userID")

	var req struct {
		Theme string `json:"theme" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Theme không hợp lệ"})
		return
	}

	if err := repository.DB.Table("users").
		Where("id = ?", userID).
		Update("theme_preference", req.Theme).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": "Không thể cập nhật theme"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Cập nhật theme thành công"})
}
