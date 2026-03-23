package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/booking-show/booking-show-api/internal/service"
	redispkg "github.com/booking-show/booking-show-api/pkg/redis"
	"github.com/gin-gonic/gin"
)

type ReviewHandler struct {
	reviewSvc *service.ReviewService
}

func NewReviewHandler() *ReviewHandler {
	return &ReviewHandler{
		reviewSvc: service.NewReviewService(),
	}
}

func (h *ReviewHandler) ListMovieReviews(c *gin.Context) {
	movieID, _ := strconv.Atoi(c.Param("id"))
	sort := c.DefaultQuery("sort", "newest")
	ratingFilter, _ := strconv.Atoi(c.DefaultQuery("rating", "0"))
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	
	// Nếu có user đang đăng nhập (optional auth)
	currentUserID := c.GetInt("userID")

	reviews, stats, err := h.reviewSvc.GetMovieReviews(movieID, sort, ratingFilter, page, limit, currentUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reviews": reviews,
		"stats":   stats,
		"page":    page,
		"limit":   limit,
	})
}

func (h *ReviewHandler) CreateReview(c *gin.Context) {
	movieID, _ := strconv.Atoi(c.Param("id"))
	userID := c.GetInt("userID")

	var req struct {
		Rating  int    `json:"rating" binding:"required,min=1,max=5"`
		Content string `json:"content" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	review, err := h.reviewSvc.CreateReview(userID, movieID, req.Rating, req.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, review)
}

func (h *ReviewHandler) DeleteReview(c *gin.Context) {
	reviewID, _ := strconv.Atoi(c.Param("reviewId"))
	userID := c.GetInt("userID")
	role := c.GetString("role")
	isAdmin := role == "ADMIN"

	if err := h.reviewSvc.DeleteReview(userID, reviewID, isAdmin); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "deleted successfully"})
}

func (h *ReviewHandler) ToggleLikeReview(c *gin.Context) {
	reviewID, _ := strconv.Atoi(c.Param("reviewId"))
	userID := c.GetInt("userID")

	if err := h.reviewSvc.ToggleLikeReview(userID, reviewID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "success"})
}

func (h *ReviewHandler) ListAdminReviews(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")

	reviews, total, err := h.reviewSvc.ListAdminReviews(page, limit, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"reviews": reviews,
		"total":   total,
		"page":    page,
		"limit":   limit,
	})
}

func (h *ReviewHandler) UpdateReviewStatus(c *gin.Context) {
	reviewID, _ := strconv.Atoi(c.Param("id"))
	var req struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.reviewSvc.UpdateReviewStatus(reviewID, req.Status); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "updated successfully"})
}

func (h *ReviewHandler) StreamMovieReviews(c *gin.Context) {
	movieID, _ := strconv.Atoi(c.Param("id"))

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")

	if redispkg.Client == nil {
		c.Status(http.StatusInternalServerError)
		return
	}

	channel := fmt.Sprintf("movie:%d:reviews", movieID)
	pubsub := redispkg.Client.Subscribe(redispkg.Ctx, channel)
	defer pubsub.Close()

	ch := pubsub.Channel()
	clientDisconnected := c.Writer.CloseNotify()

	for {
		select {
		case msg := <-ch:
			c.SSEvent("message", msg.Payload)
			c.Writer.Flush()
		case <-clientDisconnected:
			return
		}
	}
}
