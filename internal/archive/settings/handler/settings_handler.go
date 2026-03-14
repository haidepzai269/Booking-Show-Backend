package handler

import (
	"net/http"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type SettingsHandler struct {
	settingsService *service.SettingsService
}

func NewSettingsHandler() *SettingsHandler {
	return &SettingsHandler{
		settingsService: &service.SettingsService{},
	}
}

func (h *SettingsHandler) GetSettings(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	settings, err := h.settingsService.GetSettings(userID.(int))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, settings)
}

func (h *SettingsHandler) UpdateSettings(c *gin.Context) {
	userID, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req service.UserSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.settingsService.UpdateSettings(userID.(int), req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Settings updated successfully", "settings": req})
}
