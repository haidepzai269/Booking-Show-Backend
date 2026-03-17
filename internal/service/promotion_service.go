package service

import (
	"errors"
	"time"

	"github.com/booking-show/booking-show-api/internal/model"
	"github.com/booking-show/booking-show-api/internal/repository"
)

type PromotionService struct{}

type ValidatePromotionReq struct {
	Code       string `json:"code" binding:"required"`
	OrderValue int    `json:"order_value" binding:"required,gt=0"`
}

type ValidatePromotionRes struct {
	PromotionID    int    `json:"promotion_id"`
	Code           string `json:"code"`
	Description    string `json:"description"`
	DiscountAmount int    `json:"discount_amount"`
	FinalAmount    int    `json:"final_amount"`
}

func (s *PromotionService) ValidatePromotion(req ValidatePromotionReq) (*ValidatePromotionRes, *model.Promotion, error) {
	var promo model.Promotion

	if err := repository.DB.Where("code = ? AND is_active = ?", req.Code, true).First(&promo).Error; err != nil {
		return nil, nil, &AppError{Code: "PROMOTION_NOT_FOUND", Status: 404, Msg: "Ma voucher khong ton tai."}
	}

	now := time.Now()

	if now.Before(promo.ValidFrom) {
		return nil, nil, &AppError{Code: "PROMOTION_NOT_STARTED", Status: 400, Msg: "Voucher chua den ngay hieu luc."}
	}
	if now.After(promo.ValidUntil) {
		return nil, nil, &AppError{Code: "PROMOTION_EXPIRED", Status: 400, Msg: "Voucher da het han."}
	}

	if promo.UsedCount >= promo.UsageLimit {
		return nil, nil, &AppError{Code: "PROMOTION_LIMIT_REACHED", Status: 400, Msg: "Voucher da het luot su dung."}
	}

	if req.OrderValue < promo.MinOrderValue {
		return nil, nil, &AppError{
			Code:   "ORDER_VALUE_TOO_LOW",
			Status: 400,
			Msg:    "Don hang chua dat gia tri toi thieu de ap dung ma nay.",
			Data:   map[string]interface{}{"min_order_value": promo.MinOrderValue},
		}
	}

	finalAmount := req.OrderValue - promo.DiscountAmount
	if finalAmount < 0 {
		finalAmount = 0
	}

	return &ValidatePromotionRes{
		PromotionID:    promo.ID,
		Code:           promo.Code,
		Description:    promo.Description,
		DiscountAmount: promo.DiscountAmount,
		FinalAmount:    finalAmount,
	}, &promo, nil
}

func (s *PromotionService) ListActivePromotions() ([]model.Promotion, error) {
	var promos []model.Promotion
	now := time.Now()
	err := repository.DB.Where("is_active = ? AND valid_from <= ? AND valid_until >= ? AND used_count < usage_limit", true, now, now).Find(&promos).Error
	return promos, err
}

func (s *PromotionService) ListAdminPromotions(page, limit int, q string) ([]model.Promotion, int64, error) {
	var promos []model.Promotion
	var total int64

	query := repository.DB.Model(&model.Promotion{})

	if q != "" {
		query = query.Where("code ILIKE ? OR description ILIKE ?", "%"+q+"%", "%"+q+"%")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * limit
	if err := query.Order("id DESC").Offset(offset).Limit(limit).Find(&promos).Error; err != nil {
		return nil, 0, err
	}

	return promos, total, nil
}

type PromotionReq struct {
	Code           string    `json:"code" binding:"required"`
	Description    string    `json:"description" binding:"required"`
	DiscountAmount int       `json:"discount_amount" binding:"required,gt=0"`
	MinOrderValue  int       `json:"min_order_value"`
	ValidFrom      time.Time `json:"valid_from" binding:"required"`
	ValidUntil     time.Time `json:"valid_until" binding:"required"`
	UsageLimit     int       `json:"usage_limit" binding:"required,gt=0"`
	IsActive       *bool     `json:"is_active"`
}

func (s *PromotionService) CreatePromotion(req PromotionReq) (*model.Promotion, error) {
	promo := model.Promotion{
		Code:           req.Code,
		Description:    req.Description,
		DiscountAmount: req.DiscountAmount,
		MinOrderValue:  req.MinOrderValue,
		ValidFrom:      req.ValidFrom,
		ValidUntil:     req.ValidUntil,
		UsageLimit:     req.UsageLimit,
		IsActive:       true,
	}

	if req.IsActive != nil {
		promo.IsActive = *req.IsActive
	}

	if err := repository.DB.Create(&promo).Error; err != nil {
		return nil, err
	}

	return &promo, nil
}

func (s *PromotionService) UpdatePromotion(id int, req PromotionReq) (*model.Promotion, error) {
	var promo model.Promotion
	if err := repository.DB.First(&promo, id).Error; err != nil {
		return nil, errors.New("promotion not found")
	}

	promo.Code = req.Code
	promo.Description = req.Description
	promo.DiscountAmount = req.DiscountAmount
	promo.MinOrderValue = req.MinOrderValue
	promo.ValidFrom = req.ValidFrom
	promo.ValidUntil = req.ValidUntil
	promo.UsageLimit = req.UsageLimit

	if req.IsActive != nil {
		promo.IsActive = *req.IsActive
	}

	if err := repository.DB.Save(&promo).Error; err != nil {
		return nil, err
	}

	return &promo, nil
}

func (s *PromotionService) DeletePromotion(id int) error {
	var promo model.Promotion
	if err := repository.DB.First(&promo, id).Error; err != nil {
		return errors.New("promotion not found")
	}

	if err := repository.DB.Model(&promo).Update("is_active", false).Error; err != nil {
		return err
	}

	return nil
}
