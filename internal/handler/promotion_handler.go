package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/booking-show/booking-show-api/pkg/rabbitmq"
	"github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/gin-gonic/gin"
)

type PromotionHandler struct {
	PromotionService *service.PromotionService
}

func NewPromotionHandler() *PromotionHandler {
	return &PromotionHandler{
		PromotionService: &service.PromotionService{},
	}
}

type SubscribeInput struct {
	Email string `json:"email" binding:"required,email"`
}

// SubscribeNewsletter — POST /api/v1/promotions/subscribe
func (h *PromotionHandler) SubscribeNewsletter(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Bạn cần đăng nhập để thực hiện hành động này"})
		return
	}
	userID := userIDVal.(int)

	var input SubscribeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Email không hợp lệ"})
		return
	}

	// 1. Rate Limiting bằng Redis
	if redis.Client != nil {
		key := fmt.Sprintf("newsletter_limit:%d", userID)
		count, _ := redis.Client.Get(redis.Ctx, key).Int()
		if count >= 3 {
			c.JSON(http.StatusTooManyRequests, gin.H{"error": "Bạn đã đăng ký quá nhiều lần. Vui lòng thử lại sau."})
			return
		}
		redis.Client.Incr(redis.Ctx, key)
		redis.Client.Expire(redis.Ctx, key, 1*time.Hour)
	}

	// 2. Lưu vào Database
	sub := model.NewsletterSubscription{
		UserID: userID,
		Email:  input.Email,
	}

	var existing model.NewsletterSubscription
	if err := repository.DB.Where("user_id = ? AND email = ?", userID, sub.Email).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "Bạn đã đăng ký nhận tin từ trước đó!",
			"success": true,
		})
		return
	}

	if err := repository.DB.Create(&sub).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lưu thông tin đăng ký"})
		return
	}

	// 3. Đẩy vào RabbitMQ
	notification := map[string]interface{}{
		"user_id": userID,
		"email":   sub.Email,
		"type":    "NEWSLETTER_WELCOME",
		"message": "Hệ thống đã ghi nhớ bạn! Ưu đãi sẽ sớm đổ bộ vào hòm thư của bạn 🚀",
	}
	body, _ := json.Marshal(notification)
	rabbitmq.PublishMessage("promotion_notifications", body)

	c.JSON(http.StatusOK, gin.H{
		"message": "Hệ thống đã ghi nhớ bạn! Ưu đãi sẽ sớm đổ bộ vào hòm thư của bạn 🚀",
		"success": true,
	})
}

// GetSubscriptionStatus — GET /api/v1/promotions/subscription-status
func (h *PromotionHandler) GetSubscriptionStatus(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Bạn cần đăng nhập để thực hiện hành động này"})
		return
	}
	userID := userIDVal.(int)

	var sub model.NewsletterSubscription
	err := repository.DB.Where("user_id = ?", userID).First(&sub).Error

	if err == nil {
		c.JSON(http.StatusOK, gin.H{
			"subscribed": true,
			"email":      sub.Email,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"subscribed": false,
	})
}

// Validate — POST /api/v1/promotions/validate
func (h *PromotionHandler) Validate(c *gin.Context) {
	var req service.ValidatePromotionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error(), "code": "INVALID_INPUT"})
		return
	}

	result, _, err := h.PromotionService.ValidatePromotion(req)
	if err != nil {
		if appErr, ok := service.IsAppError(err); ok {
			body := gin.H{"success": false, "error": appErr.Msg, "code": appErr.Code}
			if appErr.Data != nil {
				body["data"] = appErr.Data
			}
			c.JSON(appErr.Status, body)
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": result})
}

// ─── Bổ sung vào AdminHandler ──────────────────────────────────────────────

func (h *AdminHandler) ListAdminPromotions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	q := c.Query("q")

	promoService := &service.PromotionService{}
	promos, total, err := promoService.ListAdminPromotions(page, limit, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    promos,
		"meta": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

func (h *AdminHandler) CreatePromotion(c *gin.Context) {
	var req service.PromotionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	promoService := &service.PromotionService{}
	promo, err := promoService.CreatePromotion(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": promo})
}

func (h *AdminHandler) UpdatePromotion(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid promotion ID"})
		return
	}

	var req service.PromotionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	promoService := &service.PromotionService{}
	promo, err := promoService.UpdatePromotion(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": promo})
}

func (h *AdminHandler) DeletePromotion(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid promotion ID"})
		return
	}

	promoService := &service.PromotionService{}
	if err := promoService.DeletePromotion(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Promotion soft deleted"})
}
