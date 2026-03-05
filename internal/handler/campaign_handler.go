package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
	"github.com/gin-gonic/gin"
)

type CampaignHandler struct{}

func NewCampaignHandler() *CampaignHandler { return &CampaignHandler{} }

// ─── Public API ────────────────────────────────────────────────────────────────

// ListCampaigns — GET /api/v1/campaigns
// Query: ?type=BANK&page=1&limit=12
func (h *CampaignHandler) ListCampaigns(c *gin.Context) {
	db := repository.DB

	campaignType := c.Query("type")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 12
	}
	offset := (page - 1) * limit

	now := time.Now()
	query := db.Model(&model.Campaign{}).
		Where("status = ?", model.CampaignStatusActive).
		Where("(start_date IS NULL OR start_date <= ?)", now).
		Where("(end_date IS NULL OR end_date >= ?)", now)

	if campaignType != "" {
		query = query.Where("type = ?", campaignType)
	}

	var total int64
	query.Count(&total)

	var campaigns []model.Campaign
	if err := query.Order("sort_order ASC, created_at DESC").
		Offset(offset).Limit(limit).Find(&campaigns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách chiến dịch"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  campaigns,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetCampaign — GET /api/v1/campaigns/:id
func (h *CampaignHandler) GetCampaign(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var campaign model.Campaign
	if err := repository.DB.Where("id = ? AND status = ?", id, model.CampaignStatusActive).First(&campaign).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy chiến dịch"})
		return
	}

	c.JSON(http.StatusOK, campaign)
}

// ─── Admin API ─────────────────────────────────────────────────────────────────

// AdminListCampaigns — GET /api/v1/admin/campaigns
func (h *CampaignHandler) AdminListCampaigns(c *gin.Context) {
	var campaigns []model.Campaign
	query := repository.DB.Model(&model.Campaign{})

	if status := c.Query("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	if err := query.Order("sort_order ASC, created_at DESC").Find(&campaigns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể lấy danh sách"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": campaigns})
}

// AdminGetCampaign — GET /api/v1/admin/campaigns/:id
func (h *CampaignHandler) AdminGetCampaign(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var campaign model.Campaign
	if err := repository.DB.First(&campaign, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy chiến dịch"})
		return
	}

	c.JSON(http.StatusOK, campaign)
}

type CampaignInput struct {
	Title           string  `json:"title" binding:"required"`
	Description     string  `json:"description"`
	ThumbnailURL    string  `json:"thumbnail_url"`
	BannerURL       string  `json:"banner_url"`
	Type            string  `json:"type"`
	HowToAvail      string  `json:"how_to_avail"`
	TermsConditions string  `json:"terms_conditions"`
	StartDate       *string `json:"start_date"`
	EndDate         *string `json:"end_date"`
	Status          string  `json:"status"`
	SortOrder       int     `json:"sort_order"`
}

// AdminCreateCampaign — POST /api/v1/admin/campaigns
func (h *CampaignHandler) AdminCreateCampaign(c *gin.Context) {
	var input CampaignInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	campaign := model.Campaign{
		Title:           input.Title,
		Description:     input.Description,
		ThumbnailURL:    input.ThumbnailURL,
		BannerURL:       input.BannerURL,
		Type:            model.CampaignType(input.Type),
		HowToAvail:      input.HowToAvail,
		TermsConditions: input.TermsConditions,
		Status:          model.CampaignStatus(input.Status),
		SortOrder:       input.SortOrder,
	}

	if input.StartDate != nil && *input.StartDate != "" {
		t, err := time.Parse("2006-01-02", *input.StartDate)
		if err == nil {
			campaign.StartDate = &t
		}
	}
	if input.EndDate != nil && *input.EndDate != "" {
		t, err := time.Parse("2006-01-02", *input.EndDate)
		if err == nil {
			campaign.EndDate = &t
		}
	}

	if campaign.Status == "" {
		campaign.Status = model.CampaignStatusDraft
	}
	if campaign.Type == "" {
		campaign.Type = model.CampaignTypeOther
	}

	if err := repository.DB.Create(&campaign).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể tạo chiến dịch"})
		return
	}

	c.JSON(http.StatusCreated, campaign)
}

// AdminUpdateCampaign — PUT /api/v1/admin/campaigns/:id
func (h *CampaignHandler) AdminUpdateCampaign(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	var campaign model.Campaign
	if err := repository.DB.First(&campaign, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Không tìm thấy chiến dịch"})
		return
	}

	var input CampaignInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	campaign.Title = input.Title
	campaign.Description = input.Description
	campaign.ThumbnailURL = input.ThumbnailURL
	campaign.BannerURL = input.BannerURL
	campaign.Type = model.CampaignType(input.Type)
	campaign.HowToAvail = input.HowToAvail
	campaign.TermsConditions = input.TermsConditions
	campaign.Status = model.CampaignStatus(input.Status)
	campaign.SortOrder = input.SortOrder

	// Reset date fields
	campaign.StartDate = nil
	campaign.EndDate = nil
	if input.StartDate != nil && *input.StartDate != "" {
		t, err := time.Parse("2006-01-02", *input.StartDate)
		if err == nil {
			campaign.StartDate = &t
		}
	}
	if input.EndDate != nil && *input.EndDate != "" {
		t, err := time.Parse("2006-01-02", *input.EndDate)
		if err == nil {
			campaign.EndDate = &t
		}
	}

	if err := repository.DB.Save(&campaign).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể cập nhật chiến dịch"})
		return
	}

	c.JSON(http.StatusOK, campaign)
}

// AdminDeleteCampaign — DELETE /api/v1/admin/campaigns/:id
func (h *CampaignHandler) AdminDeleteCampaign(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ID không hợp lệ"})
		return
	}

	if err := repository.DB.Delete(&model.Campaign{}, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Không thể xóa chiến dịch"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Đã xóa chiến dịch thành công"})
}
