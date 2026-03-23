package handler

import (
	"net/http"
	"strconv"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type BlacklistedWordHandler struct {
	bwSvc *service.BlacklistedWordService
}

func NewBlacklistedWordHandler() *BlacklistedWordHandler {
	return &BlacklistedWordHandler{
		bwSvc: &service.BlacklistedWordService{},
	}
}

func (h *BlacklistedWordHandler) GetBlacklistedWords(c *gin.Context) {
	words, err := h.bwSvc.GetAllWords()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, words)
}

func (h *BlacklistedWordHandler) AddBlacklistedWord(c *gin.Context) {
	var req struct {
		Word string `json:"word" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	word, err := h.bwSvc.AddWord(req.Word)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, word)
}

func (h *BlacklistedWordHandler) DeleteBlacklistedWord(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.bwSvc.DeleteWord(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted successfully"})
}
