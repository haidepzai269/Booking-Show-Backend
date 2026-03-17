package handler

import (
	"net/http"
	"strconv"

	"github.com/booking-show/booking-show-api/internal/service"
	"github.com/gin-gonic/gin"
)

type ConcessionHandler struct {
	ConcessionService *service.ConcessionService
}

func NewConcessionHandler() *ConcessionHandler {
	return &ConcessionHandler{
		ConcessionService: &service.ConcessionService{},
	}
}

func (h *ConcessionHandler) ListConcessions(c *gin.Context) {
	concessions, err := h.ConcessionService.ListConcessions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": concessions})
}

func (h *ConcessionHandler) GetConcession(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid concession ID"})
		return
	}

	concession, err := h.ConcessionService.GetConcession(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": err.Error(), "code": "NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": concession})
}

// ─── Bổ sung AdminHandler Funcs ──────────────────────────────────────────────

func (h *AdminHandler) ListAdminConcessions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
	q := c.Query("q")

	concessionSvc := &service.ConcessionService{}
	concessions, total, err := concessionSvc.ListAdminConcessions(page, limit, q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    concessions,
		"meta": gin.H{
			"page":  page,
			"limit": limit,
			"total": total,
		},
	})
}

func (h *AdminHandler) CreateConcession(c *gin.Context) {
	var req service.ConcessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	concessionSvc := &service.ConcessionService{}
	concession, err := concessionSvc.CreateConcession(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": concession})
}

func (h *AdminHandler) UpdateConcession(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid concession ID"})
		return
	}

	var req service.ConcessionReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}

	concessionSvc := &service.ConcessionService{}
	concession, err := concessionSvc.UpdateConcession(id, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "data": concession})
}

func (h *AdminHandler) DeleteConcession(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid concession ID"})
		return
	}

	concessionSvc := &service.ConcessionService{}
	if err := concessionSvc.DeleteConcession(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Concession soft deleted"})
}


