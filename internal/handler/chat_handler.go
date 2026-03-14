package handler

import (
	"net/http"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type ChatHandler struct {
	ChatService *service.ChatService
}

func NewChatHandler() *ChatHandler {
	return &ChatHandler{
		ChatService: service.NewChatService(),
	}
}

func (h *ChatHandler) GetHistory(c *gin.Context) {
	sessionID := c.Query("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "session_id is required"})
		return
	}

	// Lấy userID từ context nếu đã đăng nhập
	var userIDPtr *uint
	if val, exists := c.Get("userID"); exists {
		uid := val.(int)
		uidUint := uint(uid)
		userIDPtr = &uidUint
	}

	history, err := h.ChatService.GetHistory(sessionID, userIDPtr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    history,
	})
}
