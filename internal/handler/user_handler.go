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
func (h *UserHandler) GetMe(c *gin.Context) {
	userID, _ := c.Get("userID")

	type UserProfile struct {
		ID            int     `json:"id"`
		FullName      string  `json:"full_name"`
		Email         string  `json:"email"`
		Phone         string  `json:"phone"`
		Role          string  `json:"role"`
		Rank          string  `json:"rank"`
		TotalSpending float64 `json:"total_spending"`
		Theme         string  `json:"theme"`
		Language      string  `json:"language"`
		CreatedAt     string  `json:"created_at"`
		TotalOrders   int64   `json:"total_orders" gorm:"-"`
		TotalTickets  int64   `json:"total_tickets" gorm:"-"`
	}

	var profile UserProfile
	// Tối ưu: Lấy profile và đếm đơn hoàn thành trong 2 truy vấn đơn giản thay vì join phức tạp nếu không cần thiết
	// Nhưng ở đây ta có thể dùng Raw SQL hoặc các truy vấn gộp để tối ưu hơn.
	if err := repository.DB.
		Table("users").
		Select("id, full_name, email, phone, role, rank, total_spending, theme, language, created_at").
		Where("id = ?", userID).
		First(&profile).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "Không tìm thấy người dùng"})
		return
	}

	// Đếm đơn hàng và vé hoàn thành (Có thể gộp vào SQL SELECT nếu cần, nhưng 2 count đơn giản trên index userID thường nhanh)
	repository.DB.Table("orders").Where("user_id = ? AND status = 'COMPLETED'", userID).Count(&profile.TotalOrders)
	
	repository.DB.Table("tickets").
		Joins("JOIN orders ON orders.id = tickets.order_id").
		Where("orders.user_id = ? AND orders.status = 'COMPLETED'", userID).
		Count(&profile.TotalTickets)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"user":          profile,
			"total_orders":  profile.TotalOrders,
			"total_tickets": profile.TotalTickets,
			"total_spent":   profile.TotalSpending,
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
