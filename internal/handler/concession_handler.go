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

type PromotionHandler struct {
	PromotionService *service.PromotionService
}

func NewPromotionHandler() *PromotionHandler {
	return &PromotionHandler{
		PromotionService: &service.PromotionService{},
	}
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
